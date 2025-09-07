package handlers

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ListCallbackHandler handles shopping list related callbacks
type ListCallbackHandler struct {
	BaseHandler
}

// NewListCallbackHandler creates a new list callback handler
func NewListCallbackHandler(base BaseHandler) *ListCallbackHandler {
	return &ListCallbackHandler{
		BaseHandler: base,
	}
}

// truncateUTF8 safely truncates a UTF-8 string to maxRunes characters
// and adds "..." if truncated
func truncateUTF8(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	// Leave space for "..."
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}

	return string(runes[:maxRunes-3]) + "..."
}

// HandleViewList shows a shopping list with its items and action buttons
func (h *ListCallbackHandler) HandleViewList(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, user *users.User) {
	// Get the shopping list
	list, err := h.shoppingService.GetShoppingListByID(ctx, listID)
	if err != nil || list == nil {
		h.logger.Error("Failed to get shopping list", "error", err, "list_id", listID)
		h.AnswerCallback(callback.ID, "❌ List not found.")
		return
	}

	// Get list items
	items, err := h.shoppingService.GetListItems(ctx, listID)
	if err != nil {
		h.logger.Error("Failed to get list items", "error", err, "list_id", listID)
		h.AnswerCallback(callback.ID, "❌ Failed to load list items.")
		return
	}

	// Build the message
	message := fmt.Sprintf("📋 <b>%s</b>\n", list.Name)

	if len(items) == 0 {
		message += "\n<i>No items yet. Add some!</i>\n"
	} else {
		message += fmt.Sprintf("\n<b>Items (%d):</b>\n", len(items))

		for i, item := range items {
			status := "☐" // Incomplete
			if item.IsCompleted {
				status = "✅" // Complete
			}

			// Use display_name if available (original user input), otherwise fall back to name
			displayText := item.Name
			if item.DisplayName != nil && *item.DisplayName != "" {
				displayText = *item.DisplayName
			}

			itemText := fmt.Sprintf("%s <b>%s</b>", status, displayText)

			// Show parsing status for debugging (can be removed in production)
			if item.ParsingStatus == "parsed" && item.ParsedName != nil && *item.ParsedName != displayText {
				itemText += fmt.Sprintf(" <i>(→ %s)</i>", *item.ParsedName)
			} else if item.Quantity != nil && *item.Quantity != "" {
				// Smart quantity display - don't show "0 pieces" or similar zero quantities
				quantity := strings.TrimSpace(*item.Quantity)
				if !strings.HasPrefix(quantity, "0 ") && quantity != "0" {
					itemText += fmt.Sprintf(" <i>(%s)</i>", quantity)
				}
			}

			message += fmt.Sprintf("%d. %s\n", i+1, itemText)
		}
	}

	// Create action buttons
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("add_item", user.Locale), fmt.Sprintf("list_additem_%s", listID.String())),
		},
	}

	// Add item management buttons if there are items
	if len(items) > 0 {
		itemButtons := []tgbotapi.InlineKeyboardButton{}
		for _, item := range items {
			itemName := item.Name
			if item.ParsedName != nil && *item.ParsedName != "" {
				itemName = *item.ParsedName
			}

			// Add completion status indicator to button text
			var buttonText string
			if item.IsCompleted {
				buttonText = "✅ " + truncateUTF8(itemName, 16)
			} else {
				buttonText = truncateUTF8(itemName, 17)
			}

			shortListID := listID.String()[:8]  // First 8 chars of UUID
			shortItemID := item.ID.String()[:8] // First 8 chars of UUID
			button := tgbotapi.NewInlineKeyboardButtonData(
				buttonText,
				fmt.Sprintf("li_t_%s_%s", shortListID, shortItemID),
			)
			itemButtons = append(itemButtons, button)

			if len(itemButtons) == 2 {
				buttons = append(buttons, itemButtons)
				itemButtons = []tgbotapi.InlineKeyboardButton{}
			}
		}

		if len(itemButtons) > 0 {
			buttons = append(buttons, itemButtons)
		}
	}

	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("refresh", user.Locale), fmt.Sprintf("list_view_%s", listID.String())),
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("all_lists", user.Locale), "show_all_lists"),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Edit the original message
	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		message,
	)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &keyboard

	if _, err := h.bot.Send(editMsg); err != nil {
		// Check if error is due to message not being modified (content is the same)
		if strings.Contains(err.Error(), "message is not modified") {
			h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("success_list_up_to_date", user.Locale))
			return
		}
		h.logger.Error("Failed to edit list view message", "error", err)
		h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("error_failed_to_update_list_view", user.Locale))
		return
	}

	h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("success_list_updated", user.Locale))
}

