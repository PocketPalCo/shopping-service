package handlers

import (
	"context"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CallbackRouter handles routing of callback queries to appropriate handlers
type CallbackRouter struct {
	BaseHandler
	menuCallbackHandler        *MenuCallbackHandler
	listCallbackHandler        *ListCallbackHandler
	generalCallbackHandler     *GeneralCallbackHandler
	duplicateCallbackHandler   *DuplicateCallbackHandler
	productListCallbackHandler *ProductListCallbackHandler
	receiptsCallbackHandler    *ReceiptsCallbackHandler
	languageHandler            *LanguageHandler
	stateManager               *StateManager
}

// NewCallbackRouter creates a new callback router
func NewCallbackRouter(
	base BaseHandler,
	menuHandler *MenuCallbackHandler,
	listHandler *ListCallbackHandler,
	generalHandler *GeneralCallbackHandler,
	duplicateHandler *DuplicateCallbackHandler,
	productListHandler *ProductListCallbackHandler,
	receiptsHandler *ReceiptsCallbackHandler,
	languageHandler *LanguageHandler,
	stateManager *StateManager,
) *CallbackRouter {
	return &CallbackRouter{
		BaseHandler:                base,
		menuCallbackHandler:        menuHandler,
		listCallbackHandler:        listHandler,
		generalCallbackHandler:     generalHandler,
		duplicateCallbackHandler:   duplicateHandler,
		productListCallbackHandler: productListHandler,
		receiptsCallbackHandler:    receiptsHandler,
		languageHandler:            languageHandler,
		stateManager:               stateManager,
	}
}

// RouteCallbackQuery routes callback queries to the appropriate handler
func (r *CallbackRouter) RouteCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	r.logger.Info("Processing callback query",
		"callback_id", callback.ID,
		"user_id", user.TelegramID,
		"is_admin", r.usersService.IsAdmin(user.TelegramID),
		"is_authorized", user.IsAuthorized,
		"data", callback.Data,
		"chat_id", callback.Message.Chat.ID)

	// Parse callback data - handle both underscore and colon separators
	var parts []string
	if strings.Contains(callback.Data, ":") {
		parts = strings.Split(callback.Data, ":")
	} else {
		parts = strings.Split(callback.Data, "_")
	}

	if len(parts) < 1 {
		r.AnswerCallback(callback.ID, "❌ Invalid callback data.")
		return
	}

	switch parts[0] {
	case "menu":
		r.menuCallbackHandler.HandleMenuCallback(ctx, callback, parts, user)
	case "list":
		r.listCallbackHandler.HandleListCallback(ctx, callback, parts, user, r.stateManager)
	case "li":
		// Compact list item callback (li_t_listID_itemID)
		r.generalCallbackHandler.HandleCompactListItem(ctx, callback, parts, user)
	case "createlist":
		// Create list for family (createlist_familyID)
		if len(parts) >= 2 {
			r.generalCallbackHandler.HandleCreateListForFamily(ctx, callback, parts[1], user)
			r.stateManager.SetUserState(user.TelegramID, "creating_list_for_family", parts[1])
		}
	case "show":
		r.generalCallbackHandler.HandleShowCallback(ctx, callback, parts, user)
	case "create":
		if len(parts) >= 3 && parts[1] == "new" && parts[2] == "list" {
			r.generalCallbackHandler.HandleCreateNewList(ctx, callback, user)
		}
	case "dup":
		r.duplicateCallbackHandler.HandleDuplicateCallback(ctx, callback, parts, user)
	case "productlist":
		r.productListCallbackHandler.HandleProductListCallback(ctx, callback, parts, user)
	case "receipts":
		if len(parts) >= 2 && parts[1] == "menu" {
			r.receiptsCallbackHandler.HandleBackToReceiptsMenu(ctx, callback, user)
		} else {
			r.receiptsCallbackHandler.HandleReceiptsCallback(ctx, callback, parts, user)
		}
	case "lang":
		r.routeLanguageCallback(ctx, callback, parts, user)
	default:
		r.AnswerCallback(callback.ID, "❌ Unknown callback action.")
	}
}

// routeLanguageCallback routes language-related callbacks
func (r *CallbackRouter) routeLanguageCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	// Handle language change and then return to main menu
	r.languageHandler.HandleLanguageChange(ctx, callback, parts, user)

	// After language change, redirect to main menu with updated language
	r.menuCallbackHandler.HandleMenuCallback(ctx, callback, []string{"menu", "start"}, user)
}
