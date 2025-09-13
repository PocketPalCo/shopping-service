package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// GeneralCallbackHandler handles general callbacks like create list, show lists, etc.
type GeneralCallbackHandler struct {
	BaseHandler
	stateManager *StateManager
}

// NewGeneralCallbackHandler creates a new general callback handler
func NewGeneralCallbackHandler(base BaseHandler, stateManager *StateManager) *GeneralCallbackHandler {
	return &GeneralCallbackHandler{
		BaseHandler:  base,
		stateManager: stateManager,
	}
}

// HandleCreateNewList starts the create list workflow
func (h *GeneralCallbackHandler) HandleCreateNewList(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	// Get user's families
	families, err := h.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user families", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to retrieve families.")
		return
	}

	if len(families) == 0 {
		message, err := h.templateManager.RenderTemplate("createlist_no_families", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render no families template", "error", err)
			message = "❌ You need to be part of a family to create shopping lists.\n\nUse /createfamily to create a family first."
		}
		h.AnswerCallback(callback.ID, "No families found")

		editMsg := tgbotapi.NewEditMessageText(
			callback.Message.Chat.ID,
			callback.Message.MessageID,
			message,
		)
		editMsg.ParseMode = tgbotapi.ModeHTML
		if _, err := h.bot.Send(editMsg); err != nil {
			h.logger.Error("Failed to send edit message", "error", err)
		}
		return
	}

	// Create buttons for each family
	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, family := range families {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("👥 %s", family.Name),
			fmt.Sprintf("createlist_%s", family.ID.String()),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add cancel button
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("cancel", user.Locale), "show_all_lists"),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	data := struct {
		FamilyCount int
	}{
		FamilyCount: len(families),
	}

	message, err := h.templateManager.RenderTemplate("createlist_family_selection", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render family selection template", "error", err)
		message = fmt.Sprintf("📋 <b>Create New Shopping List</b>\n\nSelect the family for your new shopping list (%d families available):", len(families))
	}

	h.AnswerCallback(callback.ID, "Select family")

	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		message,
	)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &keyboard

	if _, err := h.bot.Send(editMsg); err != nil {
		h.logger.Error("Failed to edit message for family selection", "error", err)
	}
}

// HandleShowAllLists displays all user's shopping lists
func (h *GeneralCallbackHandler) HandleShowAllLists(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	listsWithFamilies, familyCount, err := h.shoppingService.GetUserShoppingListsWithFamilies(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user shopping lists with families", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to retrieve shopping lists.")
		return
	}

	if len(listsWithFamilies) == 0 {
		message, err := h.templateManager.RenderTemplate("no_lists", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render no lists template", "error", err)
			message = "📝 You don't have any shopping lists yet."
		}

		h.AnswerCallback(callback.ID, "No lists found")

		editMsg := tgbotapi.NewEditMessageText(
			callback.Message.Chat.ID,
			callback.Message.MessageID,
			message,
		)
		editMsg.ParseMode = tgbotapi.ModeHTML
		if _, err := h.bot.Send(editMsg); err != nil {
			h.logger.Error("Failed to send edit message", "error", err)
		}
		return
	}

	// Create buttons for each list
	var buttons [][]tgbotapi.InlineKeyboardButton
	showFamilyNames := familyCount > 1 // Only show family names if user has more than 1 family

	for _, listWithFamily := range listsWithFamilies {
		var buttonText string
		if showFamilyNames && listWithFamily.FamilyName != nil {
			buttonText = fmt.Sprintf("📋 %s 👥 %s", listWithFamily.Name, *listWithFamily.FamilyName)
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
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("create_new_list", user.Locale), "create_new_list"),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	data := struct {
		ListCount int
	}{
		ListCount: len(listsWithFamilies),
	}

	message, err := h.templateManager.RenderTemplate("lists_overview", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render lists overview template", "error", err)
		message = fmt.Sprintf("📝 <b>Your Shopping Lists (%d)</b>\n\nSelect a list to view or manage:", len(listsWithFamilies))
	}

	h.AnswerCallback(callback.ID, "Lists loaded")

	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		message,
	)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &keyboard

	if _, err := h.bot.Send(editMsg); err != nil {
		h.logger.Error("Failed to edit message for lists overview", "error", err)
	} else {
		// Track this message for future replacements
		if h.stateManager != nil {
			h.stateManager.SetUserState(user.TelegramID, "latest_bot_message_id", fmt.Sprintf("%d:%d", callback.Message.Chat.ID, callback.Message.MessageID))
		}
	}
}

