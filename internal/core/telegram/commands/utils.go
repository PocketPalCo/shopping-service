package commands

import (
	"strconv"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// getUserDisplayName extracts display name from user (shared utility function)
func getUserDisplayName(user *users.User) string {
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
