package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// DuplicateCallbackHandler handles duplicate item resolution callbacks
type DuplicateCallbackHandler struct {
	BaseHandler
	stateManager *StateManager
}

// NewDuplicateCallbackHandler creates a new duplicate callback handler
func NewDuplicateCallbackHandler(base BaseHandler, stateManager *StateManager) *DuplicateCallbackHandler {
	return &DuplicateCallbackHandler{
		BaseHandler:  base,
		stateManager: stateManager,
	}
}

// HandleDuplicateCallback handles all duplicate resolution callbacks
func (h *DuplicateCallbackHandler) HandleDuplicateCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	// Callback format: dup_action_index_listId or dup_globalaction_listId
	// Examples: dup_keep_0_acb1f108, dup_replace_0_acb1f108, dup_keepall_acb1f108

	if len(parts) < 3 {
		h.AnswerCallback(callback.ID, "❌ Invalid duplicate callback.")
		return
	}

	action := parts[1] // keep, replace, both, keepall, replaceall, cancel

	switch action {
	case "cancel":
		h.handleCancel(ctx, callback, user)

	case "keepall":
		h.handleKeepAll(ctx, callback, parts, user)

	case "replaceall":
		h.handleReplaceAll(ctx, callback, parts, user)

	case "keep", "replace", "both":
		if len(parts) < 4 {
			h.AnswerCallback(callback.ID, "❌ Invalid callback format.")
			return
		}
		h.handleItemAction(ctx, callback, parts, user, action)

	default:
		h.AnswerCallback(callback.ID, "❌ Unknown duplicate action.")
	}
}

func (h *DuplicateCallbackHandler) handleCancel(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.AnswerCallback(callback.ID, "Operation cancelled.")

	// Clear any pending duplicate data and item addition state
	h.stateManager.ClearUserState(user.TelegramID, "duplicate_resolution")
	h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")

	// Delete the duplicate resolution message
	deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
	if _, err := h.bot.Request(deleteMsg); err != nil {
		h.logger.Error("Failed to delete duplicate resolution message", "error", err)
	}
}

func (h *DuplicateCallbackHandler) handleKeepAll(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	h.AnswerCallback(callback.ID, "Keeping all existing items.")

	// Clear state and item addition state
	h.stateManager.ClearUserState(user.TelegramID, "duplicate_resolution")
	h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")

	// Delete the duplicate resolution message
	deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
	if _, err := h.bot.Request(deleteMsg); err != nil {
		h.logger.Error("Failed to delete duplicate resolution message", "error", err)
	}
}

