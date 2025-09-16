package translations

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TranslationAIResult represents the result of an AI translation
type TranslationAIResult struct {
	TranslatedText string
	Confidence     float64
}

// AIService interface for translation functionality
type AIService interface {
	Translate(ctx context.Context, originalText, originalLanguage, targetLanguage string) (*TranslationAIResult, error)
}

type ItemTranslation struct {
	ID                uuid.UUID `json:"id" db:"id"`
	OriginalText      string    `json:"original_text" db:"original_text"`
	OriginalLanguage  string    `json:"original_language" db:"original_language"`
	TranslatedText    string    `json:"translated_text" db:"translated_text"`
	TargetLanguage    string    `json:"target_language" db:"target_language"`
	Category          *string   `json:"category" db:"category"`
	Confidence        *float64  `json:"confidence" db:"confidence"`
	TranslationSource string    `json:"translation_source" db:"translation_source"`
	UsageCount        int       `json:"usage_count" db:"usage_count"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type ReceiptItem struct {
	ID                   uuid.UUID        `json:"id" db:"id"`
	ReceiptID            uuid.UUID        `json:"receipt_id" db:"receipt_id"`
	ItemOrder            int              `json:"item_order" db:"item_order"`
	OriginalDescription  string           `json:"original_description" db:"original_description"`
	OriginalLanguage     *string          `json:"original_language" db:"original_language"`
	LocalizedDescription *string          `json:"localized_description" db:"localized_description"`
	UserLocale           *string          `json:"user_locale" db:"user_locale"`
	Quantity             *float64         `json:"quantity" db:"quantity"`
	UnitPrice            *float64         `json:"unit_price" db:"unit_price"`
	TotalPrice           float64          `json:"total_price" db:"total_price"`
	CurrencyCode         *string          `json:"currency_code" db:"currency_code"`
	UserCategory         *string          `json:"user_category" db:"user_category"`
	UserNotes            *string          `json:"user_notes" db:"user_notes"`
	IsUserModified       bool             `json:"is_user_modified" db:"is_user_modified"`
	Confidence           *float64         `json:"confidence" db:"confidence"`
	BoundingRegions      *json.RawMessage `json:"bounding_regions" db:"bounding_regions"`
	CreatedAt            time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at" db:"updated_at"`
}

// TranslationService handles automatic translation of receipt items to user locales
type TranslationService struct {
	db                   *pgxpool.Pool
	logger               *slog.Logger
	aiService            AIService
	aiTranslationEnabled bool
}

type TranslationRequest struct {
	OriginalText     string
	OriginalLanguage string
	TargetLanguage   string
	Category         *string
	UserID           uuid.UUID
}

type TranslationResult struct {
	TranslatedText string
	Confidence     *float64
	Source         string // 'cache', 'ai', 'fallback'
	Translation    *ItemTranslation
}

func NewTranslationService(db *pgxpool.Pool, logger *slog.Logger, aiService AIService) *TranslationService {
	return &TranslationService{
		db:                   db,
		logger:               logger.With("service", "translations"),
		aiService:            aiService,
		aiTranslationEnabled: true,
	}
}

func (s *TranslationService) TranslateItem(ctx context.Context, req *TranslationRequest) (*TranslationResult, error) {
	req.OriginalLanguage = s.NormalizeLanguage(req.OriginalLanguage)
	req.TargetLanguage = s.NormalizeLanguage(req.TargetLanguage)

	if req.OriginalLanguage == req.TargetLanguage {
		return &TranslationResult{
			TranslatedText: req.OriginalText,
			Confidence:     floatPtr(1.0),
			Source:         "same_language",
		}, nil
	}

	if cachedTranslation, err := s.findCachedTranslation(ctx, req); err == nil && cachedTranslation != nil {
		s.logger.Debug("Using cached translation",
			"original", req.OriginalText,
			"target_lang", req.TargetLanguage,
			"cached_result", cachedTranslation.TranslatedText)

		return &TranslationResult{
			TranslatedText: cachedTranslation.TranslatedText,
			Confidence:     cachedTranslation.Confidence,
			Source:         "cache",
			Translation:    cachedTranslation,
		}, nil
	}

	if aiResult, err := s.translateWithAI(ctx, req); err == nil {
		s.logger.Info("AI translation successful",
			"original", req.OriginalText,
			"original_lang", req.OriginalLanguage,
			"target_lang", req.TargetLanguage,
			"result", aiResult.TranslatedText)

		if translation, err := s.storeTranslation(ctx, req, aiResult.TranslatedText, aiResult.Confidence, "ai"); err == nil {
			aiResult.Translation = translation
		}

		return aiResult, nil
	}

	return nil, fmt.Errorf("failed to translate item")
}