// HandleAddItem prepares to add an item to a list
func (h *ListCallbackHandler) HandleAddItem(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, user *users.User) {
	// Get the shopping list to verify access
	list, err := h.shoppingService.GetShoppingListByID(ctx, listID)
	if err != nil || list == nil {
		h.logger.Error("Failed to get shopping list", "error", err, "list_id", listID)
		h.AnswerCallback(callback.ID, "❌ List not found.")
		return
	}

	h.AnswerCallback(callback.ID, "Ready to add item!")

	// Render template for add item prompt
	data := struct {
		ListName string
	}{
		ListName: list.Name,
	}

	message, err := h.templateManager.RenderTemplate("add_item_prompt", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render add item prompt template", "error", err)
		message = fmt.Sprintf("➕ <b>Add Item to: %s</b>\n\nPlease reply with the item name and optional quantity.\n\n<b>Examples:</b>\n• Milk\n• Bread (2 loaves)\n• Apples 2kg", list.Name)
	}

	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		message,
	)
	editMsg.ParseMode = tgbotapi.ModeHTML

	// Add cancel button
	cancelButton := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("cancel", user.Locale), fmt.Sprintf("list_view_%s", listID.String())),
		},
	}
	cancelKeyboard := tgbotapi.NewInlineKeyboardMarkup(cancelButton...)
	editMsg.ReplyMarkup = &cancelKeyboard

	if _, err := h.bot.Send(editMsg); err != nil {
		h.logger.Error("Failed to edit message for add item", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to prepare add item workflow.")
		return
	}

	// Store state that user is adding item to this list - this would need to be implemented
	// For now, we'll leave this as a TODO since the state management is in the bot service
	h.logger.Info("User entered add item state", "user_id", user.TelegramID, "list_id", listID)
}

// HandleToggleItem toggles an item's completion status
func (h *ListCallbackHandler) HandleToggleItem(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, itemIDStr string, user *users.User) {
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		h.logger.Error("Failed to parse item ID", "error", err, "item_id", itemIDStr)
		h.AnswerCallback(callback.ID, "❌ Invalid item ID.")
		return
	}

	// Get the item to check current status
	items, err := h.shoppingService.GetListItems(ctx, listID)
	if err != nil {
		h.logger.Error("Failed to get list items", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to load items.")
		return
	}

	var targetItem *shopping.ShoppingItem
	for _, item := range items {
		if item.ID == itemID {
			targetItem = item
			break
		}
	}

	if targetItem == nil {
		h.AnswerCallback(callback.ID, "❌ Item not found.")
		return
	}

	// Toggle the item status
	if targetItem.IsCompleted {
		err = h.shoppingService.UncompleteItem(ctx, itemID)
		if err != nil {
			h.logger.Error("Failed to uncomplete item", "error", err)
			h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("error_failed_to_uncomplete_item", user.Locale))
			return
		}
		h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("success_item_unmarked", user.Locale))
	} else {
		err = h.shoppingService.CompleteItem(ctx, itemID, user.ID)
		if err != nil {
			h.logger.Error("Failed to complete item", "error", err)
			h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("error_failed_to_complete_item", user.Locale))
			return
		}
		h.AnswerCallback(callback.ID, h.templateManager.RenderMessage("success_item_completed", user.Locale))
	}

	// Refresh the list view
	h.HandleViewList(ctx, callback, listID, user)
}
