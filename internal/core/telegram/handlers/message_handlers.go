package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/ai"
	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/commands"
	"github.com/PocketPalCo/shopping-service/internal/core/translations"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ProductListDetectedTemplateData holds data for product list detection templates
type ProductListDetectedTemplateData struct {
	DetectedItemsCount int
	ConfidencePercent  float64
	SampleItemsStr     string
}

// MessageHandler handles different types of messages
type MessageHandler struct {
	BaseHandler
	stateManager *StateManager
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(base BaseHandler, stateManager *StateManager) *MessageHandler {
	return &MessageHandler{
		BaseHandler:  base,
		stateManager: stateManager,
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
		successMessage = fmt.Sprintf("✅ Shopping List Created!\n\n📋 Name: %s\n👥 Family: %s", shoppingList.Name, family.Name)
	}

	// Create action buttons for the new list
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("add_item", user.Locale), fmt.Sprintf("list_additem_%s", shoppingList.ID.String())),
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("view_list", user.Locale), fmt.Sprintf("list_view_%s", shoppingList.ID.String())),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("all_lists", user.Locale), "show_all_lists"),
		},
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Delete the user's input message
	h.DeleteMessage(message.Chat.ID, message.MessageID)

	// Get the stored message ID for replacement
	messageIDStr, hasMessageID := h.stateManager.GetUserState(user.TelegramID, "create_list_message_id")

	if hasMessageID {
		// Parse chat ID and message ID from the stored string (format: "chatID:messageID")
		parts := strings.Split(messageIDStr, ":")
		if len(parts) == 2 {
			var chatID, messageID int64
			if n, err := fmt.Sscanf(messageIDStr, "%d:%d", &chatID, &messageID); n == 2 && err == nil {
				// Replace the original "creating list" message with the success message
				h.EditMessageWithKeyboard(chatID, int(messageID), successMessage, keyboard)
			} else {
				h.logger.Error("Failed to parse stored message ID", "message_id_str", messageIDStr)
				// Fallback: send new message
				h.SendMessageWithKeyboard(message.Chat.ID, successMessage, keyboard)
			}
		} else {
			h.logger.Error("Invalid message ID format", "message_id_str", messageIDStr)
			// Fallback: send new message
			h.SendMessageWithKeyboard(message.Chat.ID, successMessage, keyboard)
		}
	} else {
		// Fallback: send new message if no stored message ID
		h.SendMessageWithKeyboard(message.Chat.ID, successMessage, keyboard)
	}

	// Clear the state after successful creation
	h.stateManager.ClearUserState(user.TelegramID, "creating_list_for_family")
	h.stateManager.ClearUserState(user.TelegramID, "create_list_message_id")

	// Feature placeholder: Family notification system will be implemented
	// This will notify family members when new shopping lists are created
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

	// Use AI-powered language detection for accurate results
	detectedLanguage, err := h.shoppingService.DetectLanguage(ctx, itemText)
	if err != nil {
		h.logger.Error("Failed to detect language, falling back to user locale", "error", err, "item_text", itemText)
		detectedLanguage = translations.NormalizeLanguageCode(user.Locale)
	}

	h.logger.Info("Language detection for item input",
		"item_text", itemText,
		"user_locale", user.Locale,
		"detected_language", detectedLanguage,
		"user_id", user.ID)

	// Send loading message while processing AI
	loadingText, err := h.templateManager.RenderTemplate("processing_items", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render processing items template", "error", err)
		loadingText = "🔄 Processing items..."
	}
	loadingMessageID := h.SendLoadingMessage(message.Chat.ID, loadingText)

	// Check for duplicate items first
	duplicates, uniqueItems, err := h.shoppingService.CheckDuplicateItems(ctx, listID, itemText, detectedLanguage, user.ID)
	if err != nil {
		h.logger.Error("Failed to check for duplicate items", "error", err)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		h.SendMessage(message.Chat.ID, "❌ Failed to process items. Please try again.")
		return
	}

	// If there are duplicates, show user choices
	if len(duplicates) > 0 {
		// Delete loading message before showing duplicate resolution
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		h.handleDuplicateItems(ctx, message.Chat.ID, user, duplicates, uniqueItems, listID)
		return
	}

	// No duplicates, add unique items directly
	addedItems, failedItems, err := h.shoppingService.AddParsedItemsToList(ctx, listID, uniqueItems, user.ID)
	if err != nil {
		h.logger.Error("Failed to add items to list", "error", err)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		h.SendMessage(message.Chat.ID, "❌ Failed to add items. Please try again.")
		return
	}

	// Feature placeholder: Item notification system will be implemented
	// This will notify family members when new items are added to shared lists

	// Feature placeholder: State management cleanup will be implemented
	// This will clear user input state after successful item addition

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
			// Show notes if available
			if addedItems[0].Notes != nil && *addedItems[0].Notes != "" {
				successText += fmt.Sprintf(" <i>— %s</i>", *addedItems[0].Notes)
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
				// Show notes if available
				if item.Notes != nil && *item.Notes != "" {
					successText += fmt.Sprintf(" <i>— %s</i>", *item.Notes)
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

	// Delete the user's input message
	h.DeleteMessage(message.Chat.ID, message.MessageID)

	// Try to use the localized template for successful item additions
	var templateData interface{}
	var templateName string

	if len(addedItems) == 1 {
		templateName = "item_added_success"
		templateData = struct {
			ItemName string
			Quantity string
		}{
			ItemName: addedItems[0].Name,
			Quantity: func() string {
				if addedItems[0].Quantity != nil && *addedItems[0].Quantity != "" {
					quantity := strings.TrimSpace(*addedItems[0].Quantity)
					if !strings.HasPrefix(quantity, "0 ") && quantity != "0" {
						return quantity
					}
				}
				return ""
			}(),
		}
	} else {
		// For multiple items, fallback to the constructed text
		templateName = ""
	}

	var finalSuccessText string
	if templateName != "" {
		rendered, err := h.templateManager.RenderTemplate(templateName, user.Locale, templateData)
		if err != nil {
			h.logger.Error("Failed to render success template", "error", err, "template", templateName)
			finalSuccessText = successText
		} else {
			finalSuccessText = rendered
		}
	} else {
		finalSuccessText = successText
	}

	// Create localized action buttons
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("add_another", user.Locale), fmt.Sprintf("list_additem_%s", listID.String())),
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("view_list", user.Locale), fmt.Sprintf("list_view_%s", listID.String())),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("all_lists", user.Locale), "show_all_lists"),
		},
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Check if we have a stored message ID to replace (from viewing list)
	messageIDStr, hasMessageID := h.stateManager.GetUserState(user.TelegramID, "add_item_message_id")
	messageReplaced := false

	if hasMessageID {
		// Parse chat ID and message ID from the stored string (format: "chatID:messageID")
		parts := strings.Split(messageIDStr, ":")
		if len(parts) == 2 {
			var chatID, messageID int64
			if n, err := fmt.Sscanf(messageIDStr, "%d:%d", &chatID, &messageID); n == 2 && err == nil {
				// Delete loading message first
				if loadingMessageID > 0 {
					h.DeleteMessage(message.Chat.ID, loadingMessageID)
				}

				// We came from viewing a list - replace with updated list view instead of success message
				listCallbackHandler := NewListCallbackHandler(h.BaseHandler, h.stateManager)
				listViewMessage, listViewKeyboard, err := listCallbackHandler.BuildListViewMessage(ctx, listID, user)
				if err != nil {
					h.logger.Error("Failed to build updated list view", "error", err)
					// Fallback to success message
					h.EditMessageWithKeyboard(chatID, int(messageID), finalSuccessText, keyboard)
				} else {
					// Replace with updated list view
					h.EditMessageWithKeyboard(chatID, int(messageID), listViewMessage, listViewKeyboard)

					// Set viewing state again so user can continue adding items
					viewStateData := fmt.Sprintf("%s:%d:%d", listID.String(), chatID, messageID)
					h.stateManager.SetUserState(user.TelegramID, "viewing_list", viewStateData)
				}
				messageReplaced = true
			} else {
				h.logger.Error("Failed to parse stored message ID", "message_id_str", messageIDStr)
			}
		} else {
			h.logger.Error("Invalid message ID format", "message_id_str", messageIDStr)
		}
	} else if loadingMessageID > 0 {
		// No stored message ID, just replace loading message with success message
		h.EditMessageWithKeyboard(message.Chat.ID, loadingMessageID, finalSuccessText, keyboard)
		messageReplaced = true
	}

	// Only send new message if we couldn't replace any existing message
	if !messageReplaced {
		h.SendMessageWithKeyboard(message.Chat.ID, finalSuccessText, keyboard)
	}

	// Clear the state after successful addition
	h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")
	h.stateManager.ClearUserState(user.TelegramID, "add_item_message_id")
}

