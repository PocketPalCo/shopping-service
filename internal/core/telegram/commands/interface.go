package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

// Command represents a bot command handler
type Command interface {
	// GetName returns the command name (without /)
	GetName() string

	// RequiresAuth returns true if the command requires user authorization
	RequiresAuth() bool

	// RequiresAdmin returns true if the command requires admin privileges
	RequiresAdmin() bool

	// Handle executes the command
	Handle(ctx context.Context, chatID int64, user *users.User, args []string) error
}

// CommandRegistry manages bot commands
type CommandRegistry struct {
	commands map[string]Command
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]Command),
	}
}

// Register adds a command to the registry
func (r *CommandRegistry) Register(cmd Command) {
	r.commands[cmd.GetName()] = cmd
}

// Get retrieves a command by name
func (r *CommandRegistry) Get(name string) (Command, bool) {
	cmd, exists := r.commands[name]
	return cmd, exists
}

// List returns all registered commands
func (r *CommandRegistry) List() map[string]Command {
	return r.commands
}

// ExecuteCommand executes a command by name with authorization checks
// Note: This method needs access to usersService to check admin status
func (r *CommandRegistry) ExecuteCommand(ctx context.Context, commandName string, chatID int64, user *users.User, args []string) error {
	command, exists := r.Get(commandName)
	if !exists {
		return nil // Command not found - silently ignore
	}

	// Check authorization requirements
	if command.RequiresAuth() && !user.IsAuthorized {
		return nil // Not authorized - silently ignore
	}

	// Admin check will be handled by individual commands since we don't have usersService here

	// Execute command
	return command.Handle(ctx, chatID, user, args)
}
