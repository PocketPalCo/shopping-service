package telegram

import (
	"context"
	"log/slog"
	"sync"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/ai"
	"github.com/PocketPalCo/shopping-service/internal/core/cloud"
	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/products"
	"github.com/PocketPalCo/shopping-service/internal/core/receipts"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/stt"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TelegramService interface defines the contract for Telegram bot management
type TelegramService interface {
	Start(ctx context.Context) error
	Stop()
	IsEnabled() bool
}

// Service implements TelegramService interface
type Service struct {
	cfg          *config.Config
	usersService *users.Service
	botService   *BotService
	enabled      bool
	logger       *slog.Logger
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewTelegramService creates a new Telegram service instance
func NewTelegramService(cfg *config.Config, db *pgxpool.Pool, logger *slog.Logger) (TelegramService, error) {
	if cfg.TelegramBotToken == "" {
		logger.Info("Telegram bot disabled - no token provided")
		return &Service{
			cfg:     cfg,
			enabled: false,
			logger:  logger,
		}, nil
	}

	// Load admin users from config
	admins, err := cfg.GetTelegramAdmins()
	if err != nil {
		logger.Error("failed to parse telegram admin IDs", "error", err)
		return nil, err
	}

	usersService := users.NewService(db, admins)
	familiesService := families.NewService(db)

	// Initialize products service
	productsService := products.NewService(db, logger)

	// Initialize AI service with OpenAI client using config
	openAIConfig := cfg.GetOpenAIConfig()

	if openAIConfig.APIKey == "" {
		logger.Error("failed to initialize OpenAI client",
			"error", "OpenAI API key not configured (set SSV_OPENAI_API_KEY)",
			"component", "ai_service")
		return nil, nil
	}

	// Create OpenAI client with the products service
	aiClient := ai.NewOpenAIClient(openAIConfig, logger, productsService)

	// Create the AI service with the client
	aiService, err := ai.NewService(db, aiClient, *cfg, logger)
	if err != nil {
		logger.Error("failed to initialize AI service", "error", err)
		return nil, err
	}

	shoppingService := shopping.NewService(db, aiService)

	// Initialize cloud storage service
	cloudConfig := cfg.GetCloudConfig()
	cloudService, err := cloud.NewService(cloudConfig, logger)
	if err != nil {
		logger.Error("failed to initialize cloud service", "error", err)
		return nil, err
	}

	// Initialize receipts service
	receiptsService := receipts.NewService(db, cloudService, aiService, logger)

	// Initialize STT client
	sttClient := stt.NewClient(cfg.STTServiceURL)

	botService, err := NewBotService(cfg.TelegramBotToken, usersService, familiesService, shoppingService, receiptsService, sttClient, logger, cfg.TelegramDebug)
	if err != nil {
		logger.Error("failed to initialize telegram bot", "error", err)
		return nil, err
	}

	if len(admins) > 0 {
		logger.Info("Telegram admin users configured", "count", len(admins))
	} else {
		logger.Warn("No Telegram admin users configured - admin commands will not work")
	}

	logger.Info("Telegram bot initialized",
		"bot_enabled", true,
		"admin_count", len(admins),
		"debug_mode", cfg.TelegramDebug,
		"component", "telegram_service")

	return &Service{
		cfg:          cfg,
		usersService: usersService,
		botService:   botService,
		enabled:      true,
		logger:       logger,
	}, nil
}

// Start begins the Telegram bot service
func (s *Service) Start(ctx context.Context) error {
	if !s.enabled {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start bot service
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.botService.Start(s.ctx); err != nil && err != context.Canceled {
			s.logger.Error("Telegram bot error", "error", err)
		}
	}()

	s.logger.Info("Telegram service started",
		"component", "telegram_service",
		"bot_enabled", s.enabled)
	return nil
}

// Stop gracefully shuts down the Telegram service
func (s *Service) Stop() {
	if !s.enabled {
		return
	}

	s.logger.Info("Stopping Telegram service...")

	// Cancel context to stop all goroutines
	if s.cancel != nil {
		s.cancel()
	}

	// Stop bot service
	if s.botService != nil {
		s.botService.Stop()
	}

	// Wait for all goroutines to finish
	s.wg.Wait()

	s.logger.Info("Telegram service stopped")
}

// IsEnabled returns whether the Telegram service is enabled
func (s *Service) IsEnabled() bool {
	return s.enabled
}
