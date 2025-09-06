package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/families"
	"github.com/PocketPalCo/shopping-service/internal/core/shopping"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type UserState struct {
	Value     string
	ExpiresAt time.Time
}

type BotService struct {
	bot             *tgbotapi.BotAPI
	usersService    *users.Service
	familiesService *families.Service
	shoppingService *shopping.Service
	logger          *slog.Logger
	templateManager *TemplateManager
	userStates      map[string]*UserState // key: "telegramID:state"
	statesMutex     sync.RWMutex
}

func NewBotService(token string, usersService *users.Service, familiesService *families.Service, shoppingService *shopping.Service, logger *slog.Logger, debug bool) (*BotService, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	bot.Debug = debug

	templateManager, err := NewTemplateManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create template manager: %w", err)
	}

	return &BotService{
		bot:             bot,
		usersService:    usersService,
		familiesService: familiesService,
		shoppingService: shoppingService,
		logger:          logger,
		templateManager: templateManager,
		userStates:      make(map[string]*UserState),
		statesMutex:     sync.RWMutex{},
	}, nil
}

func (s *BotService) Start(ctx context.Context) error {
	s.logger.Info("Starting Telegram bot", "bot_username", s.bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Bot context cancelled, stopping")
			s.bot.StopReceivingUpdates()
			return ctx.Err()
		case update := <-updates:
			if update.Message != nil {
				go s.handleMessage(ctx, update.Message)
			} else if update.CallbackQuery != nil {
				go s.handleCallbackQuery(ctx, update.CallbackQuery)
			}
		}
	}
}

func (s *BotService) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	user := message.From
	chatID := message.Chat.ID
	text := message.Text

	s.logger.Info("Received message",
		"component", "telegram_bot",
		"user_id", user.ID,
		"username", user.UserName,
		"message", text,
		"chat_id", chatID,
		"message_type", func() string {
			if message.IsCommand() {
				return "command"
			}
			return "text"
		}(),
		"command", func() string {
			if message.IsCommand() {
				return message.Command()
			}
			return ""
		}())

	// Check if user exists
	dbUser, err := s.usersService.GetUserByTelegramID(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user", "error", err)
		s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return
	}

	// Create user if doesn't exist
	isNewUser := false
	if dbUser == nil {
		isNewUser = true
		authReq := users.AuthorizationRequest{
			TelegramID:   user.ID,
			Username:     &user.UserName,
			FirstName:    user.FirstName,
			LastName:     &user.LastName,
			LanguageCode: &user.LanguageCode,
		}

		dbUser, err = s.usersService.CreateUser(ctx, authReq)
		if err != nil {
			s.logger.Error("Failed to create user", "error", err)
			s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
			return
		}
	}

	// If this is a new user trying to start the bot, notify admins
	if isNewUser && message.IsCommand() && message.Command() == "start" {
		s.notifyAdminsNewUser(ctx, dbUser)
	}

	// Handle commands
	if message.IsCommand() {
		s.handleCommand(ctx, message, dbUser)
		return
	}

	// For non-commands, check if user is authorized
	if !dbUser.IsAuthorized {
		s.sendUnauthorizedMessage(chatID, dbUser)
		return
	}

	// Handle regular messages for authorized users
	s.handleAuthorizedMessage(ctx, message, dbUser)
}

func (s *BotService) handleCommand(ctx context.Context, message *tgbotapi.Message, user *users.User) {
	command := message.Command()
	args := message.CommandArguments()
	chatID := message.Chat.ID

	switch command {
	case "start":
		s.handleStartCommand(ctx, chatID, user)
	case "help":
		s.handleHelpCommand(chatID, user)
	case "status":
		s.handleStatusCommand(ctx, chatID, user)
	case "myid":
		s.handleMyIdCommand(chatID, user)
	case "authorize":
		if s.usersService.IsAdmin(user.TelegramID) {
			s.handleAuthorizeCommand(ctx, chatID, args)
		} else {
			s.sendMessage(chatID, "❌ You don't have permission to use this command.")
		}
	case "revoke":
		if s.usersService.IsAdmin(user.TelegramID) {
			s.handleRevokeCommand(ctx, chatID, args)
		} else {
			s.sendMessage(chatID, "❌ You don't have permission to use this command.")
		}
	case "users":
		if s.usersService.IsAdmin(user.TelegramID) {
			s.handleUsersCommand(ctx, chatID, user)
		} else {
			s.sendMessage(chatID, "❌ You don't have permission to use this command.")
		}
	case "stats":
		if s.usersService.IsAdmin(user.TelegramID) {
			s.handleStatsCommand(ctx, chatID)
		} else {
			s.sendMessage(chatID, "❌ You don't have permission to use this command.")
		}
	case "families":
		if user.IsAuthorized {
			s.handleFamiliesCommand(ctx, chatID, user)
		} else {
			s.sendUnauthorizedMessage(chatID, user)
		}
	case "createlist":
		if user.IsAuthorized {
			s.handleCreateListCommand(ctx, chatID, user)
		} else {
			s.sendUnauthorizedMessage(chatID, user)
		}
	case "lists":
		if user.IsAuthorized {
			s.handleListsCommand(ctx, chatID, user)
		} else {
			s.sendUnauthorizedMessage(chatID, user)
		}
	case "createfamily":
		if s.usersService.IsAdmin(user.TelegramID) {
			s.handleCreateFamilyCommand(ctx, chatID, user, args)
		} else {
			s.sendMessage(chatID, "❌ Only admins can create families.")
		}
	case "addfamilymember":
		if s.usersService.IsAdmin(user.TelegramID) {
			s.handleAddFamilyMemberCommand(ctx, chatID, user, args)
		} else {
			s.sendMessage(chatID, "❌ Only admins can add family members.")
		}
	default:
		s.sendMessage(chatID, "❓ Unknown command. Use /help to see available commands.")
	}
}

func (s *BotService) handleStartCommand(ctx context.Context, chatID int64, user *users.User) {
	data := StartTemplateData{
		FirstName:    s.getUserDisplayName(user),
		IsAuthorized: user.IsAuthorized,
	}

	message, err := s.templateManager.RenderTemplate("start", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render start template", "error", err)
		s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return
	}

	s.sendMessage(chatID, message)
}

func (s *BotService) handleHelpCommand(chatID int64, user *users.User) {
	data := HelpTemplateData{
		IsAuthorized: user.IsAuthorized,
		IsAdmin:      s.usersService.IsAdmin(user.TelegramID),
	}

	message, err := s.templateManager.RenderTemplate("help", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render help template", "error", err)
		s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return
	}

	s.sendMessage(chatID, message)
}

func (s *BotService) handleStatusCommand(ctx context.Context, chatID int64, user *users.User) {
	data := StatusTemplateData{
		FirstName: user.FirstName,
		LastName: func() string {
			if user.LastName != nil {
				return *user.LastName
			}
			return ""
		}(),
		Username: func() string {
			if user.Username != nil {
				return *user.Username
			}
			return ""
		}(),
		TelegramID:   user.TelegramID,
		IsAuthorized: user.IsAuthorized,
		AuthorizedAt: user.AuthorizedAt,
	}

	message, err := s.templateManager.RenderTemplate("status", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render status template", "error", err)
		s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return
	}

	s.sendMessage(chatID, message)
}

