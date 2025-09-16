package handlers

import (
	"context"
	"fmt"

	"github.com/PocketPalCo/shopping-service/internal/core/telegram/commands"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MenuCallbackHandler handles menu button callbacks
type MenuCallbackHandler struct {
	BaseHandler
	commandRegistry CommandExecutor
	stateManager    *StateManager
}

// CommandExecutor interface for executing commands from menu
type CommandExecutor interface {
	ExecuteCommand(ctx context.Context, commandName string, chatID int64, user *users.User, args []string) error
}

// NewMenuCallbackHandler creates a new menu callback handler
func NewMenuCallbackHandler(base BaseHandler, commandRegistry CommandExecutor, stateManager *StateManager) *MenuCallbackHandler {
	return &MenuCallbackHandler{
		BaseHandler:     base,
		commandRegistry: commandRegistry,
		stateManager:    stateManager,
	}
}

// HandleMenuCallback processes menu button callbacks
func (h *MenuCallbackHandler) HandleMenuCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 2 {
		h.answerCallback(callback.ID, "❌ Invalid menu action.")
		return
	}

	menuAction := parts[1]

	// Clear all list-related states when navigating to main menu areas
	if h.stateManager != nil {
		h.stateManager.ClearUserState(user.TelegramID, "viewing_list")
		h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")
		h.stateManager.ClearUserState(user.TelegramID, "add_item_message_id")
		h.stateManager.ClearUserState(user.TelegramID, "product_list_selection")
		h.stateManager.ClearUserState(user.TelegramID, "creating_list_for_family")
		h.stateManager.ClearUserState(user.TelegramID, "duplicate_resolution")
		h.stateManager.ClearUserState(user.TelegramID, "creating_custom_productlist")
		h.stateManager.ClearUserState(user.TelegramID, "replace_message_id")
		h.logger.Info("Cleared all list-related states for navigation", "user_id", user.TelegramID, "menu_action", menuAction)
	}

	// Map menu actions to their corresponding commands
	var commandName string

	switch menuAction {
	case "start":
		commandName = "start"
	case "lists":
		commandName = "lists"
	case "families":
		commandName = "families"
	case "createlist":
		commandName = "createlist"
	case "createfamily":
		commandName = "createfamily"
	case "help":
		commandName = "help"
	case "status":
		commandName = "status"
	case "myid":
		commandName = "myid"
	case "receipts":
		commandName = "receipts"
	case "users":
		// Admin only
		if !h.usersService.IsAdmin(user.TelegramID) {
			h.answerCallback(callback.ID, "❌ Access denied.")
			return
		}
		commandName = "users"
	case "stats":
		// Admin only
		if !h.usersService.IsAdmin(user.TelegramID) {
			h.answerCallback(callback.ID, "❌ Access denied.")
			return
		}
		commandName = "stats"
	case "language":
		commandName = "language"
	default:
		h.answerCallback(callback.ID, "❌ Unknown menu action.")
		return
	}

	// Handle all menu commands with message editing instead of sending new messages
	switch commandName {
	case "start":
		h.handleStartMenuEdit(ctx, callback, user)
	case "lists":
		h.handleListsMenuEdit(ctx, callback, user)
	case "families":
		h.handleFamiliesMenuEdit(ctx, callback, user)
	case "help":
		h.handleHelpMenuEdit(ctx, callback, user)
	case "status":
		h.handleStatusMenuEdit(ctx, callback, user)
	case "createlist":
		h.handleCreateListMenuEdit(ctx, callback, user)
	case "createfamily":
		h.handleCreateFamilyMenuEdit(ctx, callback, user)
	case "myid":
		h.handleMyIDMenuEdit(ctx, callback, user)
	case "receipts":
		h.handleReceiptsMenuEdit(ctx, callback, user)
	case "users":
		h.handleUsersMenuEdit(ctx, callback, user)
	case "stats":
		h.handleStatsMenuEdit(ctx, callback, user)
	case "language":
		h.handleLanguageMenuEdit(ctx, callback, user)
	default:
		h.answerCallback(callback.ID, "❌ Unknown command.")
	}
}

// answerCallback sends a callback answer
func (h *MenuCallbackHandler) answerCallback(callbackID, text string) {
	h.AnswerCallback(callbackID, text)
}

