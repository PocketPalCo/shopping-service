package commands

import (
	"strconv"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// GetUserDisplayName extracts display name from user (shared utility function)
func GetUserDisplayName(user *users.User) string {
	if user.FirstName != "" {
		if user.LastName != nil && *user.LastName != "" {
			return user.FirstName + " " + *user.LastName
		}
		return user.FirstName
	}
	if user.Username != nil && *user.Username != "" {
		return "@" + *user.Username
	}
	return "User_" + strconv.FormatInt(user.TelegramID, 10)
}
