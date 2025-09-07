package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// MessageHandler handles different types of messages
type MessageHandler struct {
	BaseHandler
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(base BaseHandler) *MessageHandler {
	return &MessageHandler{
		BaseHandler: base,
	}
}

// HandleShoppingListNameInput processes user input when naming a new shopping list
func (h *MessageHandler) HandleShoppingListNameInput(ctx context.Context, message *tgbotapi.Message, user *users.User, familyIDStr string) {
	listName := strings.TrimSpace(message.Text)

	if listName == "" || len(listName) < 2 {
		errorMsg, err := h.templateManager.RenderTemplate("createlist_invalid_name", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render invalid name template", "error", err)
			errorMsg = "❌ List name is too short. Please provide a name with at least 2 characters."
		}
		h.SendMessage(message.Chat.ID, errorMsg)
		return
	}

	if len(listName) > 100 {
		h.SendMessage(message.Chat.ID, "❌ List name is too long. Please keep it under 100 characters.")
		return
	}

	// Parse family ID
	familyID, err := uuid.Parse(familyIDStr)
	if err != nil {
		h.logger.Error("Failed to parse family ID from state", "error", err, "family_id", familyIDStr)
		h.SendMessage(message.Chat.ID, "❌ Something went wrong. Please try again.")
		return
	}

	// Verify user has access to this family
	canAccess, err := h.familiesService.IsUserFamilyMember(ctx, familyID, user.ID)
	if err != nil {
		h.logger.Error("Failed to check family access", "error", err)
		h.SendMessage(message.Chat.ID, "❌ Failed to verify family access.")
		return
	}

	if !canAccess {
		h.SendMessage(message.Chat.ID, "❌ You don't have access to this family.")
		return
	}

	// Get family details
	family, err := h.familiesService.GetFamilyByID(ctx, familyID)
	if err != nil {
		h.logger.Error("Failed to get family", "error", err, "family_id", familyID)
		h.SendMessage(message.Chat.ID, "❌ Family not found.")
		return
	}

	// Create the shopping list
	req := shopping.CreateShoppingListRequest{
		Name:     listName,
		OwnerID:  user.ID,
		FamilyID: &familyID,
		IsShared: true,
	}
	shoppingList, err := h.shoppingService.CreateShoppingList(ctx, req)
	if err != nil {
		h.logger.Error("Failed to create shopping list", "error", err)

		if strings.Contains(err.Error(), "already exists") {
			data := struct {
				ListName   string
				FamilyName string
			}{
				ListName:   listName,
				FamilyName: family.Name,
			}

			errorMessage, templateErr := h.templateManager.RenderTemplate("createlist_error", user.Locale, data)
			if templateErr != nil {
				h.logger.Error("Failed to render error template", "error", templateErr)
				errorMessage = fmt.Sprintf("❌ A shopping list named '%s' already exists for family '%s'.", listName, family.Name)
			}
			h.SendMessage(message.Chat.ID, errorMessage)
		} else {
			h.SendMessage(message.Chat.ID, "❌ Failed to create shopping list. Please try again.")
		}
		return
	}

	// Clear user state - this would need to be implemented
	h.logger.Info("Shopping list created successfully",
		"list_id", shoppingList.ID,
		"list_name", shoppingList.Name,
		"family_id", familyID,
		"created_by", user.TelegramID)

	data := struct {
		ListName   string
		FamilyName string
		CreatedAt  string
	}{
		ListName:   shoppingList.Name,
		FamilyName: family.Name,
		CreatedAt:  shoppingList.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	successMessage, err := h.templateManager.RenderTemplate("list_created_success", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render list created success template", "error", err)
		successMessage = fmt.Sprintf("✅ Shopping List Created!\n\n📋 Name: %s\n🏠 Family: %s", shoppingList.Name, family.Name)
	}

	// Create action buttons for the new list
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("➕ Add Item", fmt.Sprintf("list_additem_%s", shoppingList.ID.String())),
			tgbotapi.NewInlineKeyboardButtonData("📋 View List", fmt.Sprintf("list_view_%s", shoppingList.ID.String())),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("📝 All Lists", "show_all_lists"),
		},
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	h.SendMessageWithKeyboard(message.Chat.ID, successMessage, keyboard)

	// TODO: Notify family members about the new list
	// This would require implementing the notification system
}

// HandleAddItemInput processes user input when adding an item to a shopping list
func (h *MessageHandler) HandleAddItemInput(ctx context.Context, message *tgbotapi.Message, user *users.User, listIDStr string) {
	itemText := strings.TrimSpace(message.Text)

	if itemText == "" {
		h.SendMessage(message.Chat.ID, "❌ Please provide an item name.")
		return
	}

	// Parse list ID
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		h.logger.Error("Failed to parse list ID from state", "error", err, "list_id", listIDStr)
		h.SendMessage(message.Chat.ID, "❌ Something went wrong. Please try again.")
		return
	}

	// Check if user can access this list
	canAccess, err := h.shoppingService.CanUserAccessList(ctx, listID, user.ID)
	if err != nil {
		h.logger.Error("Failed to check list access", "error", err)
		h.SendMessage(message.Chat.ID, "❌ Failed to verify list access.")
		return
	}

	if !canAccess {
		h.SendMessage(message.Chat.ID, "❌ You don't have access to this list.")
		return
	}

	// Use AI to intelligently parse and separate multiple items
	addedItems, failedItems, err := h.shoppingService.AddItemsToListWithAI(ctx, listID, itemText, user.Locale, user.ID)
	if err != nil {
		h.logger.Error("Failed to add items to list with AI", "error", err)
		h.SendMessage(message.Chat.ID, "❌ Failed to add items. Please try again.")
		return
	}

	// TODO: Notify family members about each new item
	// This would require implementing the notification system

	// TODO: Clear user state
	// This would need to be implemented since state management is in the bot service

	// Prepare success message based on results
	var successText string
	if len(addedItems) > 0 {
		if len(addedItems) == 1 {
			successText = fmt.Sprintf("✅ <b>Item Added!</b>\n\n➕ <b>%s</b>", addedItems[0].Name)
			if addedItems[0].Quantity != nil && *addedItems[0].Quantity != "" {
				quantity := strings.TrimSpace(*addedItems[0].Quantity)
				if !strings.HasPrefix(quantity, "0 ") && quantity != "0" {
					successText += fmt.Sprintf(" <i>(%s)</i>", quantity)
				}
			}
		} else {
			successText = fmt.Sprintf("✅ <b>%d Items Added!</b>\n\n", len(addedItems))
			for _, item := range addedItems {
				successText += fmt.Sprintf("➕ <b>%s</b>", item.Name)
				if item.Quantity != nil && *item.Quantity != "" {
					quantity := strings.TrimSpace(*item.Quantity)
					if !strings.HasPrefix(quantity, "0 ") && quantity != "0" {
						successText += fmt.Sprintf(" <i>(%s)</i>", quantity)
					}
				}
				successText += "\n"
			}
		}
	}

	if len(failedItems) > 0 {
		if len(addedItems) > 0 {
			successText += "\n❌ <b>Failed to add:</b> " + strings.Join(failedItems, ", ")
		} else {
			successText = "❌ <b>Failed to add items:</b> " + strings.Join(failedItems, ", ") + "\n\nPlease try again."
		}
	}

	if successText == "" {
		successText = "❌ No items were added. Please try again."
	}

	// Create action buttons
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("➕ Add Another", fmt.Sprintf("list_additem_%s", listID.String())),
			tgbotapi.NewInlineKeyboardButtonData("📋 View List", fmt.Sprintf("list_view_%s", listID.String())),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("📝 All Lists", "show_all_lists"),
		},
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	h.SendMessageWithKeyboard(message.Chat.ID, successText, keyboard)
}

