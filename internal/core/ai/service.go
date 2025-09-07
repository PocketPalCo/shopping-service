package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("ai-service")

type OriginalItem struct {
	ID           uuid.UUID `json:"id" db:"id"`
	RawText      string    `json:"raw_text" db:"raw_text"`
	LanguageCode string    `json:"language_code" db:"language_code"`
	QuantityRaw  *string   `json:"quantity_raw" db:"quantity_raw"`
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type ParsedItem struct {
	ID               uuid.UUID `json:"id" db:"id"`
	StandardizedName string    `json:"standardized_name" db:"standardized_name"`
	Category         *string   `json:"category" db:"category"`
	Subcategory      *string   `json:"subcategory" db:"subcategory"`
	QuantityValue    *float64  `json:"quantity_value" db:"quantity_value"`
	QuantityUnit     *string   `json:"quantity_unit" db:"quantity_unit"`
	LanguageCode     string    `json:"language_code" db:"language_code"`
	ConfidenceScore  *float64  `json:"confidence_score" db:"confidence_score"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type ItemMapping struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	OriginalItemID uuid.UUID  `json:"original_item_id" db:"original_item_id"`
	ParsedItemID   uuid.UUID  `json:"parsed_item_id" db:"parsed_item_id"`
	MappingMethod  string     `json:"mapping_method" db:"mapping_method"`
	IsValidated    bool       `json:"is_validated" db:"is_validated"`
	ValidatedBy    *uuid.UUID `json:"validated_by" db:"validated_by"`
	ValidatedAt    *time.Time `json:"validated_at" db:"validated_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type ParsedResult struct {
	StandardizedName string     `json:"standardized_name"`
	Category         string     `json:"category"`
	Subcategory      string     `json:"subcategory"`
	QuantityValue    *float64   `json:"quantity_value,omitempty"`
	QuantityUnit     string     `json:"quantity_unit"`
	ConfidenceScore  float64    `json:"confidence_score"`
	OriginalItemID   *uuid.UUID `json:"original_item_id,omitempty"`
	ParsedItemID     *uuid.UUID `json:"parsed_item_id,omitempty"`
}

type OpenAIClient interface {
	ParseItem(ctx context.Context, rawText, languageCode string) (*ParsedResult, error)
	ParseItems(ctx context.Context, rawText, languageCode string) ([]*ParsedResult, error)
	DetectLanguage(ctx context.Context, text string) (string, error)
}

type Service struct {
	db           *pgxpool.Pool
	openaiClient OpenAIClient
	logger       *slog.Logger
}

func NewService(db *pgxpool.Pool, openaiClient OpenAIClient, logger *slog.Logger) *Service {
	return &Service{
		db:           db,
		openaiClient: openaiClient,
		logger:       logger,
	}
}

// ParseAndStoreItem processes raw item text through AI and stores all data for training
func (s *Service) ParseAndStoreItem(ctx context.Context, rawText, languageCode string, userID uuid.UUID) (*ParsedResult, error) {
	ctx, span := tracer.Start(ctx, "ai.ParseAndStoreItem")
	defer span.End()

	// Extract quantity from raw text (simple implementation)
	itemText, quantityRaw := s.extractQuantity(rawText)

	// Store original item
	originalItem, err := s.storeOriginalItem(ctx, rawText, languageCode, quantityRaw, userID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to store original item: %w", err)
	}

	// Parse item using AI
	parsedResult, err := s.openaiClient.ParseItem(ctx, itemText, languageCode)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to parse item with AI", "error", err, "raw_text", rawText)
		return nil, fmt.Errorf("failed to parse item: %w", err)
	}

	// Store parsed item
	parsedItem, err := s.storeParsedItem(ctx, parsedResult, languageCode)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to store parsed item: %w", err)
	}

	// Create mapping
	err = s.createItemMapping(ctx, originalItem.ID, parsedItem.ID, "ai_openai")
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to create item mapping", "error", err)
		// Don't return error here, parsing was successful
	}

	return parsedResult, nil
}

// GetOrCreateParsedItem gets existing parsed result or creates new one
func (s *Service) GetOrCreateParsedItem(ctx context.Context, rawText, languageCode string, userID uuid.UUID) (*ParsedResult, error) {
	ctx, span := tracer.Start(ctx, "ai.GetOrCreateParsedItem")
	defer span.End()

	// Try to find existing parsed result for this raw text
	existing, err := s.findExistingParsedItem(ctx, rawText, languageCode)
	if err == nil && existing != nil {
		return existing, nil
	}

	// If not found, parse and store new
	return s.ParseAndStoreItem(ctx, rawText, languageCode, userID)
}

