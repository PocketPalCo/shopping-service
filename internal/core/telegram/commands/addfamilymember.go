package commands

import (
	"context"
	"strconv"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	"github.com/google/uuid"
)

// AddFamilyMemberCommand handles the /addfamilymember command
type AddFamilyMemberCommand struct {
	BaseCommand
}

// NewAddFamilyMemberCommand creates a new addfamilymember command
func NewAddFamilyMemberCommand(base BaseCommand) *AddFamilyMemberCommand {
	return &AddFamilyMemberCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *AddFamilyMemberCommand) GetName() string {
	return "addfamilymember"
}

// RequiresAuth returns true as addfamilymember command requires authorization
func (c *AddFamilyMemberCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as addfamilymember command doesn't require admin privileges
func (c *AddFamilyMemberCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the addfamilymember command
func (c *AddFamilyMemberCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	if len(args) < 2 {
		// Show usage template
		message, err := c.templateManager.RenderTemplate("addfamilymember_usage", user.Locale, nil)
		if err != nil {
			c.logger.Error("Failed to render addfamilymember usage template", "error", err)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err
		}
		c.SendHTMLMessage(chatID, message)
		return nil
	}

	familyName := args[0]
	memberIdentifier := args[1] // Can be @username or telegram_id

	// Get user's families to find the family by name
	familiesInfo, err := c.familiesService.GetUserFamiliesWithInfo(ctx, user.ID)
	if err != nil {
		c.logger.Error("Failed to get user families", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_failed_to_retrieve_families", user.Locale))
		return err
	}

	// Find the family by name
	var targetFamilyID *uuid.UUID
	var userIsAdmin bool
	for _, info := range familiesInfo {
		if strings.EqualFold(info.Family.Name, familyName) {
			targetFamilyID = &info.Family.ID
			userIsAdmin = info.UserRole == "admin"
			break
		}
	}

	if targetFamilyID == nil {
		data := struct {
			FamilyName string
		}{
			FamilyName: familyName,
		}
		message, err := c.templateManager.RenderTemplate("family_not_found", user.Locale, data)
		if err != nil {
			c.logger.Error("Failed to render family not found template", "error", err)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err
		}
		c.SendHTMLMessage(chatID, message)
		return nil
	}

	// Check if user is admin of the family
	if !userIsAdmin {
		data := struct {
			FamilyName string
		}{
			FamilyName: familyName,
		}
		message, err := c.templateManager.RenderTemplate("family_admin_required", user.Locale, data)
		if err != nil {
			c.logger.Error("Failed to render admin required template", "error", err)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err
		}
		c.SendHTMLMessage(chatID, message)
		return nil
	}

	// Find the user to add
	var targetUser *users.User
	if strings.HasPrefix(memberIdentifier, "@") {
		// Username
		username := strings.TrimPrefix(memberIdentifier, "@")
		targetUser, err = c.usersService.GetUserByUsername(ctx, username)
		if err != nil {
			c.logger.Error("Failed to get user by username", "error", err, "username", username)
			data := struct {
				Username string
			}{
				Username: memberIdentifier,
			}
			message, err2 := c.templateManager.RenderTemplate("user_not_found_username", user.Locale, data)
			if err2 != nil {
				c.logger.Error("Failed to render user not found template", "error", err2)
				c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
				return err2
			}
			c.SendHTMLMessage(chatID, message)
			return err
		}
	} else {
		// Telegram ID
		telegramID, err := strconv.ParseInt(memberIdentifier, 10, 64)
		if err != nil {
			data := struct {
				Identifier string
			}{
				Identifier: memberIdentifier,
			}
			message, err2 := c.templateManager.RenderTemplate("invalid_user_identifier", user.Locale, data)
			if err2 != nil {
				c.logger.Error("Failed to render invalid identifier template", "error", err2)
				c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
				return err2
			}
			c.SendHTMLMessage(chatID, message)
			return err
		}

		targetUser, err = c.usersService.GetUserByTelegramID(ctx, telegramID)
		if err != nil {
			c.logger.Error("Failed to get user by telegram ID", "error", err, "telegram_id", telegramID)
			data := struct {
				TelegramID string
			}{
				TelegramID: memberIdentifier,
			}
			message, err2 := c.templateManager.RenderTemplate("user_not_found_telegram_id", user.Locale, data)
			if err2 != nil {
				c.logger.Error("Failed to render user not found template", "error", err2)
				c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
				return err2
			}
			c.SendHTMLMessage(chatID, message)
			return err
		}
	}

	if targetUser == nil {
		data := struct {
			Identifier string
		}{
			Identifier: memberIdentifier,
		}
		message, err := c.templateManager.RenderTemplate("user_not_found_general", user.Locale, data)
		if err != nil {
			c.logger.Error("Failed to render user not found template", "error", err)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err
		}
		c.SendHTMLMessage(chatID, message)
		return nil
	}

	// Add member to family
	err = c.familiesService.AddMemberToFamily(ctx, *targetFamilyID, targetUser.ID, user.ID)
	if err != nil {
		c.logger.Error("Failed to add member to family", "error", err, "family_id", *targetFamilyID, "user_id", targetUser.ID)

		// Check if it's a "user already member" error
		if strings.Contains(err.Error(), "already a member") {
			data := struct {
				UserName   string
				FamilyName string
			}{
				UserName:   GetUserDisplayName(targetUser),
				FamilyName: familyName,
			}
			message, err2 := c.templateManager.RenderTemplate("user_already_member", user.Locale, data)
			if err2 != nil {
				c.logger.Error("Failed to render already member template", "error", err2)
				c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
				return err2
			}
			c.SendHTMLMessage(chatID, message)
			return err
		}

		// General error
		message, err2 := c.templateManager.RenderTemplate("add_member_error", user.Locale, nil)
		if err2 != nil {
			c.logger.Error("Failed to render add member error template", "error", err2)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err2
		}
		c.SendHTMLMessage(chatID, message)
		return err
	}

	// Send success message to admin
	adminData := struct {
		UserName   string
		FamilyName string
	}{
		UserName:   GetUserDisplayName(targetUser),
		FamilyName: familyName,
	}

	message, err := c.templateManager.RenderTemplate("family_member_added", user.Locale, adminData)
	if err != nil {
		c.logger.Error("Failed to render member added template", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
		return err
	}
	c.SendHTMLMessage(chatID, message)

	// Send notification to the new member
	memberData := struct {
		FamilyName string
		AdminName  string
	}{
		FamilyName: familyName,
		AdminName:  GetUserDisplayName(user),
	}

	notificationMsg, err := c.templateManager.RenderTemplate("family_member_notification", targetUser.Locale, memberData)
	if err != nil {
		c.logger.Error("Failed to render member notification template", "error", err)
		// Don't fail the whole operation if notification fails
		c.logger.Warn("Skipping member notification due to template error")
	} else {
		c.SendHTMLMessage(targetUser.TelegramID, notificationMsg)
	}

	c.logger.Info("Family member added successfully",
		"family_id", *targetFamilyID,
		"family_name", familyName,
		"new_member_id", targetUser.ID,
		"new_member_telegram_id", targetUser.TelegramID,
		"added_by", user.TelegramID)

	return nil
}
