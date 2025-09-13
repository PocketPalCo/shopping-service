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

// EditMessage edits an existing message with new text
func (bh *BaseHandler) EditMessage(chatID int64, messageID int, text string) {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.DisableWebPagePreview = true

	if _, err := bh.bot.Send(editMsg); err != nil {
		bh.logger.Error("Failed to edit message", "error", err, "chat_id", chatID, "message_id", messageID)
	}
}

// EditMessageWithKeyboard edits an existing message with new text and keyboard
func (bh *BaseHandler) EditMessageWithKeyboard(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &keyboard
	editMsg.DisableWebPagePreview = true

	if _, err := bh.bot.Send(editMsg); err != nil {
		bh.logger.Error("Failed to edit message with keyboard", "error", err, "chat_id", chatID, "message_id", messageID)
	}
}

// DeleteMessage deletes an existing message
func (bh *BaseHandler) DeleteMessage(chatID int64, messageID int) {
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	if _, err := bh.bot.Request(deleteMsg); err != nil {
		bh.logger.Error("Failed to delete message", "error", err, "chat_id", chatID, "message_id", messageID)
	}
}

// SendChatAction sends a chat action (typing indicator)
func (bh *BaseHandler) SendChatAction(chatID int64, action string) {
	chatAction := tgbotapi.NewChatAction(chatID, action)

	if _, err := bh.bot.Request(chatAction); err != nil {
		bh.logger.Error("Failed to send chat action", "error", err, "chat_id", chatID, "action", action)
	}
}

// SendLoadingMessage sends a loading message and returns the message ID for later deletion
func (bh *BaseHandler) SendLoadingMessage(chatID int64, text string) int {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true

	sentMsg, err := bh.bot.Send(msg)
	if err != nil {
		bh.logger.Error("Failed to send loading message", "error", err, "chat_id", chatID)
		return 0
	}
	return sentMsg.MessageID
}

// SendMessageWithKeyboardAndGetID sends a message with keyboard and returns the message ID
func (bh *BaseHandler) SendMessageWithKeyboardAndGetID(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) int {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	msg.DisableWebPagePreview = true

	sentMsg, err := bh.bot.Send(msg)
	if err != nil {
		bh.logger.Error("Failed to send message with keyboard", "error", err, "chat_id", chatID)
		return 0
	}
	return sentMsg.MessageID
}
