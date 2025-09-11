package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/stt"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/commands"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/handlers"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	"github.com/PocketPalCo/shopping-service/pkg/telemetry"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
)

type BotService struct {
	bot             *tgbotapi.BotAPI
	usersService    *users.Service
	familiesService *families.Service
	shoppingService *shopping.Service
	sttClient       *stt.Client
	logger          *slog.Logger
	templateManager *TemplateManager

	// Refactored components with proper user separation
	commandRegistry            *commands.CommandRegistry
	stateManager               *handlers.StateManager
	listCallbackHandler        *handlers.ListCallbackHandler
	generalCallbackHandler     *handlers.GeneralCallbackHandler
	duplicateCallbackHandler   *handlers.DuplicateCallbackHandler
	productListCallbackHandler *handlers.ProductListCallbackHandler
	messageHandler             *handlers.MessageHandler
	userMapper                 *UserMapper
}

func NewBotService(token string, usersService *users.Service, familiesService *families.Service, shoppingService *shopping.Service, sttClient *stt.Client, logger *slog.Logger, debug bool) (*BotService, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	bot.Debug = debug

	templateManager, err := NewTemplateManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create template manager: %w", err)
	}

	// Set up command registry
	commandRegistry := commands.SetupCommands(bot, usersService, familiesService, shoppingService, templateManager, logger)

	// Set up user mapper
	userMapper := NewUserMapper(usersService)

	// Set up handlers
	baseHandler := handlers.NewBaseHandler(bot, usersService, familiesService, shoppingService, templateManager, logger)
	stateManager := handlers.NewStateManager(logger)
	listCallbackHandler := handlers.NewListCallbackHandler(baseHandler)
	generalCallbackHandler := handlers.NewGeneralCallbackHandler(baseHandler)
	duplicateCallbackHandler := handlers.NewDuplicateCallbackHandler(baseHandler, stateManager)
	productListCallbackHandler := handlers.NewProductListCallbackHandler(baseHandler, stateManager)
	messageHandler := handlers.NewMessageHandler(baseHandler, stateManager)

	return &BotService{
		bot:                        bot,
		usersService:               usersService,
		familiesService:            familiesService,
		shoppingService:            shoppingService,
		sttClient:                  sttClient,
		logger:                     logger,
		templateManager:            templateManager,
		commandRegistry:            commandRegistry,
		stateManager:               stateManager,
		listCallbackHandler:        listCallbackHandler,
		generalCallbackHandler:     generalCallbackHandler,
		duplicateCallbackHandler:   duplicateCallbackHandler,
		productListCallbackHandler: productListCallbackHandler,
		messageHandler:             messageHandler,
		userMapper:                 userMapper,
	}, nil
}

func (s *BotService) Start(ctx context.Context) error {
	s.logger.Info("Starting refactored bot service")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Bot service context cancelled, stopping")
			return ctx.Err()
		case update := <-updates:
			go s.handleUpdate(ctx, update)
		}
	}
}

func (s *BotService) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.Message != nil {
		s.handleMessage(ctx, update.Message)
	} else if update.CallbackQuery != nil {
		s.handleCallbackQuery(ctx, update.CallbackQuery)
	}
}

func (s *BotService) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	// Convert Telegram user to our TelegramUser type
	tgUser := NewTelegramUserFromAPI(message.From)

	// Get or create internal user using proper mapping
	internalUser, err := s.userMapper.GetOrCreateInternalUser(ctx, tgUser)
	if err != nil {
		s.logger.Error("Failed to get or create internal user", "error", err)
		return
	}

	s.logger.Info("Received message",
		"user_id", internalUser.TelegramID,
		"username", internalUser.Username,
		"is_admin", internalUser.IsAdmin,
		"is_authorized", internalUser.IsAuthorized,
		"message", message.Text,
		"is_command", message.IsCommand())

	// Determine message type
	messageType := "text"
	if message.IsCommand() {
		messageType = "command"
	} else if message.Voice != nil {
		messageType = "voice"
	} else if message.Audio != nil {
		messageType = "audio"
	}

	// Record message received metric
	if telemetry.TelegramMessagesTotal != nil {
		telemetry.TelegramMessagesTotal.Add(ctx, 1,
			api.WithAttributes(
				attribute.String("type", messageType),
				attribute.Bool("authorized", internalUser.IsAuthorized),
				attribute.Bool("admin", internalUser.IsAdmin),
			),
		)
	}

	// Handle different message types
	if message.IsCommand() {
		s.handleCommand(ctx, message, internalUser)
	} else if message.Voice != nil || message.Audio != nil {
		s.handleAudioMessage(ctx, message, internalUser)
	} else {
		s.handleTextMessage(ctx, message, internalUser)
	}
}