func (s *BotService) handleMyIdCommand(chatID int64, user *users.User) {
	data := MyIDTemplateData{
		TelegramID: user.TelegramID,
	}

	message, err := s.templateManager.RenderTemplate("myid", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render myid template", "error", err)
		s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return
	}

	s.sendMessage(chatID, message)
}

func (s *BotService) handleStatsCommand(ctx context.Context, chatID int64) {
	// Placeholder for stats command - can be enhanced later
	statsMessage := `📊 <b>Bot Statistics</b>

🚧 <i>Statistics feature coming soon!</i>

<b>Future stats will include:</b>
• Total registered users
• Active authorized users  
• Daily/weekly usage metrics
• Most used commands
• Session statistics

💡 Use /users to see current authorized users`

	s.sendMessage(chatID, statsMessage)
}

func (s *BotService) handleAuthorizeCommand(ctx context.Context, chatID int64, args string) {
	if args == "" {
		s.sendMessage(chatID, "❌ Please provide a Telegram ID. Usage: /authorize <telegram_id>")
		return
	}

	telegramID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		s.sendMessage(chatID, "❌ Invalid Telegram ID format.")
		return
	}

	err = s.usersService.AuthorizeUser(ctx, telegramID)
	if err != nil {
		s.logger.Error("Failed to authorize user", "error", err, "telegram_id", telegramID)
		s.sendMessage(chatID, "❌ Failed to authorize user.")
		return
	}

	s.sendMessage(chatID, fmt.Sprintf("✅ User %d has been authorized.", telegramID))
}

func (s *BotService) handleRevokeCommand(ctx context.Context, chatID int64, args string) {
	if args == "" {
		s.sendMessage(chatID, "❌ Please provide a Telegram ID. Usage: /revoke <telegram_id>")
		return
	}

	telegramID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		s.sendMessage(chatID, "❌ Invalid Telegram ID format.")
		return
	}

	err = s.usersService.RevokeUser(ctx, telegramID)
	if err != nil {
		s.logger.Error("Failed to revoke authorization", "error", err, "telegram_id", telegramID)
		s.sendMessage(chatID, "❌ Failed to revoke authorization.")
		return
	}

	s.sendMessage(chatID, fmt.Sprintf("✅ Authorization revoked for user %d.", telegramID))
}

func (s *BotService) handleUsersCommand(ctx context.Context, chatID int64, adminUser *users.User) {
	userList, err := s.usersService.GetAllUsers(ctx)
	if err != nil {
		s.logger.Error("Failed to get authorized users", "error", err)
		s.sendMessage(chatID, "❌ Failed to retrieve users.")
		return
	}

	// Convert users.User slice to User slice for template
	templateUsers := make([]User, len(userList))
	for i, u := range userList {
		templateUsers[i] = User{
			ID:           u.ID,
			TelegramID:   u.TelegramID,
			Username:     u.Username,
			FirstName:    &u.FirstName,
			LastName:     u.LastName,
			Locale:       u.Locale,
			IsAuthorized: u.IsAuthorized,
			AuthorizedBy: u.AuthorizedBy,
			AuthorizedAt: u.AuthorizedAt,
			CreatedAt:    u.CreatedAt,
			UpdatedAt:    u.UpdatedAt,
		}
	}

	data := UsersListTemplateData{
		Users: templateUsers,
	}

	message, err := s.templateManager.RenderTemplate("users_list", adminUser.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render users list template", "error", err)
		s.sendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return
	}

	s.sendMessage(chatID, message)
}

func (s *BotService) handleAuthorizedMessage(ctx context.Context, message *tgbotapi.Message, user *users.User) {
	// Check if user is in a state where they're creating a shopping list
	if familyIDStr, hasState := s.getUserState(user.TelegramID, "creating_list_for_family"); hasState {
		s.handleShoppingListNameInput(ctx, message, user, familyIDStr)
		return
	}

	// Check if user is adding an item to a list
	if listIDStr, hasState := s.getUserState(user.TelegramID, "adding_item_to_list"); hasState {
		s.handleAddItemInput(ctx, message, user, listIDStr)
		return
	}

	// For now, just acknowledge other messages
	s.sendMessage(message.Chat.ID, "✅ Message received! Use /lists to manage your shopping lists or /help to see available commands.")
}

func (s *BotService) sendUnauthorizedMessage(chatID int64, user *users.User) {
	message := fmt.Sprintf(`🔒 Access Denied

Hello %s! You need to be authorized to use this bot.

Your Telegram ID: %d

Please contact an administrator to request access.

Available commands: /start, /help, /status`,
		s.getUserDisplayName(user), user.TelegramID)

	s.sendMessage(chatID, message)
}

func (s *BotService) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send message", "error", err, "chat_id", chatID)
	}
}

func (s *BotService) getUserDisplayName(user *users.User) string {
	if user.FirstName != "" {
		if user.LastName != nil && *user.LastName != "" {
			return fmt.Sprintf("%s %s", user.FirstName, *user.LastName)
		}
		return user.FirstName
	}
	if user.Username != nil && *user.Username != "" {
		return "@" + *user.Username
	}
	return fmt.Sprintf("User_%d", user.TelegramID)
}

func (s *BotService) Stop() {
	s.bot.StopReceivingUpdates()
}

// notifyAdminsNewUser sends a notification to all admins about a new user request
func (s *BotService) notifyAdminsNewUser(ctx context.Context, user *users.User) {
	// Get all admin users from the users service (this would need to be implemented)
	// For now, we'll skip this notification feature in the refactored version
	// TODO: Implement admin notification system
	s.logger.Info("New user created, admin notification skipped in refactored version",
		"component", "telegram_bot",
		"new_user_id", user.TelegramID)
	return
}

// handleCallbackQuery processes inline keyboard button presses
func (s *BotService) handleCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	s.logger.Info("Received callback query",
		"component", "telegram_bot",
		"user_id", callback.From.ID,
		"data", callback.Data,
		"chat_id", callback.Message.Chat.ID)

	// Parse callback data
	parts := strings.Split(callback.Data, "_")
	if len(parts) < 2 {
		s.answerCallback(callback.ID, "❌ Invalid action.")
		return
	}

	actionType := parts[0]

	switch actionType {
	case "createlist":
		// Handle shopping list creation callbacks
		s.handleCreateListCallback(ctx, callback, parts)
	case "list":
		// Handle shopping list management callbacks
		s.handleListCallback(ctx, callback, parts)
	case "li":
		// Handle compact list item callbacks (li_t_listID_itemID)
		s.handleCompactListItemCallback(ctx, callback, parts)
	case "show":
		// Handle show callbacks (like show_all_lists)
		s.handleShowCallback(ctx, callback, parts)
	case "create":
		// Handle create callbacks (like create_new_list)
		s.handleCreateCallback(ctx, callback, parts)
	case "user":
		// Handle admin user authorization callbacks
		s.handleUserAuthorizationCallback(ctx, callback, parts)
	default:
		s.answerCallback(callback.ID, "❌ Unknown action.")
	}
}

// handleUserAuthorizationCallback handles admin user authorization callbacks
func (s *BotService) handleUserAuthorizationCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	// Check if user is admin
	if !s.usersService.IsAdmin(callback.From.ID) {
		s.answerCallback(callback.ID, "❌ You don't have permission to perform this action.")
		return
	}

	if len(parts) < 3 {
		s.answerCallback(callback.ID, "❌ Invalid action.")
		return
	}

	action := parts[1] // approve or deny
	targetUserIDStr := parts[2]

	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		s.answerCallback(callback.ID, "❌ Invalid user ID.")
		return
	}

	switch action {
	case "approve":
		s.handleAuthorizationApproval(ctx, callback, targetUserID)
	case "deny":
		s.handleAuthorizationDenial(ctx, callback, targetUserID)
	default:
		s.answerCallback(callback.ID, "❌ Unknown action.")
	}
}