// HandleAuthorizedMessage processes messages from authorized users
func (h *MessageHandler) HandleAuthorizedMessage(ctx context.Context, message *tgbotapi.Message, user *users.User) {
	// Check for user states and delegate to appropriate handlers - check if user has any relevant active states
	allStates := h.stateManager.GetAllStates()
	userPrefix := fmt.Sprintf("%d:", user.TelegramID)
	hasActiveState := false

	// Only consider list-related states as "active states" that block normal message processing
	relevantStates := []string{
		"adding_item_to_list",
		"viewing_list",
		"creating_list_for_family",
		"product_list_selection",
		"creating_custom_productlist",
		"duplicate_resolution",
	}

	for key := range allStates {
		if strings.HasPrefix(key, userPrefix) {
			// Extract the state name (everything after "userID:")
			parts := strings.SplitN(key, ":", 2)
			if len(parts) == 2 {
				stateName := parts[1]
				for _, relevantState := range relevantStates {
					if stateName == relevantState {
						hasActiveState = true
						break
					}
				}
			}
			if hasActiveState {
				break
			}
		}
	}

	if hasActiveState {
		// User has an active state, check what specific state they're in
		h.logger.Info("User has active state, checking state type", "user_id", user.ID, "telegram_id", user.TelegramID)

		// Check for specific states and handle them appropriately
		// PRIORITY ORDER: Handle direct list operations first, then general product list states
		if _, exists := h.stateManager.GetUserState(user.TelegramID, "adding_item_to_list"); exists {
			// Handle add item input - this takes priority over other states
			listIDStr := strings.Split(strings.Split(allStates[fmt.Sprintf("%d:adding_item_to_list", user.TelegramID)], ":")[1], "")[0]
			h.HandleAddItemInput(ctx, message, user, listIDStr)
			return
		} else if viewStateData, exists := h.stateManager.GetUserState(user.TelegramID, "viewing_list"); exists {
			h.logger.Info("Processing message while viewing list", "user_id", user.ID, "list_data", viewStateData, "message_text", message.Text)
			// Handle message while viewing a list - check if it's a product list
			h.HandleViewingListMessage(ctx, message, user, viewStateData)
			return
		} else if _, exists := h.stateManager.GetUserState(user.TelegramID, "creating_list_for_family"); exists {
			// Handle shopping list name input
			familyIDStr := strings.Split(strings.Split(allStates[fmt.Sprintf("%d:creating_list_for_family", user.TelegramID)], ":")[1], "")[0]
			h.HandleShoppingListNameInput(ctx, message, user, familyIDStr)
			return
		} else if stateData, exists := h.stateManager.GetUserState(user.TelegramID, "product_list_selection"); exists {
			h.logger.Info("User in product_list_selection state, replacing with new message processing", "user_id", user.ID)

			// Parse the existing state to get the message ID
			var existingState ProductListState
			if err := json.Unmarshal([]byte(stateData), &existingState); err != nil {
				h.logger.Error("Failed to unmarshal existing product list state", "error", err)
			}

			// Clear the old state and process the new message as a potential product list
			h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")
			h.logger.Info("Cleared product_list_selection state, processing new message", "user_id", user.ID)

			// Store the previous message ID in a temporary state for replacement
			if existingState.ProductListMessageID != "" {
				h.stateManager.SetUserState(user.TelegramID, "replace_message_id", existingState.ProductListMessageID)
			}
			// Continue processing the new message below
		} else if stateData, exists := h.stateManager.GetUserState(user.TelegramID, "creating_custom_productlist"); exists {
			// Handle custom product list name input - delegate to ProductListCallbackHandler
			if h.stateManager != nil {
				productListHandler := NewProductListCallbackHandler(h.BaseHandler, h.stateManager)
				productListHandler.HandleCustomListNameInput(ctx, message, user, stateData)
			}
			return
		} else {
			// Unknown state, send generic message
			h.logger.Warn("User has unknown active state", "user_id", user.ID, "states", allStates)
			messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render message received template", "error", err)
				messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
			}
			h.SendMessage(message.Chat.ID, messageText)
			return
		}
	}

	// No active state, check if message contains a product list
	messageText := strings.TrimSpace(message.Text)
	if messageText == "" {
		messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render message received template", "error", err)
			messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
		}
		h.SendMessage(message.Chat.ID, messageText)
		return
	}

	// Skip obvious commands or very short messages (but allow single product names)
	if strings.HasPrefix(messageText, "/") || len(messageText) < 2 {
		messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render message received template", "error", err)
			messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
		}
		h.SendMessage(message.Chat.ID, messageText)
		return
	}

	// Use AI to detect if this is a product list
	h.logger.Info("Analyzing text for product list detection",
		"user_id", user.ID,
		"text_length", len(messageText),
		"text_preview", messageText[:min(50, len(messageText))]+"...")

	// Send loading message while processing AI
	loadingText, err := h.templateManager.RenderTemplate("processing_items", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render processing items template", "error", err)
		loadingText = "🔄 Processing items..."
	}
	loadingMessageID := h.SendLoadingMessage(message.Chat.ID, loadingText)

	productListResult, err := h.shoppingService.DetectProductList(ctx, messageText)
	if err != nil {
		h.logger.Error("Failed to detect product list", "error", err, "user_id", user.ID)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render message received template", "error", err)
			messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
		}
		h.SendMessage(message.Chat.ID, messageText)
		return
	}

	// Log the detection result
	h.logger.Info("Product list detection completed",
		"user_id", user.ID,
		"is_product_list", productListResult.IsProductList,
		"confidence", productListResult.Confidence,
		"detected_items", productListResult.DetectedItemsCount,
		"sample_items", productListResult.SampleItems)

	// If not a product list or confidence is too low, send appropriate message
	if !productListResult.IsProductList || productListResult.Confidence < 0.5 {
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}

		// If confidence is medium (0.3-0.5), suggest it might be a product list
		if productListResult.Confidence >= 0.3 && productListResult.Confidence < 0.5 {
			messageText, err := h.templateManager.RenderTemplate("product_list_low_confidence", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render low confidence template", "error", err)
				messageText = "🤔 I noticed you might be trying to add items to a shopping list, but I'm not sure. Use /lists to manage your shopping lists manually or try writing items more clearly."
			}
			h.SendMessage(message.Chat.ID, messageText)
		} else {
			// Very low confidence - send normal message received
			messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render message received template", "error", err)
				messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
			}
			h.SendMessage(message.Chat.ID, messageText)
		}
		return
	}

	// It's a product list! Now check if user has families
	h.handleProductListDetected(ctx, message, user, messageText, productListResult, loadingMessageID)
}