func (s *BotService) handleCommand(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	commandName := message.Command()
	args := strings.Fields(message.CommandArguments())

	// Get command from registry
	command, exists := s.commandRegistry.Get(commandName)
	if !exists {
		// Record unknown command metric
		if telemetry.TelegramCommandsTotal != nil {
			telemetry.TelegramCommandsTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("command", commandName),
					attribute.String("status", "unknown"),
				),
			)
		}
		s.sendMessage(message.Chat.ID, "❌ Unknown command. Type /help to see available commands.")
		return
	}

	// Check authorization requirements
	if command.RequiresAuth() && !user.IsAuthorized {
		// Record unauthorized command attempt metric
		if telemetry.TelegramCommandsTotal != nil {
			telemetry.TelegramCommandsTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("command", commandName),
					attribute.String("status", "unauthorized"),
				),
			)
		}
		s.messageHandler.SendUnauthorizedMessage(message.Chat.ID, user.User)
		return
	}

	// Check admin requirements (now properly using our internal IsAdmin)
	if command.RequiresAdmin() && !user.IsAdmin {
		// Record insufficient privileges metric
		if telemetry.TelegramCommandsTotal != nil {
			telemetry.TelegramCommandsTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("command", commandName),
					attribute.String("status", "insufficient_privileges"),
				),
			)
		}
		s.sendMessage(message.Chat.ID, "❌ This command requires administrator privileges.")
		return
	}

	// Execute command
	if err := command.Handle(ctx, message.Chat.ID, user.User, args); err != nil {
		s.logger.Error("Command execution failed", "error", err, "command", commandName)
		// Record command error metric
		if telemetry.TelegramCommandsTotal != nil {
			telemetry.TelegramCommandsTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("command", commandName),
					attribute.String("status", "error"),
				),
			)
		}
		s.sendMessage(message.Chat.ID, "❌ Command failed. Please try again.")
	} else {
		// Record successful command execution metric
		if telemetry.TelegramCommandsTotal != nil {
			telemetry.TelegramCommandsTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("command", commandName),
					attribute.String("status", "success"),
				),
			)
		}
	}
}

func (s *BotService) handleTextMessage(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	if !user.IsAuthorized {
		s.messageHandler.SendUnauthorizedMessage(message.Chat.ID, user.User)
		return
	}

	// Check for user states
	if familyIDStr, hasState := s.stateManager.GetUserState(user.TelegramID, "creating_list_for_family"); hasState {
		s.messageHandler.HandleShoppingListNameInput(ctx, message, user.User, familyIDStr)
		s.stateManager.ClearUserState(user.TelegramID, "creating_list_for_family")
		return
	}

	if listIDStr, hasState := s.stateManager.GetUserState(user.TelegramID, "adding_item_to_list"); hasState {
		s.messageHandler.HandleAddItemInput(ctx, message, user.User, listIDStr)
		s.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")
		return
	}

	if stateData, hasState := s.stateManager.GetUserState(user.TelegramID, "creating_custom_productlist"); hasState {
		s.productListCallbackHandler.HandleCustomListNameInput(ctx, message, user.User, stateData)
		s.stateManager.ClearUserState(user.TelegramID, "creating_custom_productlist")
		return
	}

	// Default message handling
	s.messageHandler.HandleAuthorizedMessage(ctx, message, user.User)
}