// HandleAuthorizedMessage processes messages from authorized users
func (h *MessageHandler) HandleAuthorizedMessage(ctx context.Context, message *tgbotapi.Message, user *users.User) {
	// TODO: Check for user states and delegate to appropriate handlers
	// For now, just acknowledge the message
	h.SendMessage(message.Chat.ID, "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands.")
}

// SendUnauthorizedMessage sends a message to unauthorized users
func (h *MessageHandler) SendUnauthorizedMessage(chatID int64, user *users.User) {
	displayName := getUserDisplayName(user)
	message := fmt.Sprintf(`🔒 Access Denied

Hello %s! You need to be authorized to use this bot.

Your Telegram ID: %d

Please contact an administrator to request access.

Available commands: /start, /help, /status`,
		displayName, user.TelegramID)

	h.SendMessage(chatID, message)
}

// getUserDisplayName extracts display name from user (helper function)
func getUserDisplayName(user *users.User) string {
	if user.FirstName != "" {
		if user.LastName != nil && *user.LastName != "" {
			return user.FirstName + " " + *user.LastName
		}
		return user.FirstName
	}
	if user.Username != nil && *user.Username != "" {
		return "@" + *user.Username
	}
	return "User_" + strconv.FormatInt(user.TelegramID, 10)
}
