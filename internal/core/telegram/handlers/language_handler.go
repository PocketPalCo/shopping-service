package handlers

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// LanguageHandler handles language selection and switching
type LanguageHandler struct {
	BaseHandler
}

// NewLanguageHandler creates a new language handler
func NewLanguageHandler(base BaseHandler) *LanguageHandler {
	return &LanguageHandler{
		BaseHandler: base,
	}
}

// HandleLanguageChange handles language selection callbacks
func (h *LanguageHandler) HandleLanguageChange(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 2 {
		h.AnswerCallback(callback.ID, "❌ Invalid language selection.")
		return
	}

	selectedLocale := parts[1]

	// Validate selected locale
	if selectedLocale != "en" && selectedLocale != "uk" && selectedLocale != "ru" {
		h.AnswerCallback(callback.ID, "❌ Unsupported language.")
		return
	}

	// Update user's locale in database
	err := h.usersService.UpdateUserLocale(ctx, user.TelegramID, selectedLocale)
	if err != nil {
		h.logger.Error("Failed to update user locale", "error", err, "user_id", user.TelegramID, "locale", selectedLocale)
		h.AnswerCallback(callback.ID, "❌ Failed to change language.")
		return
	}

	// Update user object for immediate effect
	user.Locale = selectedLocale

	// Show success message and return to main menu
	var successMessage string
	switch selectedLocale {
	case "en":
		successMessage = "🇺🇸 Language changed to English!"
	case "uk":
		successMessage = "🇺🇦 Мову змінено на українську!"
	case "ru":
		successMessage = "🇷🇺 Язык изменен на русский!"
	}

	h.AnswerCallback(callback.ID, successMessage)

	// Return to main menu with new language - we need to do this through a callback
	// since we need access to the menu handler
	h.logger.Info("Language changed successfully", "user_id", user.TelegramID, "new_locale", selectedLocale)
}