func (h *DuplicateCallbackHandler) handleReplaceAll(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 3 {
		h.AnswerCallback(callback.ID, "❌ Invalid callback format.")
		return
	}

	// Extract short list ID from callback
	shortListID := parts[2]

	// Get duplicate resolution state
	stateData, hasState := h.stateManager.GetUserState(user.TelegramID, "duplicate_resolution")
	if !hasState {
		h.AnswerCallback(callback.ID, "❌ Resolution data expired. Please try again.")
		return
	}

	// Parse stored duplicate data
	var duplicateState DuplicateState
	if err := json.Unmarshal([]byte(stateData), &duplicateState); err != nil {
		h.logger.Error("Failed to parse duplicate state", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to process data.")
		return
	}

	// Verify list ID matches
	if !strings.HasPrefix(duplicateState.ListID.String(), shortListID) {
		h.AnswerCallback(callback.ID, "❌ List ID mismatch.")
		return
	}

	// Replace all duplicate items
	successCount := 0
	errorCount := 0

	for _, duplicate := range duplicateState.Duplicates {
		// Update existing item with new quantity/info from parsed item
		updateReq := shopping.UpdateShoppingItemRequest{
			Name:     &duplicate.ParsedName,
			Quantity: duplicate.NewQuantity,
		}

		if err := h.shoppingService.UpdateShoppingItem(ctx, duplicate.ExistingItem.ID, updateReq, user.ID); err != nil {
			h.logger.Error("Failed to update item", "error", err, "item_id", duplicate.ExistingItem.ID)
			errorCount++
		} else {
			successCount++
		}
	}

	// Add any unique items that weren't duplicates
	if len(duplicateState.UniqueItems) > 0 {
		addedItems, failedItems, err := h.shoppingService.AddParsedItemsToList(ctx, duplicateState.ListID, duplicateState.UniqueItems, user.ID)
		if err != nil {
			h.logger.Error("Failed to add unique items", "error", err)
		} else {
			successCount += len(addedItems)
			errorCount += len(failedItems)
		}
	}

	// Prepare response message
	var responseText string
	if successCount > 0 && errorCount == 0 {
		responseText = fmt.Sprintf("✅ Replaced all items successfully! (%d items)", successCount)
	} else if successCount > 0 && errorCount > 0 {
		responseText = fmt.Sprintf("⚠️ Replaced %d items, but %d failed.", successCount, errorCount)
	} else {
		responseText = "❌ Failed to replace items. Please try again."
	}

	h.AnswerCallback(callback.ID, responseText)

	// Clear state and item addition state
	h.stateManager.ClearUserState(user.TelegramID, "duplicate_resolution")
	h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")

	// Delete the duplicate resolution message
	deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
	if _, err := h.bot.Request(deleteMsg); err != nil {
		h.logger.Error("Failed to delete duplicate resolution message", "error", err)
	}

	// Show the updated list
	h.showUpdatedList(ctx, callback.Message.Chat.ID, duplicateState.ListID, user)
}

func (h *DuplicateCallbackHandler) handleItemAction(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User, action string) {
	// Parse index and list ID
	indexStr := parts[2]
	shortListID := parts[3]

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		h.AnswerCallback(callback.ID, "❌ Invalid item index.")
		return
	}

	// Get duplicate resolution state
	stateData, hasState := h.stateManager.GetUserState(user.TelegramID, "duplicate_resolution")
	if !hasState {
		h.AnswerCallback(callback.ID, "❌ Resolution data expired. Please try again.")
		return
	}

	// Parse stored duplicate data
	var duplicateState DuplicateState
	if err := json.Unmarshal([]byte(stateData), &duplicateState); err != nil {
		h.logger.Error("Failed to parse duplicate state", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to process data.")
		return
	}

	// Verify list ID matches
	if !strings.HasPrefix(duplicateState.ListID.String(), shortListID) {
		h.AnswerCallback(callback.ID, "❌ List ID mismatch.")
		return
	}

	// Verify index is valid
	if index < 0 || index >= len(duplicateState.Duplicates) {
		h.AnswerCallback(callback.ID, "❌ Invalid item index.")
		return
	}

	duplicate := duplicateState.Duplicates[index]
	var responseText string

	switch action {
	case "keep":
		// Just acknowledge, no database changes needed
		responseText = "✅ Keeping existing item."

	case "replace":
		// Update existing item with new quantity/info from parsed item
		updateReq := shopping.UpdateShoppingItemRequest{
			Name:     &duplicate.ParsedName,
			Quantity: duplicate.NewQuantity,
		}

		if err := h.shoppingService.UpdateShoppingItem(ctx, duplicate.ExistingItem.ID, updateReq, user.ID); err != nil {
			h.logger.Error("Failed to update item", "error", err, "item_id", duplicate.ExistingItem.ID)
			responseText = "❌ Failed to replace item."
		} else {
			responseText = "✅ Item replaced successfully."
		}

	case "both":
		// Add new item alongside existing one
		quantity := ""
		if duplicate.NewQuantity != nil {
			quantity = *duplicate.NewQuantity
		}

		if _, err := h.shoppingService.AddItemToList(ctx, duplicateState.ListID, duplicate.ParsedName, quantity, user.ID); err != nil {
			h.logger.Error("Failed to add duplicate item", "error", err)
			responseText = "❌ Failed to add both items."
		} else {
			responseText = "✅ Added both items successfully."
		}
	}

	h.AnswerCallback(callback.ID, responseText)

	// Mark this duplicate as resolved in the state
	duplicateState.Duplicates[index].Resolved = true

	// Check if all duplicates are resolved
	allResolved := true
	for _, dup := range duplicateState.Duplicates {
		if !dup.Resolved {
			allResolved = false
			break
		}
	}

	if allResolved {
		// Add any remaining unique items
		if len(duplicateState.UniqueItems) > 0 {
			_, _, err := h.shoppingService.AddParsedItemsToList(ctx, duplicateState.ListID, duplicateState.UniqueItems, user.ID)
			if err != nil {
				h.logger.Error("Failed to add unique items after resolving duplicates", "error", err)
			}
		}

		// Clear all relevant states
		h.stateManager.ClearUserState(user.TelegramID, "duplicate_resolution")
		h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")
		deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
		if _, err := h.bot.Send(deleteMsg); err != nil {
			h.logger.Error("Failed to delete duplicate resolution message", "error", err)
		}

		// Show the updated list
		h.showUpdatedList(ctx, callback.Message.Chat.ID, duplicateState.ListID, user)
	} else {
		// Update state with resolution status
		stateJSON, _ := json.Marshal(duplicateState)
		h.stateManager.SetUserState(user.TelegramID, "duplicate_resolution", string(stateJSON))

		// Update the message to reflect the resolution
		h.updateDuplicateMessage(ctx, callback, duplicateState, user)
	}
}

func (h *DuplicateCallbackHandler) showUpdatedList(ctx context.Context, chatID int64, listID uuid.UUID, user *users.User) {
	// Create a ListCallbackHandler to reuse the existing list building logic
	listHandler := NewListCallbackHandler(h.BaseHandler, h.stateManager)
	message, keyboard, err := listHandler.BuildListViewMessage(ctx, listID, user)
	if err != nil {
		h.logger.Error("Failed to build list view for updated view", "error", err, "list_id", listID)
		return
	}

	// Send a NEW message instead of trying to edit the deleted one
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("Failed to send updated list view message", "error", err)
	}
}

