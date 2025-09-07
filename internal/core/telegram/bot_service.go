package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/commands"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/handlers"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type BotService struct {
	bot             *tgbotapi.BotAPI
	usersService    *users.Service
	familiesService *families.Service
	shoppingService *shopping.Service
	logger          *slog.Logger
	templateManager *TemplateManager

	// Refactored components with proper user separation
	commandRegistry          *commands.CommandRegistry
	stateManager             *handlers.StateManager
	listCallbackHandler      *handlers.ListCallbackHandler
	generalCallbackHandler   *handlers.GeneralCallbackHandler
	duplicateCallbackHandler *handlers.DuplicateCallbackHandler
	messageHandler           *handlers.MessageHandler
	userMapper               *UserMapper
}

func NewBotService(token string, usersService *users.Service, familiesService *families.Service, shoppingService *shopping.Service, logger *slog.Logger, debug bool) (*BotService, error) {
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
	messageHandler := handlers.NewMessageHandler(baseHandler, stateManager)

	return &BotService{
		bot:                      bot,
		usersService:             usersService,
		familiesService:          familiesService,
		shoppingService:          shoppingService,
		logger:                   logger,
		templateManager:          templateManager,
		commandRegistry:          commandRegistry,
		stateManager:             stateManager,
		listCallbackHandler:      listCallbackHandler,
		generalCallbackHandler:   generalCallbackHandler,
		duplicateCallbackHandler: duplicateCallbackHandler,
		messageHandler:           messageHandler,
		userMapper:               userMapper,
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

	if message.IsCommand() {
		s.handleCommand(ctx, message, internalUser)
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
		s.sendMessage(message.Chat.ID, "❌ Unknown command. Type /help to see available commands.")
		return
	}

	// Check authorization requirements
	if command.RequiresAuth() && !user.IsAuthorized {
		s.messageHandler.SendUnauthorizedMessage(message.Chat.ID, user.User)
		return
	}

	// Check admin requirements (now properly using our internal IsAdmin)
	if command.RequiresAdmin() && !user.IsAdmin {
		s.sendMessage(message.Chat.ID, "❌ This command requires administrator privileges.")
		return
	}

	// Execute command
	if err := command.Handle(ctx, message.Chat.ID, user.User, args); err != nil {
		s.logger.Error("Command execution failed", "error", err, "command", commandName)
		s.sendMessage(message.Chat.ID, "❌ Command failed. Please try again.")
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

	// Default message handling
	s.messageHandler.HandleAuthorizedMessage(ctx, message, user.User)
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
		s.handleListCallback(ctx, callback, parts, internalUser)
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
		s.handleShowCallback(ctx, callback, parts, internalUser)
	case "create":
		if len(parts) >= 3 && parts[1] == "new" && parts[2] == "list" {
			s.generalCallbackHandler.HandleCreateNewList(ctx, callback, internalUser.User)
		}
	case "dup":
		s.duplicateCallbackHandler.HandleDuplicateCallback(ctx, callback, parts, internalUser.User)
	default:
		s.answerCallback(callback.ID, "❌ Unknown callback action.")
	}
}

func (s *BotService) handleListCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *InternalUser) {
	if len(parts) < 3 {
		s.answerCallback(callback.ID, "❌ Invalid list callback.")
		return
	}

	listIDStr := parts[2]
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		s.logger.Error("Failed to parse list ID", "error", err, "list_id", listIDStr)
		s.answerCallback(callback.ID, "❌ Invalid list ID.")
		return
	}

	action := parts[1]
	switch action {
	case "view":
		s.listCallbackHandler.HandleViewList(ctx, callback, listID, user.User)
	case "additem":
		s.listCallbackHandler.HandleAddItem(ctx, callback, listID, user.User)
		s.stateManager.SetUserState(user.TelegramID, "adding_item_to_list", listID.String())
	case "toggleitem":
		if len(parts) >= 4 {
			s.listCallbackHandler.HandleToggleItem(ctx, callback, listID, parts[3], user.User)
		}
	default:
		s.answerCallback(callback.ID, "❌ Unknown list action.")
	}
}

func (s *BotService) handleShowCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *InternalUser) {
	if len(parts) < 2 {
		s.answerCallback(callback.ID, "❌ Invalid show callback.")
		return
	}

	switch parts[1] {
	case "all", "lists":
		s.generalCallbackHandler.HandleShowAllLists(ctx, callback, user.User)
	default:
		s.answerCallback(callback.ID, "❌ Unknown show action.")
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