// TranslateReceiptItems translates all items in a receipt to the user's locale
func (s *TranslationService) TranslateReceiptItems(ctx context.Context, receiptID uuid.UUID, userLocale string, items []*ReceiptItem) ([]*ReceiptItem, error) {
	userLocale = s.NormalizeLanguage(userLocale)
	translatedItems := make([]*ReceiptItem, 0, len(items))

	for _, item := range items {
		translatedItem := *item // Copy the item

		// Determine original language - use detected language or default
		originalLang := "unknown"
		if item.OriginalLanguage != nil {
			originalLang = *item.OriginalLanguage
		}

		// Skip if already in target language
		if originalLang == userLocale {
			if translatedItem.LocalizedDescription == nil {
				translatedItem.LocalizedDescription = &item.OriginalDescription
			}
			if translatedItem.UserLocale == nil {
				translatedItem.UserLocale = &userLocale
			}
			translatedItems = append(translatedItems, &translatedItem)
			continue
		}

		// Translate the item
		req := &TranslationRequest{
			OriginalText:     item.OriginalDescription,
			OriginalLanguage: originalLang,
			TargetLanguage:   userLocale,
			Category:         item.UserCategory,
		}

		result, err := s.TranslateItem(ctx, req)
		if err != nil {
			s.logger.Error("Failed to translate item",
				"error", err,
				"receipt_id", receiptID,
				"item_description", item.OriginalDescription,
				"target_locale", userLocale)

			// Use original description as fallback
			translatedItem.LocalizedDescription = &item.OriginalDescription
			translatedItem.UserLocale = &userLocale
		} else {
			translatedItem.LocalizedDescription = &result.TranslatedText
			translatedItem.UserLocale = &userLocale
		}

		translatedItems = append(translatedItems, &translatedItem)
	}

	return translatedItems, nil
}