func (h *DuplicateCallbackHandler) updateDuplicateMessage(ctx context.Context, callback *tgbotapi.CallbackQuery, state DuplicateState, user *users.User) {
	// Rebuild the message with updated resolution status
	var messageText strings.Builder
	messageText.WriteString("🔍 <b>Duplicate Items Found!</b>\n\n")

	for i, duplicate := range state.Duplicates {
		if duplicate.Resolved {
			messageText.WriteString(fmt.Sprintf("✅ <s>%s</s> - Resolved\n", duplicate.NewItemName))
		} else {
			messageText.WriteString(fmt.Sprintf("%d. <b>%s</b> vs existing <b>%s</b>\n", i+1, duplicate.NewItemName, duplicate.ExistingItem.Name))
		}
	}

	// Create buttons only for unresolved items
	var buttons [][]tgbotapi.InlineKeyboardButton
	shortListID := state.ListID.String()[:8]

	for i, duplicate := range state.Duplicates {
		if !duplicate.Resolved {
			itemButtons := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					h.templateManager.RenderButton("keep_existing", user.Locale),
					fmt.Sprintf("dup_keep_%d_%s", i, shortListID),
				),
				tgbotapi.NewInlineKeyboardButtonData(
					h.templateManager.RenderButton("replace", user.Locale),
					fmt.Sprintf("dup_replace_%d_%s", i, shortListID),
				),
				tgbotapi.NewInlineKeyboardButtonData(
					h.templateManager.RenderButton("add_both", user.Locale),
					fmt.Sprintf("dup_both_%d_%s", i, shortListID),
				),
			}
			buttons = append(buttons, itemButtons)
		}
	}

	// Add global action buttons for remaining unresolved items
	if len(buttons) > 0 {
		globalButtons := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("keep_all_existing", user.Locale),
				fmt.Sprintf("dup_keepall_%s", shortListID),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("replace_all", user.Locale),
				fmt.Sprintf("dup_replaceall_%s", shortListID),
			),
		}
		buttons = append(buttons, globalButtons)
	}

	// Add cancel button
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			h.templateManager.RenderButton("cancel", user.Locale),
			fmt.Sprintf("dup_cancel_%s", shortListID),
		),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		messageText.String(),
	)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &keyboard

	if _, err := h.bot.Send(editMsg); err != nil {
		h.logger.Error("Failed to update duplicate message", "error", err)
	}
}