// handleAuthorizationApproval approves a user's authorization request
func (s *BotService) handleAuthorizationApproval(ctx context.Context, callback *tgbotapi.CallbackQuery, targetUserID int64) {
	err := s.usersService.AuthorizeUser(ctx, targetUserID)
	if err != nil {
		s.logger.Error("Failed to authorize user via callback",
			"component", "telegram_bot",
			"error", err,
			"admin_id", callback.From.ID,
			"target_user_id", targetUserID)
		s.answerCallback(callback.ID, "❌ Failed to authorize user.")
		return
	}

	// Get target user info for template
	targetUser, err := s.usersService.GetUserByTelegramID(ctx, targetUserID)
	if err != nil {
		s.logger.Error("Failed to get target user for template", "error", err)
		s.answerCallback(callback.ID, "❌ Internal error occurred.")
		return
	}
	if targetUser == nil {
		s.logger.Error("Target user not found for template")
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}

	// Update the original message to show approval
	data := AdminAuthorizationTemplateData{
		FirstName: targetUser.FirstName,
		LastName: func() string {
			if targetUser.LastName != nil {
				return *targetUser.LastName
			}
			return ""
		}(),
		Username: func() string {
			if targetUser.Username != nil && *targetUser.Username != "" {
				return *targetUser.Username
			}
			return ""
		}(),
	}

	editedMessage, err := s.templateManager.RenderTemplate("admin_authorization_success", "en", data)
	if err != nil {
		s.logger.Error("Failed to render admin authorization success template", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to update message.")
		return
	}

	editMsg := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, editedMessage)
	editMsg.ParseMode = tgbotapi.ModeHTML
	s.bot.Send(editMsg)

	// Notify the authorized user
	s.notifyUserAuthorized(ctx, targetUserID)

	s.answerCallback(callback.ID, "✅ User authorized successfully!")

	s.logger.Info("User authorized via admin callback",
		"component", "telegram_bot",
		"admin_id", callback.From.ID,
		"target_user_id", targetUserID)
}

// handleAuthorizationDenial denies a user's authorization request
func (s *BotService) handleAuthorizationDenial(ctx context.Context, callback *tgbotapi.CallbackQuery, targetUserID int64) {
	// Get target user info for template
	targetUser, err := s.usersService.GetUserByTelegramID(ctx, targetUserID)
	if err != nil {
		s.logger.Error("Failed to get target user for template", "error", err)
		s.answerCallback(callback.ID, "❌ Internal error occurred.")
		return
	}
	if targetUser == nil {
		s.logger.Error("Target user not found for template")
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}

	// Update the original message to show denial
	data := AdminAuthorizationTemplateData{
		FirstName: targetUser.FirstName,
		LastName: func() string {
			if targetUser.LastName != nil {
				return *targetUser.LastName
			}
			return ""
		}(),
		Username: func() string {
			if targetUser.Username != nil && *targetUser.Username != "" {
				return *targetUser.Username
			}
			return ""
		}(),
	}

	editedMessage, err := s.templateManager.RenderTemplate("admin_authorization_denied", "en", data)
	if err != nil {
		s.logger.Error("Failed to render admin authorization denied template", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to update message.")
		return
	}

	editMsg := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, editedMessage)
	editMsg.ParseMode = tgbotapi.ModeHTML
	s.bot.Send(editMsg)

	s.answerCallback(callback.ID, "❌ User access denied.")

	s.logger.Info("User authorization denied via admin callback",
		"component", "telegram_bot",
		"admin_id", callback.From.ID,
		"target_user_id", targetUserID)
}

// notifyUserAuthorized sends a notification to the newly authorized user
func (s *BotService) notifyUserAuthorized(ctx context.Context, userID int64) {
	// Get target user info for template
	targetUser, err := s.usersService.GetUserByTelegramID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get target user for authorization notification", "error", err)
		return
	}
	if targetUser == nil {
		s.logger.Error("Target user not found for authorization notification")
		return
	}

	data := AuthorizationSuccessTemplateData{
		FirstName: targetUser.FirstName,
	}

	message, err := s.templateManager.RenderTemplate("authorization_success", targetUser.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render authorization success template", "error", err)
		return
	}

	msg := tgbotapi.NewMessage(userID, message)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send authorization notification to user",
			"component", "telegram_bot",
			"error", err,
			"user_id", userID)
	} else {
		s.logger.Info("Authorization notification sent to user",
			"component", "telegram_bot",
			"user_id", userID)
	}
}

// answerCallback sends a callback query answer
func (s *BotService) answerCallback(callbackQueryID, text string) {
	callback := tgbotapi.NewCallback(callbackQueryID, text)
	if _, err := s.bot.Request(callback); err != nil {
		s.logger.Error("Failed to answer callback query",
			"component", "telegram_bot",
			"error", err)
	}
}

// handleFamiliesCommand shows user's families
func (s *BotService) handleFamiliesCommand(ctx context.Context, chatID int64, user *users.User) {
	familiesData, err := s.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user families", "error", err)
		s.sendMessage(chatID, "❌ Failed to retrieve families.")
		return
	}

	// Convert to template format with additional data
	type FamilyData struct {
		Name        string
		Description *string
		MemberCount int
		Role        string
	}

	templateFamilies := make([]FamilyData, len(familiesData))
	for i, family := range familiesData {
		// Get member count (simplified, would need actual count from service)
		templateFamilies[i] = FamilyData{
			Name:        family.Name,
			Description: family.Description,
			MemberCount: 1,       // Placeholder - would need actual count
			Role:        "Admin", // Placeholder - would need actual role
		}
	}

	data := struct {
		Families []FamilyData
	}{
		Families: templateFamilies,
	}

	message, err := s.templateManager.RenderTemplate("families_list", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render families list template", "error", err)
		s.sendMessage(chatID, "❌ Failed to display families.")
		return
	}

	s.sendMessage(chatID, message)
}

// handleCreateFamilyCommand creates a new family
func (s *BotService) handleCreateFamilyCommand(ctx context.Context, chatID int64, user *users.User, args string) {
	if args == "" {
		message, err := s.templateManager.RenderTemplate("createfamily_usage", user.Locale, nil)
		if err != nil {
			s.logger.Error("Failed to render createfamily usage template", "error", err)
			s.sendMessage(chatID, "❌ Please provide a family name.")
			return
		}
		s.sendMessage(chatID, message)
		return
	}

	parts := strings.SplitN(args, " ", 2)
	name := strings.TrimSpace(parts[0])
	var description *string
	if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
		desc := strings.TrimSpace(parts[1])
		description = &desc
	}

	req := families.CreateFamilyRequest{
		Name:        name,
		Description: description,
		CreatedBy:   user.ID,
		MemberIDs:   []uuid.UUID{}, // Just the creator for now
	}

	family, err := s.familiesService.CreateFamily(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create family", "error", err)
		s.sendMessage(chatID, "❌ Failed to create family.")
		return
	}

	data := struct {
		Name        string
		Description *string
		CreatedAt   time.Time
	}{
		Name:        family.Name,
		Description: family.Description,
		CreatedAt:   family.CreatedAt,
	}

	message, err := s.templateManager.RenderTemplate("family_created", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render family created template", "error", err)
		s.sendMessage(chatID, "❌ Family created but failed to display details.")
		return
	}

	s.sendMessage(chatID, message)
}

