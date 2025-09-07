package telegram

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// TelegramUser represents a user from Telegram API
type TelegramUser struct {
	ID           int64   `json:"id"`
	Username     *string `json:"username,omitempty"`
	FirstName    string  `json:"first_name"`
	LastName     *string `json:"last_name,omitempty"`
	LanguageCode *string `json:"language_code,omitempty"`
}

// NewTelegramUserFromAPI creates a TelegramUser from Telegram Bot API User
func NewTelegramUserFromAPI(tgUser *tgbotapi.User) *TelegramUser {
	var username *string
	if tgUser.UserName != "" {
		username = &tgUser.UserName
	}

	var lastName *string
	if tgUser.LastName != "" {
		lastName = &tgUser.LastName
	}

	var languageCode *string
	if tgUser.LanguageCode != "" {
		languageCode = &tgUser.LanguageCode
	}

	return &TelegramUser{
		ID:           tgUser.ID,
		Username:     username,
		FirstName:    tgUser.FirstName,
		LastName:     lastName,
		LanguageCode: languageCode,
	}
}