func (s *Service) storeOriginalItem(ctx context.Context, rawText, languageCode string, quantityRaw *string, userID uuid.UUID) (*OriginalItem, error) {
	query := `
		INSERT INTO original_items (raw_text, language_code, quantity_raw, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, raw_text, language_code, quantity_raw, user_id, created_at, updated_at
	`

	var item OriginalItem
	err := s.db.QueryRow(ctx, query, rawText, languageCode, quantityRaw, userID).Scan(
		&item.ID,
		&item.RawText,
		&item.LanguageCode,
		&item.QuantityRaw,
		&item.UserID,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert original item: %w", err)
	}

	return &item, nil
}

func (s *Service) storeParsedItem(ctx context.Context, result *ParsedResult, languageCode string) (*ParsedItem, error) {
	// Check if this standardized item already exists for this language
	existingQuery := `
		SELECT id, standardized_name, category, subcategory, quantity_value, quantity_unit, 
		       language_code, confidence_score, created_at, updated_at
		FROM parsed_items 
		WHERE standardized_name = $1 AND language_code = $2
	`

	var existing ParsedItem
	err := s.db.QueryRow(ctx, existingQuery, result.StandardizedName, languageCode).Scan(
		&existing.ID,
		&existing.StandardizedName,
		&existing.Category,
		&existing.Subcategory,
		&existing.QuantityValue,
		&existing.QuantityUnit,
		&existing.LanguageCode,
		&existing.ConfidenceScore,
		&existing.CreatedAt,
		&existing.UpdatedAt,
	)

	if err == nil {
		// Found existing, return it
		return &existing, nil
	}

	// Insert new parsed item
	insertQuery := `
		INSERT INTO parsed_items (standardized_name, category, subcategory, quantity_value, 
		                         quantity_unit, language_code, confidence_score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, standardized_name, category, subcategory, quantity_value, quantity_unit, 
		         language_code, confidence_score, created_at, updated_at
	`

	var item ParsedItem
	err = s.db.QueryRow(ctx, insertQuery,
		result.StandardizedName,
		result.Category,
		result.Subcategory,
		result.QuantityValue,
		result.QuantityUnit,
		languageCode,
		result.ConfidenceScore,
	).Scan(
		&item.ID,
		&item.StandardizedName,
		&item.Category,
		&item.Subcategory,
		&item.QuantityValue,
		&item.QuantityUnit,
		&item.LanguageCode,
		&item.ConfidenceScore,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert parsed item: %w", err)
	}

	return &item, nil
}

func (s *Service) createItemMapping(ctx context.Context, originalID, parsedID uuid.UUID, method string) error {
	query := `
		INSERT INTO item_mappings (original_item_id, parsed_item_id, mapping_method, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (original_item_id) DO NOTHING
	`

	_, err := s.db.Exec(ctx, query, originalID, parsedID, method)
	return err
}

func (s *Service) findExistingParsedItem(ctx context.Context, rawText, languageCode string) (*ParsedResult, error) {
	query := `
		SELECT pi.standardized_name, pi.category, pi.subcategory, pi.quantity_value, 
		       pi.quantity_unit, pi.confidence_score
		FROM original_items oi
		JOIN item_mappings im ON oi.id = im.original_item_id
		JOIN parsed_items pi ON im.parsed_item_id = pi.id
		WHERE oi.raw_text = $1 AND oi.language_code = $2
		ORDER BY oi.created_at DESC
		LIMIT 1
	`

	var result ParsedResult
	var category, subcategory, quantityUnit *string
	var quantityValue *float64

	err := s.db.QueryRow(ctx, query, rawText, languageCode).Scan(
		&result.StandardizedName,
		&category,
		&subcategory,
		&quantityValue,
		&quantityUnit,
		&result.ConfidenceScore,
	)

	if err != nil {
		return nil, err
	}

	if category != nil {
		result.Category = *category
	}
	if subcategory != nil {
		result.Subcategory = *subcategory
	}
	if quantityUnit != nil {
		result.QuantityUnit = *quantityUnit
	}
	result.QuantityValue = quantityValue

	return &result, nil
}

// extractQuantity extracts quantity information from raw text
func (s *Service) extractQuantity(rawText string) (itemText string, quantity *string) {
	text := strings.TrimSpace(rawText)

	// Simple quantity extraction - look for patterns like "2kg", "1L", "500g", etc.
	// This will be enhanced by the AI parsing later

	// Check for parentheses format: "Milk (2L)"
	if strings.Contains(text, "(") && strings.Contains(text, ")") {
		parts := strings.Split(text, "(")
		if len(parts) == 2 {
			itemText = strings.TrimSpace(parts[0])
			quantityText := strings.TrimSpace(strings.TrimSuffix(parts[1], ")"))
			if quantityText != "" {
				return itemText, &quantityText
			}
		}
	}

	// Check if last part could be quantity: "Apples 2kg"
	parts := strings.Fields(text)
	if len(parts) >= 2 {
		lastPart := parts[len(parts)-1]
		// Simple heuristic: if last part contains numbers and is short, might be quantity
		if containsDigit(lastPart) && len(lastPart) <= 10 {
			itemText = strings.Join(parts[:len(parts)-1], " ")
			return itemText, &lastPart
		}
	}

	// No quantity found, return original text
	return text, nil
}