// handleAddFamilyMemberCommand adds a member to a family
func (s *BotService) handleAddFamilyMemberCommand(ctx context.Context, chatID int64, user *users.User, args string) {
	if args == "" {
		message, err := s.templateManager.RenderTemplate("addfamilymember_usage", user.Locale, nil)
		if err != nil {
			s.logger.Error("Failed to render addfamilymember usage template", "error", err)
			s.sendMessage(chatID, "❌ Invalid command usage.")
			return
		}
		s.sendMessage(chatID, message)
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		message, err := s.templateManager.RenderTemplate("addfamilymember_usage", user.Locale, nil)
		if err != nil {
			s.logger.Error("Failed to render addfamilymember usage template", "error", err)
			s.sendMessage(chatID, "❌ Invalid command usage.")
			return
		}
		s.sendMessage(chatID, message)
		return
	}

	familyName := parts[0]
	memberIdentifier := parts[1]

	// First, find the family by name (simplified - in real implementation you'd want better family identification)
	userFamilies, err := s.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user families", "error", err)
		s.sendMessage(chatID, "❌ Failed to retrieve families.")
		return
	}

	var targetFamily *families.Family
	for _, family := range userFamilies {
		if family.Name == familyName {
			targetFamily = family
			break
		}
	}

	if targetFamily == nil {
		s.sendMessage(chatID, fmt.Sprintf("❌ Family '%s' not found.", familyName))
		return
	}

	// Find the user to add
	var targetUser *users.User

	// Try to parse as Telegram ID first
	if telegramID, err := strconv.ParseInt(memberIdentifier, 10, 64); err == nil {
		targetUser, err = s.usersService.GetUserByTelegramID(ctx, telegramID)
		if err != nil {
			s.logger.Error("Failed to get user by telegram ID", "error", err)
			s.sendMessage(chatID, "❌ Failed to find user.")
			return
		}
	} else {
		// Try as username
		targetUser, err = s.usersService.GetUserByUsername(ctx, strings.TrimPrefix(memberIdentifier, "@"))
		if err != nil {
			s.logger.Error("Failed to get user by username", "error", err)
			s.sendMessage(chatID, "❌ Failed to find user.")
			return
		}
	}

	if targetUser == nil {
		s.sendMessage(chatID, "❌ User not found.")
		return
	}

	// Add the member
	err = s.familiesService.AddMemberToFamily(ctx, targetFamily.ID, targetUser.ID, user.ID)
	if err != nil {
		s.logger.Error("Failed to add member to family", "error", err)
		s.sendMessage(chatID, "❌ Failed to add member to family.")
		return
	}

	displayName := targetUser.FirstName
	if targetUser.Username != nil && *targetUser.Username != "" {
		displayName = "@" + *targetUser.Username
	}

	// Send notification to the added user
	addedByName := user.FirstName
	if user.Username != nil && *user.Username != "" {
		addedByName = "@" + *user.Username
	}

	notificationData := struct {
		FamilyName        string
		FamilyDescription *string
		AddedByName       string
		AddedAt           time.Time
	}{
		FamilyName:        targetFamily.Name,
		FamilyDescription: targetFamily.Description,
		AddedByName:       addedByName,
		AddedAt:           time.Now(),
	}

	notificationMessage, err := s.templateManager.RenderTemplate("family_member_notification", targetUser.Locale, notificationData)
	if err != nil {
		s.logger.Error("Failed to render family member notification template", "error", err)
	} else {
		s.sendMessage(targetUser.TelegramID, notificationMessage)
		s.logger.Info("Sent family member notification",
			"family_name", targetFamily.Name,
			"new_member_id", targetUser.TelegramID,
			"added_by_id", user.TelegramID)
	}

	// Send confirmation to admin
	confirmationData := struct {
		UserName   string
		FamilyName string
		AddedAt    time.Time
	}{
		UserName:   displayName,
		FamilyName: targetFamily.Name,
		AddedAt:    time.Now(),
	}

	message, err := s.templateManager.RenderTemplate("family_member_added", user.Locale, confirmationData)
	if err != nil {
		s.logger.Error("Failed to render family member added template", "error", err)
		s.sendMessage(chatID, fmt.Sprintf("✅ Successfully added %s to family '%s'.", displayName, targetFamily.Name))
		return
	}

	s.sendMessage(chatID, message)
}

// handleCreateListCommand starts the shopping list creation process
func (s *BotService) handleCreateListCommand(ctx context.Context, chatID int64, user *users.User) {
	// Get user's families
	userFamilies, err := s.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user families", "error", err)
		message, templateErr := s.templateManager.RenderTemplate("createlist_error", user.Locale, nil)
		if templateErr != nil {
			s.logger.Error("Failed to render createlist error template", "error", templateErr)
			s.sendMessage(chatID, "❌ Failed to retrieve your families.")
			return
		}
		s.sendMessage(chatID, message)
		return
	}

	if len(userFamilies) == 0 {
		message, err := s.templateManager.RenderTemplate("createlist_no_families", user.Locale, nil)
		if err != nil {
			s.logger.Error("Failed to render createlist no families template", "error", err)
			s.sendMessage(chatID, "❌ You need to be a member of at least one family to create shopping lists.")
			return
		}
		s.sendMessage(chatID, message)
		return
	}

	// Create inline keyboard with families
	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, family := range userFamilies {
		button := tgbotapi.NewInlineKeyboardButtonData(family.Name, fmt.Sprintf("createlist_family_%s", family.ID.String()))
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	message, err := s.templateManager.RenderTemplate("createlist_family_selection", user.Locale, nil)
	if err != nil {
		s.logger.Error("Failed to render createlist family selection template", "error", err)
		message = "🛒 Create Shopping List\n\nPlease select a family for this shopping list:"
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send family selection message", "error", err, "chat_id", chatID)
	}
}

// handleCreateListCallback handles the family selection for shopping list creation
func (s *BotService) handleCreateListCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) < 3 {
		s.answerCallback(callback.ID, "❌ Invalid family selection.")
		return
	}

	if parts[1] != "family" {
		s.answerCallback(callback.ID, "❌ Invalid action.")
		return
	}

	familyIDStr := parts[2]
	familyID, err := uuid.Parse(familyIDStr)
	if err != nil {
		s.logger.Error("Failed to parse family ID", "error", err, "family_id", familyIDStr)
		s.answerCallback(callback.ID, "❌ Invalid family ID.")
		return
	}

	// Get user from database
	user, err := s.usersService.GetUserByTelegramID(ctx, callback.From.ID)
	if err != nil {
		s.logger.Error("Failed to get user", "error", err, "telegram_id", callback.From.ID)
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}

	// Verify user is member of this family
	userFamilies, err := s.familiesService.GetUserFamilies(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user families", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to verify family membership.")
		return
	}

	var selectedFamily *families.Family
	for _, family := range userFamilies {
		if family.ID == familyID {
			selectedFamily = family
			break
		}
	}

	if selectedFamily == nil {
		s.answerCallback(callback.ID, "❌ You are not a member of this family.")
		return
	}

	// Ask for shopping list name
	s.answerCallback(callback.ID, "Family selected!")

	// Render the template for asking list name
	data := struct {
		FamilyName string
	}{
		FamilyName: selectedFamily.Name,
	}

	message, err := s.templateManager.RenderTemplate("createlist_enter_name", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render createlist enter name template", "error", err)
		message = fmt.Sprintf("🛒 <b>Creating list for: %s</b>\n\nPlease reply with the name for your shopping list:", selectedFamily.Name)
	}

	// Edit the original message to ask for list name
	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		message,
	)
	editMsg.ParseMode = tgbotapi.ModeHTML

	if _, err := s.bot.Send(editMsg); err != nil {
		s.logger.Error("Failed to edit message", "error", err)
	}

	// Store the selected family ID in a temporary state (for now, we'll use a simple approach)
	// In a real implementation, you'd want to store this in Redis or database
	s.storeUserState(user.TelegramID, "creating_list_for_family", familyIDStr)
}