// handleStartMenuEdit handles the start command by editing the existing message
func (h *MenuCallbackHandler) handleStartMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	// Create start template data
	displayName := commands.GetUserDisplayName(user)
	data := struct {
		FirstName    string
		IsAuthorized bool
	}{
		FirstName:    displayName,
		IsAuthorized: user.IsAuthorized,
	}

	// Render start message
	message, err := h.templateManager.RenderTemplate("start", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render start template", "error", err)
		h.answerCallback(callback.ID, "❌ Internal error occurred.")
		return
	}

	// Create main menu keyboard
	keyboard := h.createMainMenu(user)

	// Edit the existing message instead of sending a new one
	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)

	// Track this message for future replacements
	if h.stateManager != nil {
		h.stateManager.SetUserState(user.TelegramID, "latest_bot_message_id", fmt.Sprintf("%d:%d", callback.Message.Chat.ID, callback.Message.MessageID))
	}

	h.answerCallback(callback.ID, "")
}

// createMainMenu creates the main menu inline keyboard based on user authorization
func (h *MenuCallbackHandler) createMainMenu(user *users.User) tgbotapi.InlineKeyboardMarkup {
	var buttons [][]tgbotapi.InlineKeyboardButton

	if user.IsAuthorized {
		// Authorized user menu - using localized button templates
		buttons = [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_lists", user.Locale), "menu_lists"),
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_families", user.Locale), "menu_families"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_createlist", user.Locale), "menu_createlist"),
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_createfamily", user.Locale), "menu_createfamily"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_receipts", user.Locale), "menu_receipts"),
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_help", user.Locale), "menu_help"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_status", user.Locale), "menu_status"),
			},
		}

		// Add language button
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_language", user.Locale), "menu_language"),
		})

		// Add admin menu if user is admin
		if h.usersService.IsAdmin(user.TelegramID) {
			adminButtons := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_users", user.Locale), "menu_users"),
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_stats", user.Locale), "menu_stats"),
			}
			buttons = append(buttons, adminButtons)
		}
	} else {
		// Non-authorized user menu (limited options) - using localized button templates
		buttons = [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_help", user.Locale), "menu_help"),
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_status", user.Locale), "menu_status"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_myid", user.Locale), "menu_myid"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("menu_language", user.Locale), "menu_language"),
			},
		}
	}

	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