func (s *BotService) handleAudioMessage(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	if !user.IsAuthorized {
		s.messageHandler.SendUnauthorizedMessage(message.Chat.ID, user.User)
		return
	}

	s.logger.Info("Processing audio message",
		"user_id", user.TelegramID,
		"username", user.Username,
		"chat_id", message.Chat.ID)

	// Send processing message
	s.sendMessage(message.Chat.ID, "🎵 Processing your audio message...")

	// Determine the audio file to process
	var fileID string
	var filename string
	if message.Voice != nil {
		fileID = message.Voice.FileID
		filename = fmt.Sprintf("voice_%d.ogg", message.MessageID)
	} else if message.Audio != nil {
		fileID = message.Audio.FileID
		filename = message.Audio.FileName
		if filename == "" {
			filename = fmt.Sprintf("audio_%d.mp3", message.MessageID)
		}
	} else {
		s.sendMessage(message.Chat.ID, "❌ No audio file found in message.")
		return
	}

	// Get file from Telegram
	file, err := s.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		s.logger.Error("Failed to get audio file from Telegram", "error", err)
		s.sendMessage(message.Chat.ID, "❌ Failed to download audio file.")
		return
	}

	// Download file content
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", s.bot.Token, file.FilePath)
	resp, err := http.Get(fileURL)
	if err != nil {
		s.logger.Error("Failed to download audio file", "error", err)
		s.sendMessage(message.Chat.ID, "❌ Failed to download audio file.")
		return
	}
	defer resp.Body.Close()

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Error("Failed to read audio data", "error", err)
		s.sendMessage(message.Chat.ID, "❌ Failed to read audio file.")
		return
	}

	// Determine user's language from their locale
	language := user.Locale
	if language == "" {
		language = "en" // Default to English
	}

	// Map locale to STT language codes
	sttLanguage := language
	switch language {
	case "uk":
		sttLanguage = "uk"
	case "ru":
		sttLanguage = "ru"
	default:
		sttLanguage = "en"
	}

	// Process audio with STT service
	sttReq := stt.STTRequest{
		SessionID:      fmt.Sprintf("telegram_%d_%d", user.TelegramID, message.MessageID),
		ChunkID:        1,
		Language:       sttLanguage,
		TargetLanguage: sttLanguage, // Same language for now
		AudioData:      audioData,
		Filename:       filename,
	}

	sttResp, err := s.sttClient.ProcessAudio(ctx, sttReq)
	if err != nil {
		s.logger.Error("Failed to process audio with STT service", "error", err)
		s.sendMessage(message.Chat.ID, "❌ Failed to transcribe audio. Please try again.")
		return
	}

	// Send transcription result
	transcriptionText := sttResp.RawText
	if transcriptionText == "" {
		s.sendMessage(message.Chat.ID, "🎵 Audio processed, but no speech was detected.")
		return
	}

	// Format response message
	responseMsg := fmt.Sprintf("🎵 <b>Audio Transcription:</b>\n\n%s", transcriptionText)
	if sttResp.Translation != "" && sttResp.Translation != sttResp.RawText {
		responseMsg += fmt.Sprintf("\n\n<b>Translation:</b>\n%s", sttResp.Translation)
	}

	// Add processing time if available
	if sttResp.ProcessingTimeS > 0 {
		responseMsg += fmt.Sprintf("\n\n<i>Processing time: %.2fs</i>", sttResp.ProcessingTimeS)
	}

	s.sendMessage(message.Chat.ID, responseMsg)

	s.logger.Info("Audio message processed successfully",
		"user_id", user.TelegramID,
		"session_id", sttReq.SessionID,
		"text_length", len(transcriptionText),
		"processing_time", sttResp.ProcessingTimeS)
}

func (s *BotService) handleCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	// Convert Telegram user to our TelegramUser type
	tgUser := NewTelegramUserFromAPI(callback.From)

	// Get internal user using proper mapping
	internalUser, err := s.userMapper.GetInternalUserByTelegramID(ctx, tgUser.ID)
	if err != nil {
		s.logger.Error("Failed to get internal user", "error", err, "telegram_id", callback.From.ID)
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}
	if internalUser == nil {
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}

	s.logger.Info("Received callback query",
		"user_id", internalUser.TelegramID,
		"is_admin", internalUser.IsAdmin,
		"is_authorized", internalUser.IsAuthorized,
		"data", callback.Data,
		"chat_id", callback.Message.Chat.ID)

	// Parse callback data
	parts := strings.Split(callback.Data, "_")
	if len(parts) < 1 {
		s.answerCallback(callback.ID, "❌ Invalid callback data.")
		return
	}

	switch parts[0] {
	case "list":
		s.listCallbackHandler.HandleListCallback(ctx, callback, parts, internalUser.User, s.stateManager)
	case "li":
		// Compact list item callback (li_t_listID_itemID)
		s.generalCallbackHandler.HandleCompactListItem(ctx, callback, parts, internalUser.User)
	case "createlist":
		// Create list for family (createlist_familyID)
		if len(parts) >= 2 {
			s.generalCallbackHandler.HandleCreateListForFamily(ctx, callback, parts[1], internalUser.User)
			s.stateManager.SetUserState(internalUser.TelegramID, "creating_list_for_family", parts[1])
		}
	case "show":
		s.generalCallbackHandler.HandleShowCallback(ctx, callback, parts, internalUser.User)
	case "create":
		if len(parts) >= 3 && parts[1] == "new" && parts[2] == "list" {
			s.generalCallbackHandler.HandleCreateNewList(ctx, callback, internalUser.User)
		}
	case "dup":
		s.duplicateCallbackHandler.HandleDuplicateCallback(ctx, callback, parts, internalUser.User)
	case "productlist":
		s.productListCallbackHandler.HandleProductListCallback(ctx, callback, parts, internalUser.User)
	default:
		s.answerCallback(callback.ID, "❌ Unknown callback action.")
	}
}

func (s *BotService) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send message", "error", err, "chat_id", chatID)
	}
}

func (s *BotService) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := s.bot.Request(callback); err != nil {
		s.logger.Error("Failed to answer callback", "error", err, "callback_id", callbackID)
	}
}

func (s *BotService) Stop() {
	s.bot.StopReceivingUpdates()
}