// storeUserState stores temporary user state with expiration
func (s *BotService) storeUserState(telegramID int64, state, value string) {
	s.statesMutex.Lock()
	defer s.statesMutex.Unlock()

	key := fmt.Sprintf("%d:%s", telegramID, state)
	s.userStates[key] = &UserState{
		Value:     value,
		ExpiresAt: time.Now().Add(30 * time.Minute), // Expire after 30 minutes
	}

	s.logger.Info("Storing user state", "telegram_id", telegramID, "state", state, "value", value)
}

// getUserState retrieves temporary user state and checks if it's still valid
func (s *BotService) getUserState(telegramID int64, state string) (string, bool) {
	s.statesMutex.RLock()
	defer s.statesMutex.RUnlock()

	key := fmt.Sprintf("%d:%s", telegramID, state)
	userState, exists := s.userStates[key]

	if !exists {
		return "", false
	}

	// Check if state has expired
	if time.Now().After(userState.ExpiresAt) {
		// Clean up expired state (will be done properly in a cleanup goroutine in production)
		go s.cleanupExpiredState(key)
		return "", false
	}

	return userState.Value, true
}

// cleanupExpiredState removes expired state entries
func (s *BotService) cleanupExpiredState(key string) {
	s.statesMutex.Lock()
	defer s.statesMutex.Unlock()
	delete(s.userStates, key)
}

// handleShoppingListNameInput handles when user provides a name for their shopping list
func (s *BotService) handleShoppingListNameInput(ctx context.Context, message *tgbotapi.Message, user *users.User, familyIDStr string) {
	listName := strings.TrimSpace(message.Text)

	if listName == "" {
		errorMessage, err := s.templateManager.RenderTemplate("createlist_invalid_name", user.Locale, nil)
		if err != nil {
			s.logger.Error("Failed to render createlist invalid name template", "error", err)
			s.sendMessage(message.Chat.ID, "❌ Please provide a valid name for your shopping list.")
			return
		}
		s.sendMessage(message.Chat.ID, errorMessage)
		return
	}

	// Parse family ID
	familyID, err := uuid.Parse(familyIDStr)
	if err != nil {
		s.logger.Error("Failed to parse family ID from state", "error", err, "family_id", familyIDStr)
		errorMessage, templateErr := s.templateManager.RenderTemplate("createlist_error", user.Locale, nil)
		if templateErr != nil {
			s.logger.Error("Failed to render createlist error template", "error", templateErr)
			s.sendMessage(message.Chat.ID, "❌ Something went wrong. Please try again with /createlist.")
		} else {
			s.sendMessage(message.Chat.ID, errorMessage)
		}
		s.clearUserState(user.TelegramID, "creating_list_for_family")
		return
	}

	// Get family name for display
	family, err := s.familiesService.GetFamilyByID(ctx, familyID)
	if err != nil {
		s.logger.Error("Failed to get family by ID", "error", err, "family_id", familyID)
		errorMessage, templateErr := s.templateManager.RenderTemplate("createlist_error", user.Locale, nil)
		if templateErr != nil {
			s.logger.Error("Failed to render createlist error template", "error", templateErr)
			s.sendMessage(message.Chat.ID, "❌ Failed to create shopping list. Please try again.")
		} else {
			s.sendMessage(message.Chat.ID, errorMessage)
		}
		s.clearUserState(user.TelegramID, "creating_list_for_family")
		return
	}

	// Create shopping list using the shopping service
	req := shopping.CreateShoppingListRequest{
		Name:     listName,
		FamilyID: &familyID,
		OwnerID:  user.ID,
		IsShared: true, // Family lists are shared by default
	}

	shoppingList, err := s.shoppingService.CreateShoppingList(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create shopping list", "error", err)
		errorMessage, templateErr := s.templateManager.RenderTemplate("createlist_error", user.Locale, nil)
		if templateErr != nil {
			s.logger.Error("Failed to render createlist error template", "error", templateErr)
			s.sendMessage(message.Chat.ID, "❌ Failed to create shopping list. Please try again.")
		} else {
			s.sendMessage(message.Chat.ID, errorMessage)
		}
		return
	}

	// Clear user state
	s.clearUserState(user.TelegramID, "creating_list_for_family")

	// Send success message using template
	data := struct {
		ListName   string
		FamilyName string
		CreatedAt  time.Time
	}{
		ListName:   shoppingList.Name,
		FamilyName: family.Name,
		CreatedAt:  shoppingList.CreatedAt,
	}

	successMessage, err := s.templateManager.RenderTemplate("list_created_success", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render list created success template", "error", err)
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

	msg := tgbotapi.NewMessage(message.Chat.ID, successMessage)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send list created message with buttons", "error", err)
		// Fallback to simple message
		s.sendMessage(message.Chat.ID, successMessage)
	}

	// Notify all family members about the new list (except the creator)
	memberUserIDs, err := s.familiesService.GetFamilyMemberUserIDs(ctx, familyID, user.ID)
	if err != nil {
		s.logger.Error("Failed to get family member user IDs for notification", "error", err)
	} else if len(memberUserIDs) > 0 {
		// Get creator display name
		createdByName := user.FirstName
		if user.Username != nil && *user.Username != "" {
			createdByName = "@" + *user.Username
		}

		// Prepare notification data
		notificationData := struct {
			ListName      string
			FamilyName    string
			CreatedByName string
			CreatedAt     time.Time
		}{
			ListName:      shoppingList.Name,
			FamilyName:    family.Name,
			CreatedByName: createdByName,
			CreatedAt:     shoppingList.CreatedAt,
		}

		// Send notification to each family member
		for _, memberUserID := range memberUserIDs {
			// Get member's user details for Telegram ID and locale
			memberUser, err := s.usersService.GetUserByID(ctx, memberUserID)
			if err != nil {
				s.logger.Error("Failed to get family member details for notification", "error", err, "member_user_id", memberUserID)
				continue
			}

			// Render localized notification message
			notificationMessage, err := s.templateManager.RenderTemplate("family_new_list_notification", memberUser.Locale, notificationData)
			if err != nil {
				s.logger.Error("Failed to render family new list notification template", "error", err, "member_locale", memberUser.Locale)
				// Fallback message
				notificationMessage = fmt.Sprintf("🛒 <b>New Shopping List Created!</b>\n\n📋 <b>List:</b> %s\n🏠 <b>Family:</b> %s\n👤 <b>Created by:</b> %s\n📅 <b>Created:</b> %s\n\nThe list is now available to all family members!",
					shoppingList.Name, family.Name, createdByName, shoppingList.CreatedAt.Format("2006-01-02 15:04:05"))
			}

			// Create notification buttons so family members can immediately access the list
			notificationButtons := [][]tgbotapi.InlineKeyboardButton{
				{
					tgbotapi.NewInlineKeyboardButtonData("📋 View List", fmt.Sprintf("list_view_%s", shoppingList.ID.String())),
					tgbotapi.NewInlineKeyboardButtonData("➕ Add Item", fmt.Sprintf("list_additem_%s", shoppingList.ID.String())),
				},
				{
					tgbotapi.NewInlineKeyboardButtonData("📝 All Lists", "show_all_lists"),
				},
			}
			notificationKeyboard := tgbotapi.NewInlineKeyboardMarkup(notificationButtons...)

			// Send notification with buttons
			notificationMsg := tgbotapi.NewMessage(memberUser.TelegramID, notificationMessage)
			notificationMsg.ParseMode = tgbotapi.ModeHTML
			notificationMsg.ReplyMarkup = notificationKeyboard

			if _, err := s.bot.Send(notificationMsg); err != nil {
				s.logger.Error("Failed to send family new list notification", "error", err,
					"member_telegram_id", memberUser.TelegramID,
					"list_name", shoppingList.Name,
					"family_name", family.Name)
			} else {
				s.logger.Info("Sent family new list notification",
					"member_telegram_id", memberUser.TelegramID,
					"list_name", shoppingList.Name,
					"family_name", family.Name,
					"created_by", createdByName)
			}
		}
	}
}

