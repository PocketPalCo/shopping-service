package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ProductListState represents the stored state data for product list selection
type ProductListState struct {
	MessageText          string                   `json:"message_text"`
	DetectionResult      map[string]interface{}   `json:"detection_result"`
	AvailableFamilies    []map[string]interface{} `json:"available_families"`
	ProductListMessageID string                   `json:"product_list_message_id,omitempty"`
}

// ProductListCallbackHandler handles product list-related callbacks
type ProductListCallbackHandler struct {
	BaseHandler
	stateManager *StateManager
}

// NewProductListCallbackHandler creates a new product list callback handler
func NewProductListCallbackHandler(base BaseHandler, stateManager *StateManager) *ProductListCallbackHandler {
	return &ProductListCallbackHandler{
		BaseHandler:  base,
		stateManager: stateManager,
	}
}

// HandleProductListCallback handles productlist_* callbacks
func (h *ProductListCallbackHandler) HandleProductListCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 2 {
		h.AnswerCallback(callback.ID, "❌ Invalid productlist callback.")
		return
	}

	action := parts[1]
	switch action {
	case "addto":
		if len(parts) < 3 {
			h.AnswerCallback(callback.ID, "❌ Invalid addto callback.")
			return
		}
		listIDShort := parts[2]
		h.handleProductListAddTo(ctx, callback, listIDShort, user)
	case "create":
		if len(parts) < 3 {
			h.AnswerCallback(callback.ID, "❌ Invalid create callback.")
			return
		}
		createType := parts[2]
		h.handleProductListCreate(ctx, callback, createType, parts, user)
	case "cancel":
		h.handleProductListCancel(ctx, callback, user)
	default:
		h.AnswerCallback(callback.ID, "❌ Unknown productlist action.")
	}
}

