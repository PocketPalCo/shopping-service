package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// StatsCommand handles the /stats command (admin only)
type StatsCommand struct {
	BaseCommand
}

// NewStatsCommand creates a new stats command
func NewStatsCommand(base BaseCommand) *StatsCommand {
	return &StatsCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *StatsCommand) GetName() string {
	return "stats"
}

// RequiresAuth returns true as stats command requires authorization
func (c *StatsCommand) RequiresAuth() bool {
	return true
}

// RequiresAdmin returns true as stats command requires admin privileges
func (c *StatsCommand) RequiresAdmin() bool {
	return true
}

// Handle executes the stats command
func (c *StatsCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	// Build basic stats message - count methods not implemented yet
	statsMessage := "📊 <b>Bot Statistics</b>\n\n"
	statsMessage += "👥 <b>Total Users:</b> Available via database queries\n"
	statsMessage += "✅ <b>Authorized Users:</b> Available via database queries\n"
	statsMessage += "🏠 <b>Total Families:</b> Available via database queries\n"
	statsMessage += "📝 <b>Shopping Lists:</b> Available via database queries\n"
	statsMessage += "\n🤖 <b>System:</b> Running\n"
	statsMessage += "📡 <b>API:</b> Telegram Bot API\n"
	statsMessage += "\n<i>Note: Detailed counts will be implemented in a future update</i>"

	c.SendMessage(chatID, statsMessage)
	return nil
}