// clearUserState removes temporary user state
func (s *BotService) clearUserState(telegramID int64, state string) {
	s.statesMutex.Lock()
	defer s.statesMutex.Unlock()

	key := fmt.Sprintf("%d:%s", telegramID, state)
	delete(s.userStates, key)

	s.logger.Info("Clearing user state", "telegram_id", telegramID, "state", state)
}

// notifyFamilyMembersItemEdit sends notifications to family members when list items are edited
func (s *BotService) notifyFamilyMembersItemEdit(ctx context.Context, listID uuid.UUID, actorUser *users.User, templateName string, itemName, quantity string) {
	// Get the shopping list to check if it's a family list
	list, err := s.shoppingService.GetShoppingListByID(ctx, listID)
	if err != nil {
		s.logger.Error("Failed to get shopping list for item edit notification", "error", err, "list_id", listID)
		return
	}

	// Only notify for family lists
	if list.FamilyID == nil {
		return
	}

	// Get family details
	family, err := s.familiesService.GetFamilyByID(ctx, *list.FamilyID)
	if err != nil {
		s.logger.Error("Failed to get family for item edit notification", "error", err, "family_id", *list.FamilyID)
		return
	}

	// Get family member user IDs (excluding the actor)
	memberUserIDs, err := s.familiesService.GetFamilyMemberUserIDs(ctx, *list.FamilyID, actorUser.ID)
	if err != nil {
		s.logger.Error("Failed to get family member user IDs for item edit notification", "error", err, "family_id", *list.FamilyID)
		return
	}

	if len(memberUserIDs) == 0 {
		return // No other family members to notify
	}

	// Get actor display name
	actorName := actorUser.FirstName
	if actorUser.Username != nil && *actorUser.Username != "" {
		actorName = "@" + *actorUser.Username
	}

	// Prepare notification data
	notificationData := struct {
		ListName        string
		FamilyName      string
		ItemName        string
		Quantity        string
		AddedByName     string
		CompletedByName string
		ReopenedByName  string
	}{
		ListName:        list.Name,
		FamilyName:      family.Name,
		ItemName:        itemName,
		Quantity:        quantity,
		AddedByName:     actorName,
		CompletedByName: actorName,
		ReopenedByName:  actorName,
	}

	// Send notification to each family member
	for _, memberUserID := range memberUserIDs {
		// Get member's user details for Telegram ID and locale
		memberUser, err := s.usersService.GetUserByID(ctx, memberUserID)
		if err != nil {
			s.logger.Error("Failed to get family member details for item edit notification", "error", err, "member_user_id", memberUserID)
			continue
		}

		// Render localized notification message
		notificationMessage, err := s.templateManager.RenderTemplate(templateName, memberUser.Locale, notificationData)
		if err != nil {
			s.logger.Error("Failed to render item edit notification template", "error", err,
				"template_name", templateName, "member_locale", memberUser.Locale)
			continue
		}

		// Create notification buttons for quick access to the list
		notificationButtons := [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData("📋 View List", fmt.Sprintf("list_view_%s", listID.String())),
				tgbotapi.NewInlineKeyboardButtonData("➕ Add Item", fmt.Sprintf("list_additem_%s", listID.String())),
			},
		}
		notificationKeyboard := tgbotapi.NewInlineKeyboardMarkup(notificationButtons...)

		// Send notification with buttons
		notificationMsg := tgbotapi.NewMessage(memberUser.TelegramID, notificationMessage)
		notificationMsg.ParseMode = tgbotapi.ModeHTML
		notificationMsg.ReplyMarkup = notificationKeyboard

		if _, err := s.bot.Send(notificationMsg); err != nil {
			s.logger.Error("Failed to send item edit notification", "error", err,
				"member_telegram_id", memberUser.TelegramID,
				"template_name", templateName,
				"item_name", itemName,
				"list_name", list.Name)
		} else {
			s.logger.Info("Sent item edit notification",
				"member_telegram_id", memberUser.TelegramID,
				"template_name", templateName,
				"item_name", itemName,
				"list_name", list.Name,
				"actor", actorName)
		}
	}
}

// handleListsCommand shows user's shopping lists
func (s *BotService) handleListsCommand(ctx context.Context, chatID int64, user *users.User) {
	lists, err := s.shoppingService.GetUserShoppingLists(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user shopping lists", "error", err)
		s.sendMessage(chatID, "❌ Failed to retrieve your shopping lists.")
		return
	}

	if len(lists) == 0 {
		message, err := s.templateManager.RenderTemplate("no_lists", user.Locale, nil)
		if err != nil {
			s.logger.Error("Failed to render no lists template", "error", err)
			s.sendMessage(chatID, "📝 You don't have any shopping lists yet.\n\nUse /createlist to create your first list!")
			return
		}
		s.sendMessage(chatID, message)
		return
	}

	// Create inline keyboard with lists
	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, list := range lists {
		listName := list.Name
		if len(listName) > 25 { // Truncate long names for button display
			listName = listName[:22] + "..."
		}
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("📋 %s", listName),
			fmt.Sprintf("list_view_%s", list.ID.String()),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add create new list button
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("➕ Create New List", "create_new_list"),
	})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	data := struct {
		ListCount int
	}{
		ListCount: len(lists),
	}

	message, err := s.templateManager.RenderTemplate("lists_overview", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render lists overview template", "error", err)
		message = fmt.Sprintf("📝 <b>Your Shopping Lists (%d)</b>\n\nSelect a list to view and manage items:", len(lists))
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send shopping lists message", "error", err, "chat_id", chatID)
	}
}