// HandleCreateListForFamily handles creating a list for a specific family
func (h *GeneralCallbackHandler) HandleCreateListForFamily(ctx context.Context, callback *tgbotapi.CallbackQuery, familyIDStr string, user *users.User) {
	familyID, err := uuid.Parse(familyIDStr)
	if err != nil {
		h.logger.Error("Failed to parse family ID", "error", err, "family_id", familyIDStr)
		h.AnswerCallback(callback.ID, "❌ Invalid family ID.")
		return
	}

	// Verify user has access to this family
	families, err := h.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user families", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to verify family access.")
		return
	}

	var targetFamilyName string
	var found bool
	for _, family := range families {
		if family.ID == familyID {
			targetFamilyName = family.Name
			found = true
			break
		}
	}

	if !found {
		h.AnswerCallback(callback.ID, "❌ Family not found or no access.")
		return
	}

	data := struct {
		FamilyName string
	}{
		FamilyName: targetFamilyName,
	}

	message, err := h.templateManager.RenderTemplate("createlist_enter_name", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render enter name template", "error", err)
		message = fmt.Sprintf("📝 <b>Create Shopping List for: %s</b>\n\nPlease reply with the name for your new shopping list.\n\n<b>Examples:</b>\n• Weekly Groceries\n• Party Supplies\n• Emergency Items", targetFamilyName)
	}

	h.AnswerCallback(callback.ID, "Enter list name")

	// Add cancel button
	cancelButton := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("cancel", user.Locale), "show_all_lists"),
		},
	}
	cancelKeyboard := tgbotapi.NewInlineKeyboardMarkup(cancelButton...)

	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		message,
	)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &cancelKeyboard

	if _, err := h.bot.Send(editMsg); err != nil {
		h.logger.Error("Failed to edit message for list name input", "error", err)
	}

	// Store state that user is creating list for this family and the message ID for later replacement
	h.stateManager.SetUserState(user.TelegramID, "creating_list_for_family", familyID.String())
	h.stateManager.SetUserState(user.TelegramID, "create_list_message_id", fmt.Sprintf("%d:%d", callback.Message.Chat.ID, callback.Message.MessageID))
	h.logger.Info("User entered create list state", "user_id", user.TelegramID, "family_id", familyID)
}

// HandleCompactListItem handles compact list item callbacks (li_t_listID_itemID)
func (h *GeneralCallbackHandler) HandleCompactListItem(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 4 {
		h.AnswerCallback(callback.ID, "❌ Invalid compact callback.")
		return
	}

	// Expected format: li_t_shortListID_shortItemID
	action := parts[1] // should be "t" for toggle
	shortListID := parts[2]
	shortItemID := parts[3]

	if action != "t" {
		h.AnswerCallback(callback.ID, "❌ Unknown compact action.")
		return
	}

	// Find the full UUIDs by searching through user's lists and items
	lists, err := h.shoppingService.GetUserShoppingLists(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user shopping lists", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to load lists.")
		return
	}

	var targetListID, targetItemID uuid.UUID
	var found bool

	// Find the matching list and item
	for _, list := range lists {
		if strings.HasPrefix(list.ID.String(), shortListID) {
			targetListID = list.ID

			// Get items for this list
			items, err := h.shoppingService.GetListItems(ctx, list.ID)
			if err != nil {
				continue
			}

			// Find matching item
			for _, item := range items {
				if strings.HasPrefix(item.ID.String(), shortItemID) {
					targetItemID = item.ID
					found = true
					break
				}
			}

			if found {
				break
			}
		}
	}

	if !found {
		h.AnswerCallback(callback.ID, "❌ Item not found.")
		return
	}

	// Check if user can access this list
	canAccess, err := h.shoppingService.CanUserAccessList(ctx, targetListID, user.ID)
	if err != nil {
		h.logger.Error("Failed to check list access", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to verify list access.")
		return
	}

	if !canAccess {
		h.AnswerCallback(callback.ID, "❌ You don't have access to this list.")
		return
	}

	// Delegate to the list callback handler for toggling the item
	listHandler := NewListCallbackHandler(h.BaseHandler, h.stateManager)
	listHandler.HandleToggleItem(ctx, callback, targetListID, targetItemID.String(), user)
}

// HandleShowCallback handles show_* callbacks (moved from bot_service.go)
func (h *GeneralCallbackHandler) HandleShowCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 2 {
		h.AnswerCallback(callback.ID, "❌ Invalid show callback.")
		return
	}

	// Clear all list-related states when navigating to lists overview
	if h.stateManager != nil {
		h.stateManager.ClearUserState(user.TelegramID, "viewing_list")
		h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")
		h.stateManager.ClearUserState(user.TelegramID, "add_item_message_id")
		h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")
		h.stateManager.ClearUserState(user.TelegramID, "creating_list_for_family")
		h.stateManager.ClearUserState(user.TelegramID, "duplicate_resolution")
		h.stateManager.ClearUserState(user.TelegramID, "creating_custom_productlist")
		h.stateManager.ClearUserState(user.TelegramID, "replace_message_id")
		h.logger.Info("Cleared all list-related states for lists overview", "user_id", user.TelegramID, "show_action", parts[1])
	}

	switch parts[1] {
	case "all", "lists":
		h.HandleShowAllLists(ctx, callback, user)
	default:
		h.AnswerCallback(callback.ID, "❌ Unknown show action.")
	}
}