// SendUnauthorizedMessage sends a message to unauthorized users
func (h *MessageHandler) SendUnauthorizedMessage(chatID int64, user *users.User) {
	displayName := commands.GetUserDisplayName(user)
	message := fmt.Sprintf(`🔒 Access Denied

Hello %s! You need to be authorized to use this bot.

Your Telegram ID: %d

Please contact an administrator to request access.

Available commands: /start, /help, /status`,
		displayName, user.TelegramID)

	h.SendMessage(chatID, message)
}

// handleDuplicateItems presents user with choices when duplicate items are detected
func (h *MessageHandler) handleDuplicateItems(ctx context.Context, chatID int64, user *users.User, duplicates []*shopping.DuplicateItemInfo, uniqueItems []*ai.ParsedResult, listID uuid.UUID) {
	// Prepare data for template
	data := struct {
		Duplicates []*shopping.DuplicateItemInfo
	}{
		Duplicates: duplicates,
	}

	// Render localized message
	message, err := h.templateManager.RenderTemplate("duplicate_items_found", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render duplicate items template", "error", err)
		message = "🔍 <b>Duplicate Items Found!</b>\n\nThe following items are already in your list. Choose what to do:"
	}

	var buttons [][]tgbotapi.InlineKeyboardButton

	for i := range duplicates {
		// Create buttons for this duplicate item
		itemButtons := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("keep_existing", user.Locale),
				fmt.Sprintf("dup_keep_%d_%s", i, listID.String()[:8]),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("replace", user.Locale),
				fmt.Sprintf("dup_replace_%d_%s", i, listID.String()[:8]),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("add_both", user.Locale),
				fmt.Sprintf("dup_both_%d_%s", i, listID.String()[:8]),
			),
		}
		buttons = append(buttons, itemButtons)
	}

	// Add global action buttons
	globalButtons := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			h.templateManager.RenderButton("keep_all_existing", user.Locale),
			fmt.Sprintf("dup_keepall_%s", listID.String()[:8]),
		),
		tgbotapi.NewInlineKeyboardButtonData(
			h.templateManager.RenderButton("replace_all", user.Locale),
			fmt.Sprintf("dup_replaceall_%s", listID.String()[:8]),
		),
		tgbotapi.NewInlineKeyboardButtonData(
			h.templateManager.RenderButton("cancel", user.Locale),
			fmt.Sprintf("dup_cancel_%s", listID.String()[:8]),
		),
	}
	buttons = append(buttons, globalButtons)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Store duplicate data in user state for callback handling
	duplicateState := DuplicateState{
		ListID:      listID,
		Duplicates:  make([]*DuplicateItemInfoWithResolution, len(duplicates)),
		UniqueItems: uniqueItems,
	}

	// Convert duplicates to include resolution status
	for i, dup := range duplicates {
		duplicateState.Duplicates[i] = &DuplicateItemInfoWithResolution{
			DuplicateItemInfo: dup,
			Resolved:          false,
		}
	}

	// Store state as JSON
	stateData, err := json.Marshal(duplicateState)
	if err != nil {
		h.logger.Error("Failed to marshal duplicate state", "error", err)
		h.SendMessage(chatID, "❌ Failed to process duplicates. Please try again.")
		return
	}

	h.stateManager.SetUserState(user.TelegramID, "duplicate_resolution", string(stateData))

	h.SendMessageWithKeyboard(chatID, message, keyboard)
}