func (h *ProductListCallbackHandler) handleProductListAddTo(ctx context.Context, callback *tgbotapi.CallbackQuery, listIDShort string, user *users.User) {
	// Get user state to retrieve the original message text
	stateData, exists := h.stateManager.GetUserState(user.TelegramID, "product_list_selection")
	if !exists || stateData == "" {
		h.AnswerCallback(callback.ID, "❌ Session expired. Please try again.")

		// Render localized error message
		errorMsg, err := h.templateManager.RenderTemplate("productlist_errors", user.Locale, struct{ ErrorType string }{ErrorType: "session_expired"})
		if err != nil {
			h.logger.Error("Failed to render session expired error template", "error", err)
			errorMsg = "❌ Session expired. Please try again."
		}
		h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, errorMsg)
		return
	}

	// Parse the stored state
	var state ProductListState
	if err := json.Unmarshal([]byte(stateData), &state); err != nil {
		h.logger.Error("Failed to unmarshal product list state", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to process request.")
		return
	}

	// Find the full list ID from the short ID
	lists, err := h.shoppingService.GetUserShoppingLists(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user lists", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to access your lists.")
		return
	}

	var targetListID uuid.UUID
	var targetListName string
	for _, list := range lists {
		if list.ID.String()[:8] == listIDShort {
			targetListID = list.ID
			targetListName = list.Name
			break
		}
	}

	if targetListID == uuid.Nil {
		h.AnswerCallback(callback.ID, "❌ List not found.")
		return
	}

	h.AnswerCallback(callback.ID, "🔄 Adding items to list...")

	// Show loading message immediately to prevent double clicks
	loadingMsg, err := h.templateManager.RenderTemplate("processing_items", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render processing items template", "error", err)
		loadingMsg = "🔄 <b>Processing items...</b>\n\nPlease wait while I analyze your shopping list."
	}
	h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, loadingMsg)

	// Detect language using AI
	detectedLanguage, err := h.shoppingService.DetectLanguage(ctx, state.MessageText)
	if err != nil {
		h.logger.Error("Failed to detect language, falling back to user locale", "error", err, "text", state.MessageText)
		detectedLanguage = user.Locale // Fallback to user locale
	}

	// Parse the original message and add items to the list
	items, failedItems, err := h.shoppingService.AddItemsToListWithAI(ctx, targetListID, state.MessageText, detectedLanguage, user.ID)
	if err != nil {
		h.logger.Error("Failed to add items to list", "error", err, "list_id", targetListID)
		h.SendMessage(callback.Message.Chat.ID, "❌ Failed to add items to the list. Please try again.")
		return
	}

	// Clear the user state since we've processed the request
	h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")

	// Create localized success message
	var message string
	if len(items) > 0 {
		var messageType string
		if len(failedItems) == 0 {
			messageType = "items_added"
		} else {
			messageType = "items_added_with_failures"
		}

		data := struct {
			MessageType string
			ItemsCount  int
			ListName    string
			FailedCount int
		}{
			MessageType: messageType,
			ItemsCount:  len(items),
			ListName:    targetListName,
			FailedCount: len(failedItems),
		}

		renderedMsg, err := h.templateManager.RenderTemplate("productlist_success", user.Locale, data)
		if err != nil {
			h.logger.Error("Failed to render success template", "error", err)
			if len(failedItems) == 0 {
				message = fmt.Sprintf("✅ Added %d items to '%s'!", len(items), targetListName)
			} else {
				message = fmt.Sprintf("✅ Added %d items to '%s'!\n⚠️ %d items failed to process.", len(items), targetListName, len(failedItems))
			}
		} else {
			message = renderedMsg
		}
	} else {
		// Render error template for no items added
		errorData := struct{ ErrorType string }{ErrorType: "no_items_added"}
		renderedMsg, err := h.templateManager.RenderTemplate("productlist_errors", user.Locale, errorData)
		if err != nil {
			h.logger.Error("Failed to render no items error template", "error", err)
			message = "❌ No items could be added to the list."
		} else {
			message = renderedMsg
		}
	}

	// Create buttons to continue working with the list
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("add_another", user.Locale),
				fmt.Sprintf("list_additem_%s", targetListID.String()[:8]),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("view_list", user.Locale),
				fmt.Sprintf("list_view_%s", targetListID.String()),
			),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("all_lists", user.Locale),
				"show_all_lists",
			),
		},
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Replace the product list selection message with updated list view instead of success message
	listCallbackHandler := NewListCallbackHandler(h.BaseHandler, h.stateManager)
	listViewMessage, listViewKeyboard, err := listCallbackHandler.BuildListViewMessage(ctx, targetListID, user)
	if err != nil {
		h.logger.Error("Failed to build updated list view", "error", err)
		// Fallback to success message
		h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	} else {
		// Replace with updated list view
		h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, listViewMessage, listViewKeyboard)

		// Set viewing state so user can continue adding items by typing
		viewStateData := fmt.Sprintf("%s:%d:%d", targetListID.String(), callback.Message.Chat.ID, callback.Message.MessageID)
		h.stateManager.SetUserState(user.TelegramID, "viewing_list", viewStateData)
	}
}

func (h *ProductListCallbackHandler) handleProductListCreate(ctx context.Context, callback *tgbotapi.CallbackQuery, createType string, parts []string, user *users.User) {
	// Get user state to retrieve the original message text
	stateData, exists := h.stateManager.GetUserState(user.TelegramID, "product_list_selection")
	if !exists || stateData == "" {
		h.AnswerCallback(callback.ID, "❌ Session expired. Please try again.")
		return
	}

	// Parse the stored state
	var state ProductListState
	if err := json.Unmarshal([]byte(stateData), &state); err != nil {
		h.logger.Error("Failed to unmarshal product list state", "error", err)
		h.AnswerCallback(callback.ID, "❌ Failed to process request.")
		return
	}

	switch createType {
	case "auto":
		// Create list with automatic name
		if len(parts) >= 4 {
			datePart := strings.Join(parts[3:], " ")
			listName := fmt.Sprintf("Shopping %s", datePart)
			h.handleCreateAndAddItems(ctx, callback, listName, state, user)
		} else {
			h.AnswerCallback(callback.ID, "❌ Invalid date format.")
		}
	case "custom":
		// Start custom name flow
		h.AnswerCallback(callback.ID, "✍️ Enter list name...")

		// Render localized prompt for list name
		promptText, err := h.templateManager.RenderTemplate("productlist_custom_name_prompt", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render productlist custom name prompt template", "error", err)
			promptText = "📝 Please enter the name for your new shopping list:"
		}

		// Replace the current message with a localized prompt for list name
		h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, promptText)
		// Set user state for custom list creation
		h.stateManager.SetUserState(user.TelegramID, "creating_custom_productlist", stateData)
	default:
		h.AnswerCallback(callback.ID, "❌ Unknown create type.")
	}
}