// handleListCallback handles shopping list management callbacks
func (s *BotService) handleListCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) < 3 {
		s.answerCallback(callback.ID, "❌ Invalid list action.")
		return
	}

	action := parts[1] // view, additem, complete, delete, etc.
	listIDStr := parts[2]

	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		s.logger.Error("Failed to parse list ID", "error", err, "list_id", listIDStr)
		s.answerCallback(callback.ID, "❌ Invalid list ID.")
		return
	}

	// Get user from callback
	user, err := s.usersService.GetUserByTelegramID(ctx, callback.From.ID)
	if err != nil {
		s.logger.Error("Failed to get user", "error", err, "telegram_id", callback.From.ID)
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}

	// Check if user can access this list
	canAccess, err := s.shoppingService.CanUserAccessList(ctx, listID, user.ID)
	if err != nil {
		s.logger.Error("Failed to check list access", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to verify list access.")
		return
	}

	if !canAccess {
		s.answerCallback(callback.ID, "❌ You don't have access to this list.")
		return
	}

	switch action {
	case "view":
		s.handleViewListCallback(ctx, callback, listID, user)
	case "additem":
		s.handleAddItemCallback(ctx, callback, listID, user)
	case "toggleitem":
		if len(parts) >= 4 {
			itemIDStr := parts[3]
			s.handleToggleItemCallback(ctx, callback, listID, itemIDStr, user)
		} else {
			s.answerCallback(callback.ID, "❌ Invalid item action.")
		}
	case "deleteitem":
		if len(parts) >= 4 {
			itemIDStr := parts[3]
			s.handleDeleteItemCallback(ctx, callback, listID, itemIDStr, user)
		} else {
			s.answerCallback(callback.ID, "❌ Invalid item action.")
		}
	default:
		s.answerCallback(callback.ID, "❌ Unknown list action.")
	}
}

// handleCompactListItemCallback handles compact list item callbacks (li_t_listID_itemID)
func (s *BotService) handleCompactListItemCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) < 4 {
		s.answerCallback(callback.ID, "❌ Invalid compact callback.")
		return
	}

	// Expected format: li_t_shortListID_shortItemID
	action := parts[1] // should be "t" for toggle
	shortListID := parts[2]
	shortItemID := parts[3]

	if action != "t" {
		s.answerCallback(callback.ID, "❌ Unknown compact action.")
		return
	}

	// Get user from callback
	user, err := s.usersService.GetUserByTelegramID(ctx, callback.From.ID)
	if err != nil {
		s.logger.Error("Failed to get user", "error", err, "telegram_id", callback.From.ID)
		s.answerCallback(callback.ID, "❌ User not found.")
		return
	}

	// Find the full UUIDs by searching through user's lists and items
	lists, err := s.shoppingService.GetUserShoppingLists(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to get user shopping lists", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to load lists.")
		return
	}

	var targetListID, targetItemID uuid.UUID
	var found bool

	// Find the matching list and item
	for _, list := range lists {
		if strings.HasPrefix(list.ID.String(), shortListID) {
			targetListID = list.ID

			// Get items for this list
			items, err := s.shoppingService.GetListItems(ctx, list.ID)
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
		s.answerCallback(callback.ID, "❌ Item not found.")
		return
	}

	// Check if user can access this list
	canAccess, err := s.shoppingService.CanUserAccessList(ctx, targetListID, user.ID)
	if err != nil {
		s.logger.Error("Failed to check list access", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to verify list access.")
		return
	}

	if !canAccess {
		s.answerCallback(callback.ID, "❌ You don't have access to this list.")
		return
	}

	// Handle the toggle action
	s.handleToggleItemCallback(ctx, callback, targetListID, targetItemID.String(), user)
}

// handleShowCallback handles show callbacks like show_all_lists
func (s *BotService) handleShowCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) < 2 {
		s.answerCallback(callback.ID, "❌ Invalid show action.")
		return
	}

	// Reconstruct the full action (handle underscores)
	action := strings.Join(parts[1:], "_")

	switch action {
	case "all_lists":
		// Get user from callback
		user, err := s.usersService.GetUserByTelegramID(ctx, callback.From.ID)
		if err != nil {
			s.logger.Error("Failed to get user", "error", err, "telegram_id", callback.From.ID)
			s.answerCallback(callback.ID, "❌ User not found.")
			return
		}

		s.answerCallback(callback.ID, "Loading lists...")
		s.handleListsCommand(ctx, callback.Message.Chat.ID, user)
	default:
		s.answerCallback(callback.ID, "❌ Unknown show action.")
	}
}

// handleViewListCallback shows a shopping list with its items and action buttons
func (s *BotService) handleViewListCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, user *users.User) {
	// Get the shopping list
	list, err := s.shoppingService.GetShoppingListByID(ctx, listID)
	if err != nil || list == nil {
		s.logger.Error("Failed to get shopping list", "error", err, "list_id", listID)
		s.answerCallback(callback.ID, "❌ List not found.")
		return
	}

	// Get list items
	items, err := s.shoppingService.GetListItems(ctx, listID)
	if err != nil {
		s.logger.Error("Failed to get list items", "error", err, "list_id", listID)
		s.answerCallback(callback.ID, "❌ Failed to load list items.")
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
				itemText += fmt.Sprintf(" <i>(%s)</i>", *item.Quantity)
			}

			message += fmt.Sprintf("%d. %s\n", i+1, itemText)
		}
	}

	// Create action buttons
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("➕ Add Item", fmt.Sprintf("list_additem_%s", listID.String())),
		},
	}

	// Add item management buttons if there are items
	if len(items) > 0 {
		itemButtons := []tgbotapi.InlineKeyboardButton{}
		for i, item := range items {
			if i >= 16 { // Limit to 8 items to avoid button overflow
				break
			}

			// Use parsed name for button if available, otherwise use original name
			itemName := item.Name
			if item.ParsedName != nil && *item.ParsedName != "" {
				itemName = *item.ParsedName
			}

			// Truncate long names for button display
			if len(itemName) > 20 {
				itemName = itemName[:17] + "..."
			}

			status := "☐"
			if item.IsCompleted {
				status = "✅"
			}

			buttonText := fmt.Sprintf("%s %s", status, itemName)

			// Use compact format: "li_t_shortListID_shortItemID" (about 20 chars)
			shortListID := listID.String()[:8]  // First 8 chars of UUID
			shortItemID := item.ID.String()[:8] // First 8 chars of UUID
			compactData := fmt.Sprintf("li_t_%s_%s", shortListID, shortItemID)

			itemButtons = append(itemButtons, tgbotapi.NewInlineKeyboardButtonData(
				buttonText,
				compactData,
			))

			// Create rows of 4 buttons each
			if len(itemButtons) == 4 {
				buttons = append(buttons, itemButtons)
				itemButtons = []tgbotapi.InlineKeyboardButton{}
			}
		}

		// Add remaining buttons
		if len(itemButtons) > 0 {
			buttons = append(buttons, itemButtons)
		}
	}

	// Add navigation buttons
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", fmt.Sprintf("list_view_%s", listID.String())),
		tgbotapi.NewInlineKeyboardButtonData("📝 All Lists", "show_all_lists"),
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

	if _, err := s.bot.Send(editMsg); err != nil {
		s.logger.Error("Failed to edit list view message", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to update list view.")
		return
	}

	s.answerCallback(callback.ID, "")
}

