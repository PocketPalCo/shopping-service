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
	MessageText     string                 `json:"message_text"`
	DetectionResult map[string]interface{} `json:"detection_result"`
	AvailableFamilies []map[string]interface{} `json:"available_families"`
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

	// Send success message
	var message string
	if len(items) > 0 {
		if len(failedItems) == 0 {
			message = fmt.Sprintf("✅ Added %d items to '%s'!", len(items), targetListName)
		} else {
			message = fmt.Sprintf("✅ Added %d items to '%s'!\n⚠️ %d items failed to process.", len(items), targetListName, len(failedItems))
		}
	} else {
		message = "❌ No items could be added to the list."
	}

	h.SendMessage(callback.Message.Chat.ID, message)
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
		h.SendMessage(callback.Message.Chat.ID, "📝 Please enter the name for your new shopping list:")
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
		h.SendMessage(callback.Message.Chat.ID, "❌ Failed to create new shopping list.")
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
		h.SendMessage(callback.Message.Chat.ID, fmt.Sprintf("✅ Created list '%s' but failed to add items. Please try adding them manually.", listName))
		return
	}

	// Clear the user state since we've processed the request
	h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")

	// Send success message
	var message string
	if len(items) > 0 {
		if len(failedItems) == 0 {
			message = fmt.Sprintf("✅ Created list '%s' and added %d items!", listName, len(items))
		} else {
			message = fmt.Sprintf("✅ Created list '%s' and added %d items!\\n⚠️ %d items failed to process.", listName, len(items), len(failedItems))
		}
	} else {
		message = fmt.Sprintf("✅ Created list '%s' but no items could be added.", listName)
	}

	h.SendMessage(callback.Message.Chat.ID, message)
}

func (h *ProductListCallbackHandler) handleProductListCancel(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.AnswerCallback(callback.ID, "❌ Operation cancelled.")
	
	// Clear the user state
	h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")
	
	h.SendMessage(callback.Message.Chat.ID, "❌ Operation cancelled.")
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