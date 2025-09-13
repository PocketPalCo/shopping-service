package commands

import (
	"context"
	"strconv"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// AuthorizeCommand handles the /authorize command (admin only)
type AuthorizeCommand struct {
	BaseCommand
}

// NewAuthorizeCommand creates a new authorize command
func NewAuthorizeCommand(base BaseCommand) *AuthorizeCommand {
	return &AuthorizeCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *AuthorizeCommand) GetName() string {
	return "authorize"
}

// RequiresAuth returns true as authorize command requires authorization
func (c *AuthorizeCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns true as authorize command requires admin privileges
func (c *AuthorizeCommand) RequiresAdmin() bool {
	return true
}

// Handle executes the authorize command
func (c *AuthorizeCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	if len(args) == 0 {
		c.SendMessage(chatID, "❌ Usage: /authorize <telegram_id>")
		return nil
	}

	telegramIDStr := args[0]
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		c.SendMessage(chatID, "❌ Invalid Telegram ID. Please provide a valid number.")
		return nil
	}

	// Get the user to authorize
	targetUser, err := c.usersService.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		c.logger.Error("Failed to get user by telegram ID", "error", err, "telegram_id", telegramID)
		c.SendMessage(chatID, "❌ User not found.")
		return err
	}

	// Check if user is already authorized
	if targetUser.IsAuthorized {
		displayName := GetUserDisplayName(targetUser)
		c.SendMessage(chatID, "✅ User "+displayName+" is already authorized.")
		return nil
	}

	// Authorize the user
	err = c.usersService.AuthorizeUser(ctx, targetUser.TelegramID)
	if err != nil {
		c.logger.Error("Failed to authorize user", "error", err, "target_user_id", targetUser.ID, "admin_user_id", user.ID)
		c.SendMessage(chatID, "❌ Failed to authorize user.")
		return err
	}

	displayName := GetUserDisplayName(targetUser)
	adminName := GetUserDisplayName(user)

	// Send confirmation to admin
	c.SendMessage(chatID, "✅ User "+displayName+" has been authorized!")

	// Send notification to the authorized user
	c.SendMessage(targetUser.TelegramID, "🎉 Great news! You have been authorized to use this bot by "+adminName+".\n\nYou can now access all features. Type /help to see available commands.")

	c.logger.Info("User authorized",
		"target_user_id", targetUser.TelegramID,
		"target_user_name", displayName,
		"authorized_by", adminName,
		"admin_id", user.TelegramID)

	return nil
}
