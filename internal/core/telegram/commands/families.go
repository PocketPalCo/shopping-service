package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// FamiliesCommand handles the /families command
type FamiliesCommand struct {
	BaseCommand
}

// FamilyDisplayData enriches family data for template display
type FamilyDisplayData struct {
	ID          string
	Name        string
	Description *string
	MemberCount int
	Role        string
}

// NewFamiliesCommand creates a new families command
func NewFamiliesCommand(base BaseCommand) *FamiliesCommand {
	return &FamiliesCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *FamiliesCommand) GetName() string {
	return "families"
}

// RequiresAuth returns true as families command requires authorization
func (c *FamiliesCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as families command doesn't require admin privileges
func (c *FamiliesCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the families command
func (c *FamiliesCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	familiesInfo, err := c.familiesService.GetUserFamiliesWithInfo(ctx, user.ID)
	if err != nil {
		c.logger.Error("Failed to get user families", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_failed_to_retrieve_families", user.Locale))
		return err
	}

	// Transform to display-friendly data
	var displayFamilies []FamilyDisplayData
	for _, info := range familiesInfo {
		displayData := FamilyDisplayData{
			ID:          info.Family.ID.String(),
			Name:        info.Family.Name,
			Description: info.Family.Description,
			MemberCount: info.MemberCount,
			Role:        info.UserRole,
		}
		displayFamilies = append(displayFamilies, displayData)
	}

	data := struct {
		Families []FamilyDisplayData
	}{
		Families: displayFamilies,
	}

	message, err := c.templateManager.RenderTemplate("families_list", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render families list template", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
		return err
	}

	// Create keyboard with Main Menu button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))

	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}
