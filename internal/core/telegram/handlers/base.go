package handlers

import (
	"log/slog"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TemplateRenderer interface defines the contract for template rendering
type TemplateRenderer interface {
	RenderTemplate(templateName, locale string, data interface{}) (string, error)
	RenderButton(buttonName, locale string) string
	RenderMessage(messageName, locale string) string
}

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
	bot             *tgbotapi.BotAPI
	usersService    *users.Service
	familiesService *families.Service
	shoppingService *shopping.Service
	templateManager TemplateRenderer
	logger          *slog.Logger
}

// NewBaseHandler creates a new base handler with common dependencies
func NewBaseHandler(
	bot *tgbotapi.BotAPI,
	usersService *users.Service,
	familiesService *families.Service,
	shoppingService *shopping.Service,
	templateManager TemplateRenderer,
	logger *slog.Logger,
) BaseHandler {
	return BaseHandler{
		bot:             bot,
		usersService:    usersService,
		familiesService: familiesService,
		shoppingService: shoppingService,
		templateManager: templateManager,
		logger:          logger,
	}
}

// SendMessage sends a simple text message
func (bh *BaseHandler) SendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true

	if _, err := bh.bot.Send(msg); err != nil {
		bh.logger.Error("Failed to send message", "error", err, "chat_id", chatID)
	}
}

// SendMessageWithKeyboard sends a message with inline keyboard
func (bh *BaseHandler) SendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	if _, err := bh.bot.Send(msg); err != nil {
		bh.logger.Error("Failed to send message with keyboard", "error", err, "chat_id", chatID)
	}
}

// AnswerCallback answers an inline keyboard callback
func (bh *BaseHandler) AnswerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := bh.bot.Request(callback); err != nil {
		bh.logger.Error("Failed to answer callback", "error", err, "callback_id", callbackID)
	}
}
