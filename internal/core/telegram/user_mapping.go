package telegram

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// UserMapper handles mapping between Telegram users and internal users
type UserMapper struct {
	usersService *users.Service
}

// NewUserMapper creates a new user mapper
func NewUserMapper(usersService *users.Service) *UserMapper {
	return &UserMapper{
		usersService: usersService,
	}
}

// InternalUser represents our internal user with business logic properties
type InternalUser struct {
	*users.User
	IsAdmin bool
}

// GetOrCreateInternalUser gets or creates an internal user from Telegram user data
func (um *UserMapper) GetOrCreateInternalUser(ctx context.Context, tgUser *TelegramUser) (*InternalUser, error) {
	// Try to get existing user first
	dbUser, err := um.usersService.GetUserByTelegramID(ctx, tgUser.ID)
	if err != nil {
		return nil, err
	}

	// If user doesn't exist, create them
	if dbUser == nil {
		dbUser, err = um.usersService.CreateUser(ctx, users.AuthorizationRequest{
			TelegramID:   tgUser.ID,
			Username:     tgUser.Username,
			FirstName:    tgUser.FirstName,
			LastName:     tgUser.LastName,
			LanguageCode: tgUser.LanguageCode,
		})
		if err != nil {
			return nil, err
		}
	}

	// Check if user is admin (this is our internal business logic)
	isAdmin := um.usersService.IsAdmin(tgUser.ID)

	return &InternalUser{
		User:    dbUser,
		IsAdmin: isAdmin,
	}, nil
}

// GetInternalUserByTelegramID gets an internal user by Telegram ID
func (um *UserMapper) GetInternalUserByTelegramID(ctx context.Context, telegramID int64) (*InternalUser, error) {
	// Get the database user
	dbUser, err := um.usersService.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	if dbUser == nil {
		return nil, nil
	}

	// Check if user is admin (this is our internal business logic)
	isAdmin := um.usersService.IsAdmin(telegramID)

	return &InternalUser{
		User:    dbUser,
		IsAdmin: isAdmin,
	}, nil
}