package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// StartCommand handles the /start command
type StartCommand struct {
	BaseCommand
}

// StartTemplateData holds data for the start template
type StartTemplateData struct {
	FirstName    string
	IsAuthorized bool
}

// NewStartCommand creates a new start command
func NewStartCommand(base BaseCommand) *StartCommand {
	return &StartCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *StartCommand) GetName() string {
	return "start"
}

// RequiresAuth returns false as start command doesn't require authorization
func (c *StartCommand) RequiresAuth() bool {
	return false
}

// RequiresAdmin returns false as start command doesn't require admin privileges
func (c *StartCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the start command
func (c *StartCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	data := StartTemplateData{
		FirstName:    getUserDisplayName(user),
		IsAuthorized: user.IsAuthorized,
	}

	message, err := c.templateManager.RenderTemplate("start", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render start template", "error", err)
		c.SendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return err
	}

	c.SendHTMLMessage(chatID, message)
	return nil
}