func (h *ProductListCallbackHandler) handleCreateAndAddItems(ctx context.Context, callback *tgbotapi.CallbackQuery, listName string, state ProductListState, user *users.User) {
	// Only send callback response if there's a callback ID (from button press)
	if callback.ID != "" {
		h.AnswerCallback(callback.ID, "🔄 Creating new list...")
	}

	// Show loading message immediately to prevent double clicks
	loadingMsg, err := h.templateManager.RenderTemplate("processing_items", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render processing items template", "error", err)
		loadingMsg = "🔄 <b>Processing items...</b>\n\nPlease wait while I analyze your shopping list."
	}
	h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, loadingMsg)

	// Get user's families to determine which family to create the list for
	// For now, we'll use the first family from the state or create for the first available family
	var familyID uuid.UUID
	if len(state.AvailableFamilies) > 0 {
		if family, ok := state.AvailableFamilies[0]["id"].(string); ok {
			if parsedID, err := uuid.Parse(family); err == nil {
				familyID = parsedID
			}
		}
	}

	if familyID == uuid.Nil {
		h.logger.Error("No family ID available for list creation")
		h.SendMessage(callback.Message.Chat.ID, "❌ Failed to create list: no family available.")
		return
	}

	// Create the new shopping list
	req := shopping.CreateShoppingListRequest{
		Name:     listName,
		OwnerID:  user.ID,
		FamilyID: &familyID,
		IsShared: true, // Created via product list detection should be shared with family
	}
	list, err := h.shoppingService.CreateShoppingList(ctx, req)
	if err != nil {
		h.logger.Error("Failed to create shopping list", "error", err, "family_id", familyID, "name", listName)
		// Replace the current message with error message
		h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, "❌ Failed to create new shopping list.")
		return
	}

	// Detect language using AI
	detectedLanguage, err := h.shoppingService.DetectLanguage(ctx, state.MessageText)
	if err != nil {
		h.logger.Error("Failed to detect language, falling back to user locale", "error", err, "text", state.MessageText)
		detectedLanguage = user.Locale // Fallback to user locale
	}

	// Add items to the new list using AI parsing
	items, failedItems, err := h.shoppingService.AddItemsToListWithAI(ctx, list.ID, state.MessageText, detectedLanguage, user.ID)
	if err != nil {
		h.logger.Error("Failed to add items to new list", "error", err, "list_id", list.ID)
		// Replace the current message with partial success message
		h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, fmt.Sprintf("✅ Created list '%s' but failed to add items. Please try adding them manually.", listName))
		return
	}

	// Clear the user state since we've processed the request
	h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")

	// Create localized success message
	var message string
	if len(items) > 0 {
		var messageType string
		if len(failedItems) == 0 {
			messageType = "list_created"
		} else {
			messageType = "list_created_with_failures"
		}

		data := struct {
			MessageType string
			ItemsCount  int
			ListName    string
			FailedCount int
		}{
			MessageType: messageType,
			ItemsCount:  len(items),
			ListName:    listName,
			FailedCount: len(failedItems),
		}

		renderedMsg, err := h.templateManager.RenderTemplate("productlist_success", user.Locale, data)
		if err != nil {
			h.logger.Error("Failed to render list creation success template", "error", err)
			if len(failedItems) == 0 {
				message = fmt.Sprintf("✅ Created list '%s' and added %d items!", listName, len(items))
			} else {
				message = fmt.Sprintf("✅ Created list '%s' and added %d items!\n⚠️ %d items failed to process.", listName, len(items), len(failedItems))
			}
		} else {
			message = renderedMsg
		}
	} else {
		// Render template for list created but no items added
		data := struct {
			MessageType string
			ListName    string
		}{
			MessageType: "list_created_no_items",
			ListName:    listName,
		}

		renderedMsg, err := h.templateManager.RenderTemplate("productlist_success", user.Locale, data)
		if err != nil {
			h.logger.Error("Failed to render list created no items template", "error", err)
			message = fmt.Sprintf("✅ Created list '%s' but no items could be added.", listName)
		} else {
			message = renderedMsg
		}
	}

	// Create buttons to continue working with the new list
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("add_item", user.Locale),
				fmt.Sprintf("list_additem_%s", list.ID.String()[:8]),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("view_list", user.Locale),
				fmt.Sprintf("list_view_%s", list.ID.String()),
			),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("all_lists", user.Locale),
				"show_all_lists",
			),
		},
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Replace the product list selection message with updated list view instead of success message
	listCallbackHandler := NewListCallbackHandler(h.BaseHandler, h.stateManager)
	listViewMessage, listViewKeyboard, err := listCallbackHandler.BuildListViewMessage(ctx, list.ID, user)
	if err != nil {
		h.logger.Error("Failed to build updated list view for new list", "error", err)
		// Fallback to success message
		h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	} else {
		// Replace with updated list view
		h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, listViewMessage, listViewKeyboard)

		// Set viewing state so user can continue adding items by typing
		viewStateData := fmt.Sprintf("%s:%d:%d", list.ID.String(), callback.Message.Chat.ID, callback.Message.MessageID)
		h.stateManager.SetUserState(user.TelegramID, "viewing_list", viewStateData)
	}
}

