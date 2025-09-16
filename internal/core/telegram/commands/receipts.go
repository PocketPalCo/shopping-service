package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ReceiptsCommand handles the /receipts command
type ReceiptsCommand struct {
	BaseCommand
}

// ReceiptsTemplateData holds data for the receipts template
type ReceiptsTemplateData struct {
	IsAuthorized bool
	UserName     string
}

// NewReceiptsCommand creates a new receipts command
func NewReceiptsCommand(base BaseCommand) *ReceiptsCommand {
	return &ReceiptsCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *ReceiptsCommand) GetName() string {
	return "receipts"
}

// RequiresAuth returns true as receipts command requires authorization
func (c *ReceiptsCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as receipts command doesn't require admin privileges
func (c *ReceiptsCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the receipts command
func (c *ReceiptsCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	c.logger.Info("Receipts command called",
		"user_id", user.ID,
		"telegram_id", user.TelegramID,
		"chat_id", chatID)

	data := ReceiptsTemplateData{
		IsAuthorized: user.IsAuthorized,
		UserName:     user.FirstName,
	}

	// Render receipts menu message
	message, err := c.templateManager.RenderTemplate("receipts_menu", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render receipts menu template", "error", err)
		c.SendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return err
	}

	// Create receipts menu keyboard
	keyboard := c.createReceiptsMenuKeyboard(user.Locale)

	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}

// createReceiptsMenuKeyboard creates the receipts menu inline keyboard
func (c *ReceiptsCommand) createReceiptsMenuKeyboard(locale string) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		// First row: Upload Receipt
		{
			tgbotapi.NewInlineKeyboardButtonData(
				c.templateManager.RenderButton("upload_receipt", locale),
				"receipts:upload",
			),
		},
		// Second row: View Receipts
		{
			tgbotapi.NewInlineKeyboardButtonData(
				c.templateManager.RenderButton("view_receipts", locale),
				"receipts:view",
			),
		},
		// Third row: Tax Summary and Statistics
		{
			tgbotapi.NewInlineKeyboardButtonData(
				c.templateManager.RenderButton("tax_summary", locale),
				"receipts:taxes",
			),
			tgbotapi.NewInlineKeyboardButtonData(
				c.templateManager.RenderButton("receipt_stats", locale),
				"receipts:stats",
			),
		},
		// Fourth row: Home
		{
			tgbotapi.NewInlineKeyboardButtonData(
				c.templateManager.RenderButton("home", locale),
				"menu_start",
			),
		},
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
