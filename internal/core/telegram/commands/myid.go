package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MyIDCommand handles the /myid command
type MyIDCommand struct {
	BaseCommand
}

// MyIDTemplateData holds data for the myid template
type MyIDTemplateData struct {
	TelegramID int64
	UserID     string
	FirstName  string
}

// NewMyIDCommand creates a new myid command
func NewMyIDCommand(base BaseCommand) *MyIDCommand {
	return &MyIDCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *MyIDCommand) GetName() string {
	return "myid"
}

// RequiresAuth returns false as myid command doesn't require authorization
func (c *MyIDCommand) RequiresAuth() bool {
	return false
}

// RequiresAdmin returns false as myid command doesn't require admin privileges
func (c *MyIDCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the myid command
func (c *MyIDCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	data := MyIDTemplateData{
		TelegramID: user.TelegramID,
		UserID:     user.ID.String(),
		FirstName:  user.FirstName,
	}

	message, err := c.templateManager.RenderTemplate("myid", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render myid template", "error", err)
		c.SendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return err
	}

	// Create keyboard with Main Menu button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))
	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}