// handleListsMenuEdit handles the lists command by editing the existing message
func (h *MenuCallbackHandler) handleListsMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	listsWithFamilies, familyCount, err := h.shoppingService.GetUserShoppingListsWithFamilies(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user shopping lists with families", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve lists.")
		return
	}

	var message string
	var keyboard tgbotapi.InlineKeyboardMarkup

	if len(listsWithFamilies) == 0 {
		message, err = h.templateManager.RenderTemplate("no_lists", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render no lists template", "error", err)
			message = "📝 You don't have any shopping lists yet."
		}
		keyboard = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
		})
	} else {
		// Create buttons for each list
		var buttons [][]tgbotapi.InlineKeyboardButton
		showFamilyNames := familyCount > 1

		for _, listWithFamily := range listsWithFamilies {
			var buttonText string
			if showFamilyNames && listWithFamily.FamilyName != nil {
				buttonText = fmt.Sprintf("📋 %s 👥 %s", listWithFamily.Name, *listWithFamily.FamilyName)
			} else {
				buttonText = fmt.Sprintf("📋 %s", listWithFamily.Name)
			}

			button := tgbotapi.NewInlineKeyboardButtonData(buttonText, fmt.Sprintf("list_view_%s", listWithFamily.ID.String()))
			buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
		}

		// Add create new list button
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("create_new_list", user.Locale), "create_new_list"),
		})

		// Add main menu button
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
		})

		keyboard = tgbotapi.NewInlineKeyboardMarkup(buttons...)

		data := struct{ ListCount int }{ListCount: len(listsWithFamilies)}
		message, err = h.templateManager.RenderTemplate("lists_overview", user.Locale, data)
		if err != nil {
			h.logger.Error("Failed to render lists overview template", "error", err)
			message = fmt.Sprintf("📝 <b>Your Shopping Lists (%d)</b>\n\nSelect a list to view or manage:", len(listsWithFamilies))
		}
	}

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleFamiliesMenuEdit handles the families command by editing the existing message
func (h *MenuCallbackHandler) handleFamiliesMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	familiesInfo, err := h.familiesService.GetUserFamiliesWithInfo(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user families", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve families.")
		return
	}

	// Transform to display-friendly data
	type FamilyDisplayData struct {
		ID          string
		Name        string
		Description *string
		MemberCount int
		Role        string
	}

	var displayFamilies []FamilyDisplayData
	for _, info := range familiesInfo {
		displayData := FamilyDisplayData{
			ID:          info.Family.ID.String(),
			Name:        info.Family.Name,
			Description: info.Family.Description,
			MemberCount: info.MemberCount,
			Role:        info.UserRole,
		}
		displayFamilies = append(displayFamilies, displayData)
	}

	data := struct{ Families []FamilyDisplayData }{Families: displayFamilies}
	message, err := h.templateManager.RenderTemplate("families_list", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render families list template", "error", err)
		message = "👥 Your families will appear here."
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleHelpMenuEdit handles the help command by editing the existing message
func (h *MenuCallbackHandler) handleHelpMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	data := struct {
		IsAuthorized bool
		IsAdmin      bool
	}{
		IsAuthorized: user.IsAuthorized,
		IsAdmin:      h.usersService.IsAdmin(user.TelegramID),
	}

	message, err := h.templateManager.RenderTemplate("help", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render help template", "error", err)
		message = "❓ Help information will appear here."
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleStatusMenuEdit handles the status command by editing the existing message
func (h *MenuCallbackHandler) handleStatusMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	var lastName, username string
	if user.LastName != nil {
		lastName = *user.LastName
	}
	if user.Username != nil {
		username = *user.Username
	}

	data := struct {
		FirstName    string
		LastName     string
		Username     string
		TelegramID   int64
		IsAuthorized bool
		AuthorizedAt interface{}
	}{
		FirstName:    user.FirstName,
		LastName:     lastName,
		Username:     username,
		TelegramID:   user.TelegramID,
		IsAuthorized: user.IsAuthorized,
		AuthorizedAt: user.AuthorizedAt,
	}

	message, err := h.templateManager.RenderTemplate("status", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render status template", "error", err)
		message = "📊 Status information will appear here."
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleCreateListMenuEdit handles the createlist command by editing the existing message
func (h *MenuCallbackHandler) handleCreateListMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	families, err := h.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user families", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve families.")
		return
	}

	var message string
	var keyboard tgbotapi.InlineKeyboardMarkup

	if len(families) == 0 {
		message, err = h.templateManager.RenderTemplate("createlist_no_families", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render no families template", "error", err)
			message = "❌ You need to be part of a family to create shopping lists.\n\nUse /createfamily to create a family first."
		}
		keyboard = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
		})
	} else {
		var buttons [][]tgbotapi.InlineKeyboardButton
		for _, family := range families {
			button := tgbotapi.NewInlineKeyboardButtonData("🏠 "+family.Name, "createlist_"+family.ID.String())
			buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
		}

		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
		})

		keyboard = tgbotapi.NewInlineKeyboardMarkup(buttons...)

		data := struct{ FamilyCount int }{FamilyCount: len(families)}
		message, err = h.templateManager.RenderTemplate("createlist_family_selection", user.Locale, data)
		if err != nil {
			h.logger.Error("Failed to render family selection template", "error", err)
			message = "📋 <b>Create New Shopping List</b>\n\nSelect the family for your new shopping list:"
		}
	}

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleCreateFamilyMenuEdit handles the createfamily command by editing the existing message
func (h *MenuCallbackHandler) handleCreateFamilyMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	message, err := h.templateManager.RenderTemplate("createfamily_usage", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render createfamily usage template", "error", err)
		message = "🏠 <b>Create Family</b>\n\nTo create a family, use the command:\n<code>/createfamily [Family Name] [Description]</code>\n\nExample:\n<code>/createfamily \"My Family\" \"Our family shopping lists\"</code>"
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleMyIDMenuEdit handles the myid command by editing the existing message
func (h *MenuCallbackHandler) handleMyIDMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	data := struct {
		TelegramID int64
		UserID     string
		FirstName  string
	}{
		TelegramID: user.TelegramID,
		UserID:     user.ID.String(),
		FirstName:  user.FirstName,
	}

	message, err := h.templateManager.RenderTemplate("myid", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render myid template", "error", err)
		message = "🆔 Your ID information will appear here."
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleReceiptsMenuEdit handles the receipts command by editing the existing message
func (h *MenuCallbackHandler) handleReceiptsMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	// Render receipts menu directly instead of executing command
	data := struct {
		IsAuthorized bool
		UserName     string
	}{
		IsAuthorized: user.IsAuthorized,
		UserName:     user.FirstName,
	}

	// Render receipts menu message
	message, err := h.templateManager.RenderTemplate("receipts_menu", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render receipts menu template", "error", err)
		// Use template system for error messages
		errorMsg, _ := h.templateManager.RenderTemplate("receipts_list_error", user.Locale, nil)
		if errorMsg == "" {
			errorMsg, _ = h.templateManager.RenderTemplate("receipts_list_error", "en", nil)
		}
		h.answerCallback(callback.ID, errorMsg)
		return
	}

	// Create receipts menu keyboard
	keyboard := h.createReceiptsMenuKeyboard(user.Locale)

	// Edit the existing message
	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "🧾 Receipts menu")
}

