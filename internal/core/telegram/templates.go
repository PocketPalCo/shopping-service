package telegram

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"path/filepath"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/users"
)

//go:embed templates/**/*.html
var templateFiles embed.FS

type TemplateManager struct {
	templates map[string]*template.Template
}

func NewTemplateManager() (*TemplateManager, error) {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
	}

	// Load templates for each supported locale
	locales := []string{"en", "uk", "ru"}

	// Define template helper functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}

	for _, locale := range locales {
		pattern := fmt.Sprintf("templates/%s/*.html", locale)
		tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFiles, pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to parse templates for locale %s: %w", locale, err)
		}
		tm.templates[locale] = tmpl
	}

	return tm, nil
}

func (tm *TemplateManager) RenderTemplate(templateName, locale string, data interface{}) (string, error) {
	// Default to English if locale not supported
	if _, exists := tm.templates[locale]; !exists {
		locale = "en"
	}

	tmpl, exists := tm.templates[locale]
	if !exists {
		return "", fmt.Errorf("no templates loaded for locale: %s", locale)
	}

	var buf bytes.Buffer
	templateFile := filepath.Base(templateName) + ".html"
	err := tmpl.ExecuteTemplate(&buf, templateFile, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute template %s for locale %s: %w", templateFile, locale, err)
	}

	return buf.String(), nil
}

// RenderButton renders a localized button text
func (tm *TemplateManager) RenderButton(buttonName, locale string) string {
	// Default to English if locale not supported
	if _, exists := tm.templates[locale]; !exists {
		locale = "en"
	}

	tmpl, exists := tm.templates[locale]
	if !exists {
		return buttonName // Fallback to button name if template not found
	}

	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "button_"+buttonName, nil)
	if err != nil {
		return buttonName // Fallback to button name if execution fails
	}

	return buf.String()
}

// RenderMessage renders a localized error or success message
func (tm *TemplateManager) RenderMessage(messageName, locale string) string {
	// Default to English if locale not supported
	if _, exists := tm.templates[locale]; !exists {
		locale = "en"
	}

	tmpl, exists := tm.templates[locale]
	if !exists {
		return messageName // Fallback to message name if template not found
	}

	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, messageName, nil)
	if err != nil {
		return messageName // Fallback to message name if execution fails
	}

	return buf.String()
}

// GetSupportedLocales returns list of supported locales
func (tm *TemplateManager) GetSupportedLocales() []string {
	return []string{"en", "uk", "ru"}
}

// IsLocaleSupported checks if a locale is supported
func (tm *TemplateManager) IsLocaleSupported(locale string) bool {
	_, exists := tm.templates[locale]
	return exists
}

// NormalizeLocale converts language codes to supported locales
func NormalizeLocale(languageCode string) string {
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

type StartTemplateData struct {
	FirstName    string
	IsAuthorized bool
}

type HelpTemplateData struct {
	IsAuthorized bool
	IsAdmin      bool
}

type StatusTemplateData struct {
	FirstName    string
	LastName     string
	Username     string
	TelegramID   int64
	IsAuthorized bool
	AuthorizedAt *time.Time
}

type UsersListTemplateData struct {
	Users []*users.User
}

type MyIDTemplateData struct {
	TelegramID int64
}

type AuthorizationSuccessTemplateData struct {
	FirstName string
}

type AdminNewUserTemplateData struct {
	FirstName  string
	LastName   string
	Username   string
	TelegramID int64
	CreatedAt  string
}

type AdminAuthorizationTemplateData struct {
	FirstName string
	LastName  string
	Username  string
}