// ParseAndStoreItems processes raw text through AI and stores training data in database
func (s *Service) ParseAndStoreItems(ctx context.Context, rawText, languageCode string, userID uuid.UUID) ([]*ParsedResult, error) {
	ctx, span := tracer.Start(ctx, "ai.ParseAndStoreItems")
	defer span.End()

	// Always use multi-item parsing - it can handle single items too and is more intelligent
	s.logger.Info("Starting AI parsing",
		"raw_text", rawText,
		"language", languageCode,
		"user_id", userID)

	parsedResults, err := s.openaiClient.ParseItems(ctx, rawText, languageCode)
	if err != nil {
		s.logger.Error("Failed to parse items with AI",
			"error", err,
			"raw_text", rawText,
			"language", languageCode,
			"user_id", userID)
		return nil, fmt.Errorf("failed to parse items: %w", err)
	}

	// Log what the AI returned for debugging - each item individually
	s.logger.Info("AI parsing completed",
		"raw_input", rawText,
		"language", languageCode,
		"results_count", len(parsedResults))

	for i, result := range parsedResults {
		s.logger.Info("AI parsed item",
			"index", i+1,
			"standardized_name", result.StandardizedName,
			"category", result.Category,
			"subcategory", result.Subcategory,
			"quantity_value", result.QuantityValue,
			"quantity_unit", result.QuantityUnit,
			"confidence_score", result.ConfidenceScore)
	}

	// Store training data for each parsed item - each gets its own original item record
	var results []*ParsedResult
	for i, parsedResult := range parsedResults {
		// Store individual original item for this specific parsed result
		// Use the standardized name as the original text to keep it within varchar(255) limits
		originalItemText := parsedResult.StandardizedName
		originalItem, err := s.storeOriginalItem(ctx, originalItemText, languageCode, nil, userID)
		if err != nil {
			s.logger.Error("Failed to store original item", "error", err, "original_text", originalItemText)
			continue // Skip this item but continue with others
		}

		// Store or find the parsed item
		parsedItem, err := s.storeParsedItem(ctx, parsedResult, languageCode)
		if err != nil {
			s.logger.Error("Failed to store parsed item", "error", err, "item", parsedResult.StandardizedName)
			continue // Skip this item but continue with others
		}

		// Store the mapping between this specific original and parsed item
		err = s.storeItemMapping(ctx, originalItem.ID, parsedItem.ID, "ai_openai", parsedResult.ConfidenceScore, userID)
		if err != nil {
			s.logger.Error("Failed to store item mapping", "error", err, "original_id", originalItem.ID, "parsed_id", parsedItem.ID)
			// Don't skip - the items are stored, just the mapping failed
		}

		s.logger.Info("Successfully stored training data for item",
			"original", originalItemText,
			"parsed", parsedResult.StandardizedName,
			"category", parsedResult.Category,
			"confidence", parsedResult.ConfidenceScore,
			"item_index", i+1,
			"total_items", len(parsedResults))

		// Attach the IDs to the parsed result so they can be used by the shopping service
		parsedResult.OriginalItemID = &originalItem.ID
		parsedResult.ParsedItemID = &parsedItem.ID

		results = append(results, parsedResult)
	}

	s.logger.Info("Successfully parsed and stored items with AI",
		"raw_input", rawText,
		"language", languageCode,
		"total_parsed", len(parsedResults),
		"successfully_stored", len(results))

	return results, nil
}

// storeItemMapping creates a mapping between an original and parsed item
func (s *Service) storeItemMapping(ctx context.Context, originalItemID, parsedItemID uuid.UUID, mappingMethod string, confidence float64, userID uuid.UUID) error {
	query := `
		INSERT INTO item_mappings (original_item_id, parsed_item_id, mapping_method, is_validated, created_at, updated_at)
		VALUES ($1, $2, $3, false, NOW(), NOW())
		ON CONFLICT (original_item_id) DO UPDATE SET
			parsed_item_id = EXCLUDED.parsed_item_id,
			mapping_method = EXCLUDED.mapping_method,
			updated_at = NOW()
	`

	_, err := s.db.Exec(ctx, query, originalItemID, parsedItemID, mappingMethod)
	if err != nil {
		return fmt.Errorf("failed to store item mapping: %w", err)
	}

	s.logger.Debug("Stored item mapping",
		"original_item_id", originalItemID,
		"parsed_item_id", parsedItemID,
		"method", mappingMethod,
		"confidence", confidence)

	return nil
}

// containsDigit checks if a string contains any digit
func containsDigit(s string) bool {
	for _, char := range s {
		if char >= '0' && char <= '9' {
			return true
		}
	}
	return false
}