// DuplicateState represents the stored state for duplicate resolution
type DuplicateState struct {
	ListID      uuid.UUID                          `json:"list_id"`
	Duplicates  []*DuplicateItemInfoWithResolution `json:"duplicates"`
	UniqueItems []*ai.ParsedResult                 `json:"unique_items"`
}

// DuplicateItemInfoWithResolution extends DuplicateItemInfo with resolution status
type DuplicateItemInfoWithResolution struct {
	*shopping.DuplicateItemInfo
	Resolved bool `json:"resolved"`
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleProductListDetected handles when a product list is detected in user's message
func (h *MessageHandler) handleProductListDetected(ctx context.Context, message *tgbotapi.Message, user *users.User, messageText string, productListResult *ai.ProductListDetectionResult, loadingMessageID int) {
	// Check if user belongs to any families
	userFamilies, err := h.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user families", "error", err, "user_id", user.ID)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		h.sendFallbackMessage(message.Chat.ID, user)
		return
	}

	if len(userFamilies) == 0 {
		// User has no families, send fallback message
		h.logger.Info("User has no families, sending fallback message", "user_id", user.ID)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		h.sendFallbackMessage(message.Chat.ID, user)
		return
	}

	// User has families! Show them options to add to existing list or create new one
	h.logger.Info("Product list detected for user with families",
		"user_id", user.ID,
		"families_count", len(userFamilies),
		"detected_items", productListResult.DetectedItemsCount,
		"confidence", productListResult.Confidence)

	// Show options to user
	h.showProductListOptions(ctx, message, user, messageText, productListResult, userFamilies, loadingMessageID)
}

