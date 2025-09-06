package telegram

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	TelegramID   int64      `json:"telegram_id" db:"telegram_id"`
	Username     *string    `json:"username" db:"username"`
	FirstName    *string    `json:"first_name" db:"first_name"`
	LastName     *string    `json:"last_name" db:"last_name"`
	Locale       string     `json:"locale" db:"locale"`
	IsAuthorized bool       `json:"is_authorized" db:"is_authorized"`
	AuthorizedBy *uuid.UUID `json:"authorized_by" db:"authorized_by"`
	AuthorizedAt *time.Time `json:"authorized_at" db:"authorized_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type UserSession struct {
	ID           uuid.UUID `json:"id" db:"id"`
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	SessionToken string    `json:"session_token" db:"session_token"`
	ChatID       int64     `json:"chat_id" db:"chat_id"`
	LastActivity time.Time `json:"last_activity" db:"last_activity"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type AuthorizationRequest struct {
	TelegramID   int64   `json:"telegram_id"`
	Username     *string `json:"username"`
	FirstName    *string `json:"first_name"`
	LastName     *string `json:"last_name"`
	LanguageCode *string `json:"language_code"`
	ChatID       int64   `json:"chat_id"`
}
