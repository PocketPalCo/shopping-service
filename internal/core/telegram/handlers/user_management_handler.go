package handlers

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserManagementHandler handles user registration, authorization, and management
type UserManagementHandler struct {
	BaseHandler
}

// NewUserManagementHandler creates a new user management handler
func NewUserManagementHandler(base BaseHandler) *UserManagementHandler {
	return &UserManagementHandler{
		BaseHandler: base,
	}
}

// InternalUser represents the bot's internal user structure
type BotInternalUser struct {
	User         *users.User
	TelegramID   int64
	IsAdmin      bool
	IsAuthorized bool
	IsNewUser    bool // Indicates if this user was just created
}

// GetOrCreateInternalUser gets or creates an internal user from Telegram user info
func (h *UserManagementHandler) GetOrCreateInternalUser(ctx context.Context, telegramUser *tgbotapi.User) (*BotInternalUser, error) {
	// Try to get existing user
	existingUser, err := h.usersService.GetUserByTelegramID(ctx, telegramUser.ID)
	if err != nil {
		h.logger.Error("Failed to get user by telegram ID", "error", err, "telegram_id", telegramUser.ID)
		return nil, err
	}

	if existingUser != nil {
		// User exists, return internal user
		return &BotInternalUser{
			User:         existingUser,
			TelegramID:   existingUser.TelegramID,
			IsAdmin:      h.usersService.IsAdmin(existingUser.TelegramID),
			IsAuthorized: existingUser.IsAuthorized,
			IsNewUser:    false, // Existing user
		}, nil
	}

	// User doesn't exist, create new user
	authRequest := users.AuthorizationRequest{
		TelegramID:   telegramUser.ID,
		Username:     &telegramUser.UserName,
		FirstName:    telegramUser.FirstName,
		LastName:     &telegramUser.LastName,
		LanguageCode: &telegramUser.LanguageCode,
	}

	// Handle empty username
	if telegramUser.UserName == "" {
		authRequest.Username = nil
	}

	// Handle empty last name
	if telegramUser.LastName == "" {
		authRequest.LastName = nil
	}

	// Handle empty language code
	if telegramUser.LanguageCode == "" {
		authRequest.LanguageCode = nil
	}

	newUser, err := h.usersService.CreateUser(ctx, authRequest)
	if err != nil {
		h.logger.Error("Failed to create new user", "error", err, "telegram_id", telegramUser.ID)
		return nil, err
	}

	h.logger.Info("Created new user",
		"user_id", newUser.ID,
		"telegram_id", newUser.TelegramID,
		"username", newUser.Username,
		"first_name", newUser.FirstName,
		"locale", newUser.Locale)

	return &BotInternalUser{
		User:         newUser,
		TelegramID:   newUser.TelegramID,
		IsAdmin:      h.usersService.IsAdmin(newUser.TelegramID),
		IsAuthorized: newUser.IsAuthorized,
		IsNewUser:    true, // Newly created user
	}, nil
}

// NotifyAdminsOfNewUser notifies all admins when a new user is created
func (h *UserManagementHandler) NotifyAdminsOfNewUser(ctx context.Context, newUser *users.User) {
	if !h.usersService.IsAdmin(newUser.TelegramID) {
		// Get all admin users to notify them
		adminUsers, err := h.usersService.GetAllUsers(ctx) // This returns authorized users, but we need to filter admins
		if err != nil {
			h.logger.Error("Failed to get admin users for notification", "error", err)
			return
		}

		// Filter for admin users and notify them
		for _, adminUser := range adminUsers {
			if h.usersService.IsAdmin(adminUser.TelegramID) {
				h.notifyAdminOfNewUser(adminUser, newUser)
			}
		}
	}
}

// notifyAdminOfNewUser sends a notification to a specific admin about a new user
func (h *UserManagementHandler) notifyAdminOfNewUser(admin *users.User, newUser *users.User) {
	var lastName, username string
	if newUser.LastName != nil {
		lastName = *newUser.LastName
	}
	if newUser.Username != nil {
		username = *newUser.Username
	}

	data := struct {
		FirstName  string
		LastName   string
		Username   string
		TelegramID int64
		CreatedAt  string
	}{
		FirstName:  newUser.FirstName,
		LastName:   lastName,
		Username:   username,
		TelegramID: newUser.TelegramID,
		CreatedAt:  newUser.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	message, err := h.templateManager.RenderTemplate("admin_new_user", admin.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render admin new user template", "error", err)
		message = "👤 New user registered: " + newUser.FirstName
	}

	msg := tgbotapi.NewMessage(admin.TelegramID, message)
	msg.ParseMode = tgbotapi.ModeHTML

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("Failed to send new user notification to admin",
			"error", err,
			"admin_id", admin.TelegramID,
			"new_user_id", newUser.TelegramID)
	}
}

// SendAuthorizationSuccess sends a success message when a user is authorized
func (h *UserManagementHandler) SendAuthorizationSuccess(user *users.User) {
	displayName := user.FirstName
	if user.LastName != nil && *user.LastName != "" {
		displayName = user.FirstName + " " + *user.LastName
	}

	data := struct {
		FirstName string
	}{
		FirstName: displayName,
	}

	message, err := h.templateManager.RenderTemplate("authorization_success", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render authorization success template", "error", err)
		message = "✅ You have been successfully authorized!"
	}

	msg := tgbotapi.NewMessage(user.TelegramID, message)
	msg.ParseMode = tgbotapi.ModeHTML

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("Failed to send authorization success message",
			"error", err,
			"user_id", user.TelegramID)
	}
}
