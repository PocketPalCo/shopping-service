package commands

import (
	"context"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// CreateFamilyCommand handles the /createfamily command
type CreateFamilyCommand struct {
	BaseCommand
}

// NewCreateFamilyCommand creates a new createfamily command
func NewCreateFamilyCommand(base BaseCommand) *CreateFamilyCommand {
	return &CreateFamilyCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *CreateFamilyCommand) GetName() string {
	return "createfamily"
}

// RequiresAuth returns true as createfamily command requires authorization
func (c *CreateFamilyCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as createfamily command doesn't require admin privileges
func (c *CreateFamilyCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the createfamily command
func (c *CreateFamilyCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	if len(args) == 0 {
		// Show usage template
		message, err := c.templateManager.RenderTemplate("createfamily_usage", user.Locale, nil)
		if err != nil {
			c.logger.Error("Failed to render createfamily usage template", "error", err)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err
		}
		// Create keyboard with Main Menu button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))
		c.SendMessageWithKeyboard(chatID, message, keyboard)
		return nil
	}

	// Parse family name and optional description
	familyName := args[0]
	var description *string
	if len(args) > 1 {
		desc := strings.Join(args[1:], " ")
		description = &desc
	}

	// Validate family name
	if len(strings.TrimSpace(familyName)) == 0 {
		message, err := c.templateManager.RenderTemplate("createfamily_invalid_name", user.Locale, nil)
		if err != nil {
			c.logger.Error("Failed to render invalid name template", "error", err)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err
		}
		// Create keyboard with Main Menu button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))
		c.SendMessageWithKeyboard(chatID, message, keyboard)
		return nil
	}

	// Create family
	createReq := families.CreateFamilyRequest{
		Name:        familyName,
		Description: description,
		CreatedBy:   user.ID,
		MemberIDs:   []uuid.UUID{}, // Start with just the creator
	}

	family, err := c.familiesService.CreateFamily(ctx, createReq)
	if err != nil {
		c.logger.Error("Failed to create family", "error", err, "family_name", familyName, "created_by", user.ID)
		message, err2 := c.templateManager.RenderTemplate("createfamily_error", user.Locale, nil)
		if err2 != nil {
			c.logger.Error("Failed to render create family error template", "error", err2)
			c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
			return err2
		}
		// Create keyboard with Main Menu button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))
		c.SendMessageWithKeyboard(chatID, message, keyboard)
		return err
	}

	// Send success message
	data := struct {
		Name        string
		Description *string
		CreatedAt   time.Time
	}{
		Name:        family.Name,
		Description: family.Description,
		CreatedAt:   family.CreatedAt,
	}

	message, err := c.templateManager.RenderTemplate("family_created", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render family created template", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_internal", user.Locale))
		return err
	}

	// Create keyboard with Main Menu button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(CreateMainMenuButton(c.templateManager, user.Locale))
	c.SendMessageWithKeyboard(chatID, message, keyboard)

	c.logger.Info("Family created successfully",
		"family_id", family.ID,
		"family_name", family.Name,
		"created_by", user.TelegramID,
		"creator_name", user.FirstName)

	return nil
}