// handleAddItemCallback initiates the add item workflow
func (s *BotService) handleAddItemCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, user *users.User) {
	// Get the shopping list for display
	list, err := s.shoppingService.GetShoppingListByID(ctx, listID)
	if err != nil || list == nil {
		s.logger.Error("Failed to get shopping list", "error", err, "list_id", listID)
		s.answerCallback(callback.ID, "❌ List not found.")
		return
	}

	s.answerCallback(callback.ID, "Ready to add item!")

	// Render template for add item prompt
	data := struct {
		ListName string
	}{
		ListName: list.Name,
	}

	message, err := s.templateManager.RenderTemplate("add_item_prompt", user.Locale, data)
	if err != nil {
		s.logger.Error("Failed to render add item prompt template", "error", err)
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
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", fmt.Sprintf("list_view_%s", listID.String())),
		},
	}
	cancelKeyboard := tgbotapi.NewInlineKeyboardMarkup(cancelButton...)
	editMsg.ReplyMarkup = &cancelKeyboard

	if _, err := s.bot.Send(editMsg); err != nil {
		s.logger.Error("Failed to edit message for add item", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to prepare add item workflow.")
		return
	}

	// Store state that user is adding item to this list
	s.storeUserState(user.TelegramID, "adding_item_to_list", listID.String())
}

// handleToggleItemCallback toggles an item's completion status
func (s *BotService) handleToggleItemCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, itemIDStr string, user *users.User) {
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		s.logger.Error("Failed to parse item ID", "error", err, "item_id", itemIDStr)
		s.answerCallback(callback.ID, "❌ Invalid item ID.")
		return
	}

	// Get the item to check current status
	items, err := s.shoppingService.GetListItems(ctx, listID)
	if err != nil {
		s.logger.Error("Failed to get list items", "error", err)
		s.answerCallback(callback.ID, "❌ Failed to load items.")
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
		s.answerCallback(callback.ID, "❌ Item not found.")
		return
	}

	// Toggle the item status
	if targetItem.IsCompleted {
		err = s.shoppingService.UncompleteItem(ctx, itemID)
		if err != nil {
			s.logger.Error("Failed to uncomplete item", "error", err)
			s.answerCallback(callback.ID, "❌ Failed to uncomplete item.")
			return
		}
		s.answerCallback(callback.ID, "Item unmarked ☐")

		// Notify family members about item being reopened
		quantityStr := ""
		if targetItem.Quantity != nil {
			quantityStr = *targetItem.Quantity
		}
		go s.notifyFamilyMembersItemEdit(ctx, listID, user, "item_uncompleted_notification", targetItem.Name, quantityStr)
	} else {
		err = s.shoppingService.CompleteItem(ctx, itemID, user.ID)
		if err != nil {
			s.logger.Error("Failed to complete item", "error", err)
			s.answerCallback(callback.ID, "❌ Failed to complete item.")
			return
		}
		s.answerCallback(callback.ID, "Item completed ✅")

		// Notify family members about item completion
		quantityStr := ""
		if targetItem.Quantity != nil {
			quantityStr = *targetItem.Quantity
		}
		go s.notifyFamilyMembersItemEdit(ctx, listID, user, "item_completed_notification", targetItem.Name, quantityStr)
	}

	// Refresh the list view
	s.handleViewListCallback(ctx, callback, listID, user)
}

// handleDeleteItemCallback handles item deletion (would need confirmation)
func (s *BotService) handleDeleteItemCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, listID uuid.UUID, itemIDStr string, user *users.User) {
	// For now, just show a message - we could implement confirmation dialog
	s.answerCallback(callback.ID, "🗑️ Item deletion coming soon!")
}

// handleAddItemInput processes user input when adding an item to a shopping list
func (s *BotService) handleAddItemInput(ctx context.Context, message *tgbotapi.Message, user *users.User, listIDStr string) {
	itemText := strings.TrimSpace(message.Text)

	if itemText == "" {
		s.sendMessage(message.Chat.ID, "❌ Please provide an item name.")
		return
	}

	// Parse list ID
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		s.logger.Error("Failed to parse list ID from state", "error", err, "list_id", listIDStr)
		s.sendMessage(message.Chat.ID, "❌ Something went wrong. Please try again.")
		s.clearUserState(user.TelegramID, "adding_item_to_list")
		return
	}

	// Check if user can access this list
	canAccess, err := s.shoppingService.CanUserAccessList(ctx, listID, user.ID)
	if err != nil {
		s.logger.Error("Failed to check list access", "error", err)
		s.sendMessage(message.Chat.ID, "❌ Failed to verify list access.")
		s.clearUserState(user.TelegramID, "adding_item_to_list")
		return
	}

	if !canAccess {
		s.sendMessage(message.Chat.ID, "❌ You don't have access to this list.")
		s.clearUserState(user.TelegramID, "adding_item_to_list")
		return
	}

	// Use AI to intelligently parse and separate multiple items
	addedItems, failedItems, err := s.shoppingService.AddItemsToListWithAI(ctx, listID, itemText, user.Locale, user.ID)
	if err != nil {
		s.logger.Error("Failed to add items to list with AI", "error", err)
		s.sendMessage(message.Chat.ID, "❌ Failed to add items. Please try again.")
		return
	}

	// Notify family members about each new item
	for _, item := range addedItems {
		quantityStr := ""
		if item.Quantity != nil {
			quantityStr = *item.Quantity
		}
		go s.notifyFamilyMembersItemEdit(ctx, listID, user, "item_added_notification", item.Name, quantityStr)
	}

	// Clear user state
	s.clearUserState(user.TelegramID, "adding_item_to_list")

	// Prepare success message based on results
	var successText string
	if len(addedItems) > 0 {
		if len(addedItems) == 1 {
			successText = fmt.Sprintf("✅ <b>Item Added!</b>\n\n➕ <b>%s</b>", addedItems[0].Name)
			if addedItems[0].Quantity != nil && *addedItems[0].Quantity != "" {
				successText += fmt.Sprintf(" <i>(%s)</i>", *addedItems[0].Quantity)
			}
		} else {
			successText = fmt.Sprintf("✅ <b>%d Items Added!</b>\n\n", len(addedItems))
			for _, item := range addedItems {
				successText += fmt.Sprintf("➕ <b>%s</b>", item.Name)
				if item.Quantity != nil && *item.Quantity != "" {
					successText += fmt.Sprintf(" <i>(%s)</i>", *item.Quantity)
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

	if len(addedItems) == 0 {
		s.sendMessage(message.Chat.ID, successText)
		return
	}

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

	msg := tgbotapi.NewMessage(message.Chat.ID, successText)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Failed to send item added message", "error", err)
		// Fallback to simple message
		s.sendMessage(message.Chat.ID, successText)
	}
}

// containsDigit checks if a string contains any digit
func containsDigit(s string) bool {
	for _, char := range s {
		if char >= '0' && char <= '9' {
			return true
		}
	}
	return false
}

// handleCreateCallback handles create callbacks like create_new_list
func (s *BotService) handleCreateCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) < 2 {
		s.answerCallback(callback.ID, "❌ Invalid create action.")
		return
	}

	// Reconstruct the full action (handle underscores)
	action := strings.Join(parts[1:], "_")

	switch action {
	case "new_list":
		// Get user from callback
		user, err := s.usersService.GetUserByTelegramID(ctx, callback.From.ID)
		if err != nil {
			s.logger.Error("Failed to get user", "error", err, "telegram_id", callback.From.ID)
			s.answerCallback(callback.ID, "❌ User not found.")
			return
		}

		s.answerCallback(callback.ID, "Starting list creation...")
		s.handleCreateListCommand(ctx, callback.Message.Chat.ID, user)
	default:
		s.answerCallback(callback.ID, "❌ Unknown create action.")
	}
}
