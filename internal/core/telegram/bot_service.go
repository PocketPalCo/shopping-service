package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/stt"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/commands"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/handlers"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	menuCallbackHandler        *handlers.MenuCallbackHandler
	messageHandler             *handlers.MessageHandler
	userMapper                 *UserMapper

	// New organized handlers
	callbackRouter        *handlers.CallbackRouter
	coreMessageHandler    *handlers.CoreMessageHandler
	languageHandler       *handlers.LanguageHandler
	userManagementHandler *handlers.UserManagementHandler
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
	listCallbackHandler := handlers.NewListCallbackHandler(baseHandler, stateManager)
	generalCallbackHandler := handlers.NewGeneralCallbackHandler(baseHandler, stateManager)
	duplicateCallbackHandler := handlers.NewDuplicateCallbackHandler(baseHandler, stateManager)
	productListCallbackHandler := handlers.NewProductListCallbackHandler(baseHandler, stateManager)
	menuCallbackHandler := handlers.NewMenuCallbackHandler(baseHandler, commandRegistry, stateManager)
	messageHandler := handlers.NewMessageHandler(baseHandler, stateManager)

	// Set up new organized handlers
	languageHandler := handlers.NewLanguageHandler(baseHandler)
	userManagementHandler := handlers.NewUserManagementHandler(baseHandler)
	coreMessageHandler := handlers.NewCoreMessageHandler(baseHandler, sttClient, commandRegistry, stateManager)

	// Set up callback router with all handlers
	callbackRouter := handlers.NewCallbackRouter(
		baseHandler,
		menuCallbackHandler,
		listCallbackHandler,
		generalCallbackHandler,
		duplicateCallbackHandler,
		productListCallbackHandler,
		languageHandler,
		stateManager,
	)

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
		menuCallbackHandler:        menuCallbackHandler,
		messageHandler:             messageHandler,
		userMapper:                 userMapper,

		// New organized handlers
		callbackRouter:        callbackRouter,
		coreMessageHandler:    coreMessageHandler,
		languageHandler:       languageHandler,
		userManagementHandler: userManagementHandler,
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
	// Get or create internal user using proper mapping
	internalUser, err := s.userManagementHandler.GetOrCreateInternalUser(ctx, message.From)
	if err != nil {
		s.logger.Error("Failed to get or create internal user", "error", err)
		return
	}

	// Notify admins if this is a new user
	if internalUser.User != nil {
		s.userManagementHandler.NotifyAdminsOfNewUser(ctx, internalUser.User)
	}

	s.logger.Info("Received message",
		"user_id", internalUser.TelegramID,
		"username", internalUser.User.Username,
		"is_admin", internalUser.IsAdmin,
		"is_authorized", internalUser.IsAuthorized,
		"message", message.Text,
		"is_command", message.IsCommand())

	// Use the core message handler based on message type
	if message.IsCommand() {
		s.coreMessageHandler.HandleCommand(ctx, message, internalUser)
	} else if message.Voice != nil || message.Audio != nil {
		s.coreMessageHandler.HandleAudioMessage(ctx, message, internalUser)
	} else {
		s.coreMessageHandler.HandleTextMessage(ctx, message, internalUser)
	}
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

	// Use the callback router to handle all callback queries
	s.callbackRouter.RouteCallbackQuery(ctx, callback, internalUser.User)
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
