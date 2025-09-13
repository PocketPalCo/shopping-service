package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HelpCommand handles the /help command
type HelpCommand struct {
	BaseCommand
}

// HelpTemplateData holds data for the help template
type HelpTemplateData struct {
	IsAuthorized bool
	IsAdmin      bool
}

// NewHelpCommand creates a new help command
func NewHelpCommand(base BaseCommand) *HelpCommand {
	return &HelpCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *HelpCommand) GetName() string {
	return "help"
}

// RequiresAuth returns false as help command doesn't require authorization
func (c *HelpCommand) RequiresAuth() bool {
	return false
}

// RequiresAdmin returns false as help command doesn't require admin privileges
func (c *HelpCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the help command
func (c *HelpCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	data := HelpTemplateData{
		IsAuthorized: user.IsAuthorized,
		IsAdmin:      c.usersService.IsAdmin(user.TelegramID),
	}

	message, err := c.templateManager.RenderTemplate("help", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render help template", "error", err)
		c.SendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return err
	}

	// Create keyboard with Main Menu button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))

	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}