func (h *ProductListCallbackHandler) handleProductListCancel(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.AnswerCallback(callback.ID, "❌ Operation cancelled.")

	// Clear the user state
	h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")

	// Instead of sending a new message, replace the current message with the lists overview
	// Get user's shopping lists to show them
	lists, err := h.shoppingService.GetUserShoppingLists(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user shopping lists for cancel", "error", err, "user_id", user.ID)
		// Fallback to simple cancelled message as replacement
		h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, "❌ Operation cancelled.")
		return
	}

	// Prepare lists overview data
	data := struct {
		Lists     []*shopping.ShoppingList
		ListCount int
	}{
		Lists:     lists,
		ListCount: len(lists),
	}

	// Render the lists template
	content, err := h.templateManager.RenderTemplate("lists_overview", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render lists overview template for cancel", "error", err)
		// Fallback to simple cancelled message as replacement
		h.EditMessage(callback.Message.Chat.ID, callback.Message.MessageID, "❌ Operation cancelled.")
		return
	}

	// Create buttons for each list
	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, list := range lists {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("📋 %s", list.Name),
				fmt.Sprintf("list_view_%s", list.ID.String()),
			),
		})
	}

	// Add action buttons
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			h.templateManager.RenderButton("create_list", user.Locale),
			"create_list",
		),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Replace the product list selection message with the lists overview
	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, content, keyboard)
}

// HandleCustomListNameInput handles user input for custom list name creation
func (h *ProductListCallbackHandler) HandleCustomListNameInput(ctx context.Context, message *tgbotapi.Message, user *users.User, stateData string) {
	// Parse the stored state
	var state ProductListState
	if err := json.Unmarshal([]byte(stateData), &state); err != nil {
		h.logger.Error("Failed to unmarshal product list state", "error", err)
		h.SendMessage(message.Chat.ID, "❌ Failed to process request. Please try again.")
		return
	}

	// Get the custom list name from the message
	listName := strings.TrimSpace(message.Text)
	if listName == "" {
		h.SendMessage(message.Chat.ID, "❌ List name cannot be empty. Please enter a valid name.")
		return
	}

	// Create the list and add items using the helper function
	callback := &tgbotapi.CallbackQuery{
		ID: "", // Empty callback ID since this is from a message, not a callback
		Message: &tgbotapi.Message{
			Chat: message.Chat,
		},
	}

	h.handleCreateAndAddItems(ctx, callback, listName, state, user)
}
