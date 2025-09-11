package language

import "strings"

// NormalizeLanguageCode normalizes language codes to standard format
func NormalizeLanguageCode(languageCode string) string {
	normalized := strings.ToLower(strings.TrimSpace(languageCode))
	
	switch normalized {
	case "ru", "rus", "russian":
		return "ru"
	case "uk", "ukr", "ukrainian":
		return "uk"
	case "en", "eng", "english":
		return "en"
	default:
		return "en" // Default to English if unknown
	}
}