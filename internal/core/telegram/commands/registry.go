package commands

import (
	"log/slog"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/receipts"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SetupCommands initializes all bot commands and returns a registry
func SetupCommands(
	bot *tgbotapi.BotAPI,
	usersService *users.Service,
	familiesService *families.Service,
	shoppingService *shopping.Service,
	receiptsService *receipts.Service,
	templateManager TemplateRenderer,
	logger *slog.Logger,
) *CommandRegistry {
	registry := NewCommandRegistry()

	// Create base command with dependencies
	base := NewBaseCommand(bot, usersService, familiesService, shoppingService, receiptsService, templateManager, logger)

	// Register all commands
	registry.Register(NewStartCommand(base))
	registry.Register(NewHelpCommand(base))
	registry.Register(NewStatusCommand(base))
	registry.Register(NewMyIDCommand(base))
	registry.Register(NewListsCommand(base))
	registry.Register(NewCreateListCommand(base))
	registry.Register(NewFamiliesCommand(base))
	registry.Register(NewCreateFamilyCommand(base))
	registry.Register(NewAddFamilyMemberCommand(base))
	registry.Register(NewReceiptsCommand(base))

	// Admin commands
	registry.Register(NewAuthorizeCommand(base))
	registry.Register(NewRevokeCommand(base))
	registry.Register(NewUsersCommand(base))
	registry.Register(NewStatsCommand(base))

	// Additional commands can be registered here as needed
	// Follow the same pattern: registry.Register(NewCommandName(base))

	return registry
}
