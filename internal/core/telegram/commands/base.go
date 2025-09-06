package commands

import (
	"log/slog"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BaseCommand provides common functionality for all commands
type BaseCommand struct {
	bot             *tgbotapi.BotAPI
	usersService    *users.Service
	familiesService *families.Service
	shoppingService *shopping.Service
	templateManager *telegram.TemplateManager
	logger          *slog.Logger
}

// NewBaseCommand creates a new base command with common dependencies
func NewBaseCommand(
	bot *tgbotapi.BotAPI,
	usersService *users.Service,
	familiesService *families.Service,
	shoppingService *shopping.Service,
	templateManager *telegram.TemplateManager,
	logger *slog.Logger,
) BaseCommand {
	return BaseCommand{
		bot:             bot,
		usersService:    usersService,
		familiesService: familiesService,
		shoppingService: shoppingService,
		templateManager: templateManager,
		logger:          logger,
	}
}

// SendMessage sends a simple text message
func (bc *BaseCommand) SendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bc.bot.Send(msg); err != nil {
		bc.logger.Error("Failed to send message", "error", err, "chat_id", chatID)
	}
}

// SendHTMLMessage sends a message with HTML formatting
func (bc *BaseCommand) SendHTMLMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := bc.bot.Send(msg); err != nil {
		bc.logger.Error("Failed to send HTML message", "error", err, "chat_id", chatID)
	}
}

// SendMessageWithKeyboard sends a message with inline keyboard
func (bc *BaseCommand) SendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	if _, err := bc.bot.Send(msg); err != nil {
		bc.logger.Error("Failed to send message with keyboard", "error", err, "chat_id", chatID)
	}
}