// sendFallbackMessage sends a fallback message for users without families
func (h *MessageHandler) sendFallbackMessage(chatID int64, user *users.User) {
	message, err := h.templateManager.RenderTemplate("product_list_fallback", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render product list fallback template", "error", err)
		message = "🛒 Shopping List Detected! To use this feature, you need to be part of a family. Use /families to create or join a family."
	}

	h.SendMessage(chatID, message)
}

// showProductListOptions shows options to add items to existing lists or create a new list
func (h *MessageHandler) showProductListOptions(ctx context.Context, message *tgbotapi.Message, user *users.User, messageText string, productListResult *ai.ProductListDetectionResult, userFamilies []*families.Family, loadingMessageID int) {
	// Get user's shopping lists for their families
	var availableLists []*shopping.ShoppingList

	for _, family := range userFamilies {
		familyLists, err := h.shoppingService.GetFamilyShoppingLists(ctx, family.ID)
		if err != nil {
			h.logger.Error("Failed to get family shopping lists", "error", err, "family_id", family.ID)
			continue
		}
		availableLists = append(availableLists, familyLists...)
	}

	// Create message with detected items
	sampleItemsStr := "unknown items"
	if len(productListResult.SampleItems) > 0 {
		sampleItemsStr = strings.Join(productListResult.SampleItems[:min(3, len(productListResult.SampleItems))], ", ")
		if len(productListResult.SampleItems) > 3 {
			sampleItemsStr += "..."
		}
	}

	// Create template data
	templateData := ProductListDetectedTemplateData{
		DetectedItemsCount: productListResult.DetectedItemsCount,
		ConfidencePercent:  productListResult.Confidence * 100,
		SampleItemsStr:     sampleItemsStr,
	}

	messageContent, err := h.templateManager.RenderTemplate("product_list_detected", user.Locale, templateData)
	if err != nil {
		h.logger.Error("Failed to render product list detected template", "error", err)
		messageContent = fmt.Sprintf("🛒 Shopping List Detected! Found %d items with %.1f%% confidence. Items like: %s",
			productListResult.DetectedItemsCount,
			productListResult.Confidence*100,
			sampleItemsStr)
	}

	var buttons [][]tgbotapi.InlineKeyboardButton

	// Show existing lists if available
	if len(availableLists) > 0 && len(availableLists) <= 5 { // Limit to 5 lists to avoid keyboard being too big
		for _, list := range availableLists {
			buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf("➕ Add to '%s'", list.Name),
					fmt.Sprintf("productlist_addto_%s", list.ID.String()[:8]),
				),
			})
		}
	}

	// Option to create a new list with automatic name
	currentDate := time.Now().Format("Jan 02")
	autoName := fmt.Sprintf("Shopping %s", currentDate)
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("📝 Create '%s'", autoName),
			fmt.Sprintf("productlist_create_auto_%s", currentDate),
		),
	})

	// Option to create a new list with custom name
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			"📝 Create with Custom Name",
			"productlist_create_custom",
		),
	})

	// Cancel option
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", "productlist_cancel"),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	// Store the message text in user state for later processing
	stateData := map[string]interface{}{
		"message_text":       messageText,
		"detection_result":   productListResult,
		"available_families": userFamilies,
	}

	stateJSON, err := json.Marshal(stateData)
	if err != nil {
		h.logger.Error("Failed to marshal product list state", "error", err)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		h.SendMessage(message.Chat.ID, "❌ Failed to process your shopping list. Please try again.")
		return
	}

	h.stateManager.SetUserState(user.TelegramID, "product_list_selection", string(stateJSON))

	// Replace the existing bot message (main menu/lists overview) and delete loading message
	var productListMessageID int
	var messageReplaced bool

	// First priority: check for latest bot message to replace (main menu/lists overview)
	if latestMessageIDStr, exists := h.stateManager.GetUserState(user.TelegramID, "latest_bot_message_id"); exists {
		// Parse chat ID and message ID from the stored string (format: "chatID:messageID")
		parts := strings.Split(latestMessageIDStr, ":")
		if len(parts) == 2 {
			var chatID, messageID int64
			if n, err := fmt.Sscanf(latestMessageIDStr, "%d:%d", &chatID, &messageID); n == 2 && err == nil && chatID == message.Chat.ID {
				productListMessageID = int(messageID)
				h.EditMessageWithKeyboard(chatID, int(messageID), messageContent, keyboard)
				messageReplaced = true
				h.logger.Info("Replaced latest bot message with product list", "chat_id", chatID, "message_id", messageID)

				// Delete the loading message since we replaced the existing bot message
				if loadingMessageID > 0 {
					h.DeleteMessage(message.Chat.ID, loadingMessageID)
					h.logger.Info("Deleted loading message after replacing bot message", "loading_message_id", loadingMessageID)
				}
			} else {
				h.logger.Error("Failed to parse latest bot message ID or chat mismatch", "latest_message_id", latestMessageIDStr, "expected_chat", message.Chat.ID)
			}
		}
	}

	// Second priority: check if we should replace a previous product list message
	if !messageReplaced {
		if replaceMessageIDStr, exists := h.stateManager.GetUserState(user.TelegramID, "replace_message_id"); exists {
			h.stateManager.ClearUserState(user.TelegramID, "replace_message_id") // Clear after reading

			// Parse chat ID and message ID from the stored string (format: "chatID:messageID")
			parts := strings.Split(replaceMessageIDStr, ":")
			if len(parts) == 2 {
				var chatID, messageID int64
				if n, err := fmt.Sscanf(replaceMessageIDStr, "%d:%d", &chatID, &messageID); n == 2 && err == nil {
					productListMessageID = int(messageID)
					h.EditMessageWithKeyboard(chatID, int(messageID), messageContent, keyboard)
					messageReplaced = true
					h.logger.Info("Replaced previous product list message", "chat_id", chatID, "message_id", messageID)

					// Delete the loading message since we replaced a specific message
					if loadingMessageID > 0 {
						h.DeleteMessage(message.Chat.ID, loadingMessageID)
					}
				} else {
					h.logger.Error("Failed to parse replace message ID", "replace_message_id", replaceMessageIDStr)
				}
			}
		}
	}

	// Third priority: use loading message if no other message to replace
	if !messageReplaced && loadingMessageID > 0 {
		productListMessageID = loadingMessageID
		h.EditMessageWithKeyboard(message.Chat.ID, loadingMessageID, messageContent, keyboard)
		messageReplaced = true
		h.logger.Info("Replaced loading message with product list (fallback)", "loading_message_id", loadingMessageID)
	}

	// Last resort: send new message
	if !messageReplaced {
		productListMessageID = h.SendMessageWithKeyboardAndGetID(message.Chat.ID, messageContent, keyboard)
		h.logger.Info("Sent new product list message (last resort)", "message_id", productListMessageID)
	}

	// Update state data to include the message ID for future replacements
	stateData["product_list_message_id"] = fmt.Sprintf("%d:%d", message.Chat.ID, productListMessageID)

	// Store the latest bot message ID for future general replacements
	h.stateManager.SetUserState(user.TelegramID, "latest_bot_message_id", fmt.Sprintf("%d:%d", message.Chat.ID, productListMessageID))

	// Re-marshal and store the updated state
	updatedStateJSON, err := json.Marshal(stateData)
	if err != nil {
		h.logger.Error("Failed to marshal updated product list state", "error", err)
	} else {
		h.stateManager.SetUserState(user.TelegramID, "product_list_selection", string(updatedStateJSON))
	}

	// Delete the user's original message containing the product list to keep chat clean
	h.DeleteMessage(message.Chat.ID, message.MessageID)
}

