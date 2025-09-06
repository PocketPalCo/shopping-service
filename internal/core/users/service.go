package users

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("users-service")

type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	TelegramID   int64      `json:"telegram_id" db:"telegram_id"`
	Username     *string    `json:"username" db:"username"`
	FirstName    string     `json:"first_name" db:"first_name"`
	LastName     *string    `json:"last_name" db:"last_name"`
	IsAuthorized bool       `json:"is_authorized" db:"is_authorized"`
	AuthorizedBy *uuid.UUID `json:"authorized_by" db:"authorized_by"`
	AuthorizedAt *time.Time `json:"authorized_at" db:"authorized_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	Locale       string     `json:"locale" db:"locale"`
}

type AuthorizationRequest struct {
	TelegramID   int64
	Username     *string
	FirstName    string
	LastName     *string
	LanguageCode *string
}

type Service struct {
	db     *pgxpool.Pool
	admins map[int64]bool
}

func NewService(db *pgxpool.Pool, adminIDs []int64) *Service {
	admins := make(map[int64]bool)
	for _, id := range adminIDs {
		admins[id] = true
	}

	return &Service{
		db:     db,
		admins: admins,
	}
}

func (s *Service) IsAdmin(telegramID int64) bool {
	return s.admins[telegramID]
}

func (s *Service) GetUserByTelegramID(ctx context.Context, telegramID int64) (*User, error) {
	ctx, span := tracer.Start(ctx, "users.GetUserByTelegramID")
	defer span.End()

	query := `
		SELECT id, telegram_id, username, first_name, last_name, is_authorized, authorized_by, authorized_at, created_at, updated_at, locale
		FROM users 
		WHERE telegram_id = $1
	`

	var user User
	err := s.db.QueryRow(ctx, query, telegramID).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsAuthorized,
		&user.AuthorizedBy,
		&user.AuthorizedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Locale,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get user by telegram_id %d: %w", telegramID, err)
	}

	return &user, nil
}

func (s *Service) CreateUser(ctx context.Context, req AuthorizationRequest) (*User, error) {
	ctx, span := tracer.Start(ctx, "users.CreateUser")
	defer span.End()

	locale := "en"
	if req.LanguageCode != nil {
		locale = normalizeLocale(*req.LanguageCode)
	}

	query := `
		INSERT INTO users (telegram_id, username, first_name, last_name, is_authorized, locale, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, telegram_id, username, first_name, last_name, is_authorized, authorized_by, authorized_at, created_at, updated_at, locale
	`

	var user User
	err := s.db.QueryRow(ctx, query, req.TelegramID, req.Username, req.FirstName, req.LastName, false, locale).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsAuthorized,
		&user.AuthorizedBy,
		&user.AuthorizedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Locale,
	)

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &user, nil
}

func (s *Service) AuthorizeUser(ctx context.Context, telegramID int64) error {
	ctx, span := tracer.Start(ctx, "users.AuthorizeUser")
	defer span.End()

	query := `
		UPDATE users 
		SET is_authorized = true, authorized_at = NOW(), updated_at = NOW()
		WHERE telegram_id = $1
	`

	result, err := s.db.Exec(ctx, query, telegramID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to authorize user %d: %w", telegramID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user with telegram_id %d not found", telegramID)
	}

	return nil
}

func (s *Service) RevokeUser(ctx context.Context, telegramID int64) error {
	ctx, span := tracer.Start(ctx, "users.RevokeUser")
	defer span.End()

	query := `
		UPDATE users 
		SET is_authorized = false, authorized_at = NULL, updated_at = NOW()
		WHERE telegram_id = $1
	`

	result, err := s.db.Exec(ctx, query, telegramID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to revoke user %d: %w", telegramID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user with telegram_id %d not found", telegramID)
	}

	return nil
}

func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	ctx, span := tracer.Start(ctx, "users.GetUserByID")
	defer span.End()

	query := `
		SELECT id, telegram_id, username, first_name, last_name, is_authorized, authorized_by, authorized_at, created_at, updated_at, locale
		FROM users 
		WHERE id = $1
	`

	var user User
	err := s.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsAuthorized,
		&user.AuthorizedBy,
		&user.AuthorizedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Locale,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get user by ID %s: %w", userID.String(), err)
	}

	return &user, nil
}

func (s *Service) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	ctx, span := tracer.Start(ctx, "users.GetUserByUsername")
	defer span.End()

	query := `
		SELECT id, telegram_id, username, first_name, last_name, is_authorized, authorized_by, authorized_at, created_at, updated_at, locale
		FROM users 
		WHERE username = $1
	`

	var user User
	err := s.db.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsAuthorized,
		&user.AuthorizedBy,
		&user.AuthorizedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Locale,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get user by username %s: %w", username, err)
	}

	return &user, nil
}

func (s *Service) GetAllUsers(ctx context.Context) ([]*User, error) {
	ctx, span := tracer.Start(ctx, "users.GetAllUsers")
	defer span.End()

	query := `
		SELECT id, telegram_id, username, first_name, last_name, is_authorized, authorized_by, authorized_at, created_at, updated_at, locale
		FROM users 
		WHERE is_authorized = true
		ORDER BY first_name, last_name
	`

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get all users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		err := rows.Scan(
			&user.ID,
			&user.TelegramID,
			&user.Username,
			&user.FirstName,
			&user.LastName,
			&user.IsAuthorized,
			&user.AuthorizedBy,
			&user.AuthorizedAt,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.Locale,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over users: %w", err)
	}

	return users, nil
}

// normalizeLocale converts language codes to supported locales
func normalizeLocale(languageCode string) string {
	switch languageCode {
	case "en", "en-US", "en-GB":
		return "en"
	case "uk", "uk-UA":
		return "uk"
	case "ru", "ru-RU":
		return "ru"
	default:
		return "en" // Default to English
	}
}
