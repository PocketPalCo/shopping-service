package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CreateListCommand handles the /createlist command
type CreateListCommand struct {
	BaseCommand
}

// NewCreateListCommand creates a new createlist command
func NewCreateListCommand(base BaseCommand) *CreateListCommand {
	return &CreateListCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *CreateListCommand) GetName() string {
	return "createlist"
}

// RequiresAuth returns true as createlist command requires authorization
func (c *CreateListCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as createlist command doesn't require admin privileges
func (c *CreateListCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the createlist command
func (c *CreateListCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	// Get user's families
	families, err := c.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		c.logger.Error("Failed to get user families", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_failed_to_retrieve_families", user.Locale))
		return err
	}

	if len(families) == 0 {
		message, err := c.templateManager.RenderTemplate("createlist_no_families", user.Locale, nil)
		if err != nil {
			c.logger.Error("Failed to render no families template", "error", err)
			message = "❌ You need to be part of a family to create shopping lists.\n\nUse /createfamily to create a family first."
		}
		// Create keyboard with Main Menu button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))
		c.SendMessageWithKeyboard(chatID, message, keyboard)
		return nil
	}

	// Create buttons for each family
	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, family := range families {
		button := tgbotapi.NewInlineKeyboardButtonData(
			"🏠 "+family.Name,
			"createlist_"+family.ID.String(),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add Main Menu button
	buttons = append(buttons, CreateMainMenuButton(c.templateManager, user.Locale))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	data := struct {
		FamilyCount int
	}{
		FamilyCount: len(families),
	}

	message, err := c.templateManager.RenderTemplate("createlist_family_selection", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render family selection template", "error", err)
		message = "📋 <b>Create New Shopping List</b>\n\nSelect the family for your new shopping list:"
	}

	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}
