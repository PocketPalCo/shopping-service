package commands

import (
	"context"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// StatusCommand handles the /status command
type StatusCommand struct {
	BaseCommand
}

// StatusTemplateData holds data for the status template
type StatusTemplateData struct {
	FirstName    string
	LastName     string
	Username     string
	TelegramID   int64
	IsAuthorized bool
	AuthorizedAt *time.Time
}

// NewStatusCommand creates a new status command
func NewStatusCommand(base BaseCommand) *StatusCommand {
	return &StatusCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *StatusCommand) GetName() string {
	return "status"
}

// RequiresAuth returns true as status command requires authorization
func (c *StatusCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as status command doesn't require admin privileges
func (c *StatusCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the status command
func (c *StatusCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	var lastName, username string
	if user.LastName != nil {
		lastName = *user.LastName
	}
	if user.Username != nil {
		username = *user.Username
	}

	data := StatusTemplateData{
		FirstName:    user.FirstName,
		LastName:     lastName,
		Username:     username,
		TelegramID:   user.TelegramID,
		IsAuthorized: user.IsAuthorized,
		AuthorizedAt: user.AuthorizedAt,
	}

	message, err := c.templateManager.RenderTemplate("status", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render status template", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
		return err
	}

	c.SendHTMLMessage(chatID, message)
	return nil
}