// handleUsersMenuEdit handles the users command by editing the existing message (admin only)
func (h *MenuCallbackHandler) handleUsersMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	// Check admin access
	if !h.usersService.IsAdmin(user.TelegramID) {
		h.answerCallback(callback.ID, "❌ Access denied.")
		return
	}

	userList, err := h.usersService.GetAllUsers(ctx)
	if err != nil {
		h.logger.Error("Failed to get all users", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve users.")
		return
	}

	data := struct {
		Users interface{}
	}{
		Users: userList,
	}

	message, err := h.templateManager.RenderTemplate("users_list", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render users list template", "error", err)
		message = "👥 <b>User Management</b>\n\nUsers list will appear here."
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleStatsMenuEdit handles the stats command by editing the existing message (admin only)
func (h *MenuCallbackHandler) handleStatsMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	// Check admin access
	if !h.usersService.IsAdmin(user.TelegramID) {
		h.answerCallback(callback.ID, "❌ Access denied.")
		return
	}

	// Get actual counts from database
	totalUsers, err := h.usersService.GetTotalUsersCount(ctx)
	if err != nil {
		h.logger.Error("Failed to get total users count", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve statistics.")
		return
	}

	authorizedUsers, err := h.usersService.GetAuthorizedUsersCount(ctx)
	if err != nil {
		h.logger.Error("Failed to get authorized users count", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve statistics.")
		return
	}

	totalFamilies, err := h.familiesService.GetTotalFamiliesCount(ctx)
	if err != nil {
		h.logger.Error("Failed to get total families count", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve statistics.")
		return
	}

	totalShoppingLists, err := h.shoppingService.GetTotalShoppingListsCount(ctx)
	if err != nil {
		h.logger.Error("Failed to get total shopping lists count", "error", err)
		h.answerCallback(callback.ID, "❌ Failed to retrieve statistics.")
		return
	}

	// Build stats message with actual counts
	statsMessage := "📊 <b>Bot Statistics</b>\n\n"
	statsMessage += fmt.Sprintf("👥 <b>Total Users:</b> %d\n", totalUsers)
	statsMessage += fmt.Sprintf("✅ <b>Authorized Users:</b> %d\n", authorizedUsers)
	statsMessage += fmt.Sprintf("👥 <b>Total Families:</b> %d\n", totalFamilies)
	statsMessage += fmt.Sprintf("📝 <b>Shopping Lists:</b> %d\n", totalShoppingLists)
	statsMessage += "\n🤖 <b>System:</b> Running\n"
	statsMessage += "📡 <b>API:</b> Telegram Bot API\n"

	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
	})

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, statsMessage, keyboard)
	h.answerCallback(callback.ID, "")
}

// handleLanguageMenuEdit handles the language command by editing the existing message
func (h *MenuCallbackHandler) handleLanguageMenuEdit(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	// Build language selection message
	message := "🌐 <b>Choose your language / Оберіть мову / Выберите язык</b>\n\n"
	message += "Select your preferred language for the bot interface:"

	// Create language buttons
	languageButtons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("🇺🇸 English", "lang_en"),
			tgbotapi.NewInlineKeyboardButtonData("🇺🇦 Українська", "lang_uk"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang_ru"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData(h.templateManager.RenderButton("main_menu", user.Locale), "menu_start"),
		},
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(languageButtons...)

	h.EditMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, message, keyboard)
	h.answerCallback(callback.ID, "")
}

// createReceiptsMenuKeyboard creates the receipts menu inline keyboard
func (h *MenuCallbackHandler) createReceiptsMenuKeyboard(locale string) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		// First row: Upload Receipt
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("upload_receipt", locale),
				"receipts:upload",
			),
		},
		// Second row: View Receipts
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("view_receipts", locale),
				"receipts:view",
			),
		},
		// Third row: Tax Summary and Statistics
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("tax_summary", locale),
				"receipts:taxes",
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("receipt_stats", locale),
				"receipts:stats",
			),
		},
		// Fourth row: Main Menu
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("main_menu", locale),
				"menu_start",
			),
		},
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
