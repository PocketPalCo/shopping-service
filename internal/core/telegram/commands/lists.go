package commands

import (
	"context"
	"fmt"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ListsCommand handles the /lists command
type ListsCommand struct {
	BaseCommand
}

// NewListsCommand creates a new lists command
func NewListsCommand(base BaseCommand) *ListsCommand {
	return &ListsCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *ListsCommand) GetName() string {
	return "lists"
}

// RequiresAuth returns true as lists command requires authorization
func (c *ListsCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns false as lists command doesn't require admin privileges
func (c *ListsCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the lists command
func (c *ListsCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	listsWithFamilies, familyCount, err := c.shoppingService.GetUserShoppingListsWithFamilies(ctx, user.ID)
	if err != nil {
		c.logger.Error("Failed to get user shopping lists with families", "error", err)
		c.SendMessage(chatID, c.templateManager.RenderMessage("error_failed_to_retrieve_lists", user.Locale))
		return err
	}

	if len(listsWithFamilies) == 0 {
		message, err := c.templateManager.RenderTemplate("no_lists", user.Locale, nil)
		if err != nil {
			c.logger.Error("Failed to render no lists template", "error", err)
			c.SendMessage(chatID, "📝 You don't have any shopping lists yet.")
			return err
		}
		c.SendHTMLMessage(chatID, message)
		return nil
	}

	// Create buttons for each list
	var buttons [][]tgbotapi.InlineKeyboardButton
	showFamilyNames := familyCount > 1 // Only show family names if user has more than 1 family

	for _, listWithFamily := range listsWithFamilies {
		var buttonText string
		if showFamilyNames && listWithFamily.FamilyName != nil {
			buttonText = fmt.Sprintf("📋 %s 🏠 %s", listWithFamily.Name, *listWithFamily.FamilyName)
		} else {
			buttonText = fmt.Sprintf("📋 %s", listWithFamily.Name)
		}

		button := tgbotapi.NewInlineKeyboardButtonData(
			buttonText,
			fmt.Sprintf("list_view_%s", listWithFamily.ID.String()),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add create new list button
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("create_new_list", user.Locale), "create_new_list"),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	data := struct {
		ListCount int
	}{
		ListCount: len(listsWithFamilies),
	}

	message, err := c.templateManager.RenderTemplate("lists_overview", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render lists overview template", "error", err)
		message = fmt.Sprintf("📝 <b>Your Shopping Lists (%d)</b>\n\nSelect a list to view or manage:", len(listsWithFamilies))
	}

	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}
