package commands

import (
	"context"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StartCommand handles the /start command
type StartCommand struct {
	BaseCommand
}

// StartTemplateData holds data for the start template
type StartTemplateData struct {
	FirstName    string
	IsAuthorized bool
}

// NewStartCommand creates a new start command
func NewStartCommand(base BaseCommand) *StartCommand {
	return &StartCommand{
		BaseCommand: base,
	}
}

// GetName returns the command name
func (c *StartCommand) GetName() string {
	return "start"
}

// RequiresAuth returns false as start command doesn't require authorization
func (c *StartCommand) RequiresAuth() bool {
	return false
}

// RequiresAdmin returns false as start command doesn't require admin privileges
func (c *StartCommand) RequiresAdmin() bool {
	return false
}

// Handle executes the start command
func (c *StartCommand) Handle(ctx context.Context, chatID int64, user *users.User, args []string) error {
	data := StartTemplateData{
		FirstName:    GetUserDisplayName(user),
		IsAuthorized: user.IsAuthorized,
	}

	message, err := c.templateManager.RenderTemplate("start", user.Locale, data)
	if err != nil {
		c.logger.Error("Failed to render start template", "error", err)
		c.SendMessage(chatID, "❌ Internal error occurred. Please try again later.")
		return err
	}

	// Create main menu with buttons instead of requiring commands
	keyboard := c.createMainMenu(user)
	c.SendMessageWithKeyboard(chatID, message, keyboard)
	return nil
}

// createMainMenu creates the main menu inline keyboard based on user authorization
func (c *StartCommand) createMainMenu(user *users.User) tgbotapi.InlineKeyboardMarkup {
	var buttons [][]tgbotapi.InlineKeyboardButton

	if user.IsAuthorized {
		// Authorized user menu - using localized button templates
		buttons = [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_lists", user.Locale), "menu_lists"),
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_families", user.Locale), "menu_families"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_createlist", user.Locale), "menu_createlist"),
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_createfamily", user.Locale), "menu_createfamily"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_help", user.Locale), "menu_help"),
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_status", user.Locale), "menu_status"),
			},
		}

		// Add admin menu if user is admin
		if c.usersService.IsAdmin(user.TelegramID) {
			adminButtons := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_users", user.Locale), "menu_users"),
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_stats", user.Locale), "menu_stats"),
			}
			buttons = append(buttons, adminButtons)
		}
	} else {
		// Non-authorized user menu (limited options) - using localized button templates
		buttons = [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_help", user.Locale), "menu_help"),
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_status", user.Locale), "menu_status"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData(c.templateManager.RenderButton("menu_myid", user.Locale), "menu_myid"),
			},
		}
	}

	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

// CreateMainMenuButton creates a localized "Main Menu" button that can be added to other command responses
func CreateMainMenuButton(templateManager TemplateRenderer, locale string) []tgbotapi.InlineKeyboardButton {
	return []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(templateManager.RenderButton("button_main_menu", locale), "menu_start"),
	}
}
