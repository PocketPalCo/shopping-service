package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// UsersCommand handles the /users command (admin only)
type UsersCommand struct {
	BaseCommand
}

// NewUsersCommand creates a new users command
func NewUsersCommand(base BaseCommand) *UsersCommand {
	return &UsersCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *UsersCommand) GetName() string {
	return "users"
}

// RequiresAuth returns true as users command requires authorization
func (c *UsersCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns true as users command requires admin privileges
func (c *UsersCommand) RequiresAdmin() bool {
	return true
}

// Handle executes the users command
func (c *UsersCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	userList, err := c.usersService.GetAllUsers(ctx)
	if err != nil {
		c.logger.Error("Failed to get all users", "error", err)
		c.SendMessage(chatID, "❌ Failed to retrieve users.")
		return err
	}

	data := struct {
		Users interface{}
	}{
		Users: userList,
	}

	message, err := c.templateManager.RenderTemplate("users_list", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render users list template", "error", err)
		c.SendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return err
	}

	c.SendHTMLMessage(chatID, message)
	return nil
}
