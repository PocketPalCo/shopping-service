package commands

import (
	"context"
	"strconv"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// RevokeCommand handles the /revoke command (admin only)
type RevokeCommand struct {
	BaseCommand
}

// NewRevokeCommand creates a new revoke command
func NewRevokeCommand(base BaseCommand) *RevokeCommand {
	return &RevokeCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *RevokeCommand) GetName() string {
	return "revoke"
}

// RequiresAuth returns true as revoke command requires authorization
func (c *RevokeCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns true as revoke command requires admin privileges
func (c *RevokeCommand) RequiresAdmin() bool {
	return true
}

// Handle executes the revoke command
func (c *RevokeCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	if len(args) == 0 {
		c.SendMessage(chatID, "❌ Usage: /revoke <telegram_id>")
		return nil
	}

	telegramIDStr := args[0]
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		c.SendMessage(chatID, "❌ Invalid Telegram ID. Please provide a valid number.")
		return nil
	}

	// Get the user to revoke
	targetUser, err := c.usersService.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		c.logger.Error("Failed to get user by telegram ID", "error", err, "telegram_id", telegramID)
		c.SendMessage(chatID, "❌ User not found.")
		return err
	}

	// Check if user is already not authorized
	if !targetUser.IsAuthorized {
		displayName := GetUserDisplayName(targetUser)
		c.SendMessage(chatID, "✅ User "+displayName+" is already not authorized.")
		return nil
	}

	// Prevent self-revocation
	if targetUser.TelegramID == user.TelegramID {
		c.SendMessage(chatID, "❌ You cannot revoke your own authorization.")
		return nil
	}

	// Revoke the user's authorization
	err = c.usersService.RevokeUser(ctx, targetUser.TelegramID)
	if err != nil {
		c.logger.Error("Failed to revoke user", "error", err, "target_user_id", targetUser.ID, "admin_user_id", user.ID)
		c.SendMessage(chatID, "❌ Failed to revoke user authorization.")
		return err
	}

	displayName := GetUserDisplayName(targetUser)
	adminName := GetUserDisplayName(user)

	// Send confirmation to admin
	c.SendMessage(chatID, "✅ User "+displayName+" authorization has been revoked!")

	// Send notification to the revoked user
	c.SendMessage(targetUser.TelegramID, "⚠️ Your authorization to use this bot has been revoked by "+adminName+".\n\nYou no longer have access to bot features. Contact an administrator if you believe this is an error.")

	c.logger.Info("User authorization revoked",
		"target_user_id", targetUser.TelegramID,
		"target_user_name", displayName,
		"revoked_by", adminName,
		"admin_id", user.TelegramID)

	return nil
}