// HandleViewingListMessage processes messages received while user is viewing a list
// It detects if the message is a product list and switches to add mode
func (h *MessageHandler) HandleViewingListMessage(ctx context.Context, message *tgbotapi.Message, user *users.User, viewStateData string) {
	// Parse view state data: "listID:chatID:messageID"
	parts := strings.Split(viewStateData, ":")
	if len(parts) != 3 {
		h.logger.Error("Invalid viewing_list state data", "state_data", viewStateData)
		h.stateManager.ClearUserState(user.TelegramID, "viewing_list")
		return
	}

	listIDStr := parts[0]
	storedChatID := parts[1]
	storedMessageID := parts[2]

	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		h.logger.Error("Failed to parse list ID from viewing state", "error", err, "list_id", listIDStr)
		h.stateManager.ClearUserState(user.TelegramID, "viewing_list")
		return
	}

	itemText := strings.TrimSpace(message.Text)
	if itemText == "" {
		h.SendMessage(message.Chat.ID, "❌ Please provide some text.")
		return
	}

	h.logger.Info("Processing message while viewing list",
		"user_id", user.ID,
		"list_id", listID,
		"message_text", itemText)

	// Send loading message
	loadingText, err := h.templateManager.RenderTemplate("processing_items", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render processing items template", "error", err)
		loadingText = "🔄 Processing items..."
	}
	loadingMessageID := h.SendLoadingMessage(message.Chat.ID, loadingText)

	// Detect if this is a product list using AI
	productListResult, err := h.shoppingService.DetectProductList(ctx, itemText)
	if err != nil {
		h.logger.Error("Failed to detect product list while viewing", "error", err, "user_id", user.ID)
		// Delete loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}
		messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render message received template", "error", err)
			messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
		}
		h.SendMessage(message.Chat.ID, messageText)
		return
	}

	// If it's a product list with good confidence, switch to add mode
	if productListResult.IsProductList && productListResult.Confidence >= 0.5 {
		h.logger.Info("Product list detected while viewing list, switching to add mode",
			"user_id", user.ID,
			"list_id", listID,
			"confidence", productListResult.Confidence)

		// Clear viewing state and switch to adding mode
		h.stateManager.ClearUserState(user.TelegramID, "viewing_list")
		h.stateManager.SetUserState(user.TelegramID, "adding_item_to_list", listID.String())

		// Store the view message ID for later replacement
		h.stateManager.SetUserState(user.TelegramID, "add_item_message_id", storedChatID+":"+storedMessageID)

		// Delete the loading message
		if loadingMessageID > 0 {
			h.DeleteMessage(message.Chat.ID, loadingMessageID)
		}

		// Call the existing HandleAddItemInput logic
		h.HandleAddItemInput(ctx, message, user, listID.String())
		return
	}

	// Not a product list or low confidence - just acknowledge the message
	if loadingMessageID > 0 {
		h.DeleteMessage(message.Chat.ID, loadingMessageID)
	}

	if productListResult.Confidence > 0.3 {
		// Medium confidence - suggest it might be a product list
		templateData := struct {
			Confidence float64
			Items      []string
		}{
			Confidence: productListResult.Confidence,
			Items:      productListResult.SampleItems,
		}

		lowConfidenceMsg, err := h.templateManager.RenderTemplate("product_list_low_confidence", user.Locale, templateData)
		if err != nil {
			h.logger.Error("Failed to render low confidence template", "error", err)
			lowConfidenceMsg = fmt.Sprintf("🤔 This might be a shopping list (confidence: %.0f%%). If you want to add items to your list, try being more specific.", productListResult.Confidence*100)
		}
		h.SendMessage(message.Chat.ID, lowConfidenceMsg)
	} else {
		// Low confidence - just acknowledge
		messageText, err := h.templateManager.RenderTemplate("message_received", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render message received template", "error", err)
			messageText = "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands."
		}
		h.SendMessage(message.Chat.ID, messageText)
	}
}