// findCachedTranslation looks for existing translation in database
func (s *TranslationService) findCachedTranslation(ctx context.Context, req *TranslationRequest) (*ItemTranslation, error) {
	query := `
		SELECT id, original_text, original_language, translated_text, target_language,
		       category, confidence, translation_source, usage_count, created_at, updated_at
		FROM item_translations
		WHERE LOWER(original_text) = LOWER($1) AND original_language = $2 AND target_language = $3
		ORDER BY usage_count DESC, confidence DESC NULLS LAST
		LIMIT 1`

	var translation ItemTranslation
	err := s.db.QueryRow(ctx, query, req.OriginalText, req.OriginalLanguage, req.TargetLanguage).Scan(
		&translation.ID, &translation.OriginalText, &translation.OriginalLanguage,
		&translation.TranslatedText, &translation.TargetLanguage, &translation.Category,
		&translation.Confidence, &translation.TranslationSource, &translation.UsageCount,
		&translation.CreatedAt, &translation.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// Increment usage count
	_, err = s.db.Exec(ctx, "UPDATE item_translations SET usage_count = usage_count + 1, updated_at = NOW() WHERE id = $1", translation.ID)
	if err != nil {
		s.logger.Warn("Failed to update translation usage count", "error", err, "id", translation.ID)
	}

	return &translation, nil
}

func (s *TranslationService) translateWithAI(ctx context.Context, req *TranslationRequest) (*TranslationResult, error) {
	if !s.aiTranslationEnabled || s.aiService == nil {
		return nil, fmt.Errorf("AI translation is disabled or AI service is not available")
	}

	s.logger.Debug("Starting AI translation",
		"original_text", req.OriginalText,
		"original_language", req.OriginalLanguage,
		"target_language", req.TargetLanguage)

	translationResult, err := s.aiService.Translate(ctx, req.OriginalText, req.OriginalLanguage, req.TargetLanguage)
	if err != nil {
		s.logger.Error("AI translation failed",
			"error", err,
			"original_text", req.OriginalText,
			"original_language", req.OriginalLanguage,
			"target_language", req.TargetLanguage)
		return nil, fmt.Errorf("AI translation failed: %w", err)
	}

	// Validate translation result
	if strings.TrimSpace(translationResult.TranslatedText) == "" {
		return nil, fmt.Errorf("AI returned empty translation")
	}

	s.logger.Debug("AI translation successful",
		"original_text", req.OriginalText,
		"translated_text", translationResult.TranslatedText,
		"confidence", translationResult.Confidence,
		"original_language", req.OriginalLanguage,
		"target_language", req.TargetLanguage)

	// Use the actual confidence from AI response
	return &TranslationResult{
		TranslatedText: translationResult.TranslatedText,
		Confidence:     &translationResult.Confidence,
		Source:         "ai",
	}, nil
}

func (s *TranslationService) storeTranslation(ctx context.Context, req *TranslationRequest, translatedText string, confidence *float64, source string) (*ItemTranslation, error) {
	translation := &ItemTranslation{
		ID:                uuid.New(),
		OriginalText:      req.OriginalText,
		OriginalLanguage:  req.OriginalLanguage,
		TranslatedText:    translatedText,
		TargetLanguage:    req.TargetLanguage,
		Category:          req.Category,
		Confidence:        confidence,
		TranslationSource: source,
		UsageCount:        1,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	query := `
		INSERT INTO item_translations (
			id, original_text, original_language, translated_text, target_language,
			category, confidence, translation_source, usage_count, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (original_text, original_language, target_language)
		DO UPDATE SET
			confidence = GREATEST(item_translations.confidence, EXCLUDED.confidence),
			usage_count = item_translations.usage_count + 1,
			updated_at = NOW()
		RETURNING id, original_text, original_language, translated_text, target_language,
		         category, confidence, translation_source, usage_count, created_at, updated_at`

	err := s.db.QueryRow(ctx, query,
		translation.ID, translation.OriginalText, translation.OriginalLanguage,
		translation.TranslatedText, translation.TargetLanguage, translation.Category,
		translation.Confidence, translation.TranslationSource, translation.UsageCount,
		translation.CreatedAt, translation.UpdatedAt,
	).Scan(
		&translation.ID, &translation.OriginalText, &translation.OriginalLanguage,
		&translation.TranslatedText, &translation.TargetLanguage, &translation.Category,
		&translation.Confidence, &translation.TranslationSource, &translation.UsageCount,
		&translation.CreatedAt, &translation.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to store item translation: %w", err)
	}

	return translation, nil
}

func (s *TranslationService) NormalizeLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))

	switch lang {
	case "en", "en-us", "en-gb", "eng", "english":
		return "en"
	case "ru", "ru-ru", "rus", "russian":
		return "ru"
	case "uk", "uk-ua", "ua", "ukrainian", "укр":
		return "uk"
	case "es", "es-es", "spanish", "español":
		return "es"
	default:
		if parts := strings.Split(lang, "-"); len(parts) > 0 {
			primary := parts[0]
			switch primary {
			case "en", "ru", "uk", "es":
				return primary
			}
		}
		return "unknown"
	}
}

func (s *TranslationService) GetSupportedLanguages() []string {
	return []string{"en", "ru", "uk", "es"}
}

func (s *TranslationService) IsLanguageSupported(lang string) bool {
	normalizedLang := s.NormalizeLanguage(lang)
	supported := s.GetSupportedLanguages()

	for _, supportedLang := range supported {
		if normalizedLang == supportedLang {
			return true
		}
	}
	return false
}

func floatPtr(f float64) *float64 {
	return &f
}

// NormalizeLanguageCode normalizes a language code to a standard format
func NormalizeLanguageCode(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))

	switch lang {
	case "en", "en-us", "en-gb", "eng", "english":
		return "en"
	case "ru", "ru-ru", "rus", "russian":
		return "ru"
	case "uk", "uk-ua", "ua", "ukrainian", "укр":
		return "uk"
	case "es", "es-es", "spanish", "español":
		return "es"
	default:
		if parts := strings.Split(lang, "-"); len(parts) > 0 {
			primary := parts[0]
			switch primary {
			case "en", "ru", "uk", "es":
				return primary
			}
		}
		return "unknown"
	}
}
