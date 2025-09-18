package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/config"
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
	Notes            *string   `json:"notes" db:"notes"`
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
	Notes            *string    `json:"notes,omitempty"`
	ConfidenceScore  float64    `json:"confidence_score"`
	OriginalItemID   *uuid.UUID `json:"original_item_id,omitempty"`
	ParsedItemID     *uuid.UUID `json:"parsed_item_id,omitempty"`
}

type ProductListDetectionResult struct {
	IsProductList      bool     `json:"is_product_list"`
	Confidence         float64  `json:"confidence"`
	DetectedItemsCount int      `json:"detected_items_count"`
	SampleItems        []string `json:"sample_items"`
}

type TranslationResult struct {
	TranslatedText string  `json:"translated_text"`
	Confidence     float64 `json:"confidence"`
}

type ReceiptItemTranslation struct {
	OriginalText     string  `json:"original_text"`
	TranslatedText   string  `json:"translated_text"`
	DetectedLanguage string  `json:"detected_language"`
	TargetLanguage   string  `json:"target_language"`
	Confidence       float64 `json:"confidence"`
}

type BatchTranslationRequest struct {
	Items        []string `json:"items"`
	TargetLocale string   `json:"target_locale"`
}

type BatchTranslationResult struct {
	DetectedLanguage string                   `json:"detected_language"`
	TargetLanguage   string                   `json:"target_language"`
	Translations     []ReceiptItemTranslation `json:"translations"`
	Confidence       float64                  `json:"confidence"`
}

type TranslationCache struct {
	ID               uuid.UUID `json:"id" db:"id"`
	OriginalItem     string    `json:"original_item" db:"original_item"`
	ItemHash         string    `json:"item_hash" db:"item_hash"`
	TargetLocale     string    `json:"target_locale" db:"target_locale"`
	AIResponse       string    `json:"ai_response" db:"ai_response"` // JSONB as string
	DetectedLanguage *string   `json:"detected_language" db:"detected_language"`
	Confidence       *float64  `json:"confidence" db:"confidence"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type OpenAIClient interface {
	ParseItem(ctx context.Context, rawText, languageCode string) (*ParsedResult, error)
	ParseItems(ctx context.Context, rawText, languageCode string) ([]*ParsedResult, error)
	DetectLanguage(ctx context.Context, text string) (string, error)
	DetectProductList(ctx context.Context, text string) (*ProductListDetectionResult, error)
	Translate(ctx context.Context, originalText, originalLanguage, targetLanguage string) (*TranslationResult, error)
	BatchTranslateReceiptItems(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error)
}

type Service struct {
	db                   *pgxpool.Pool
	openaiClient         OpenAIClient
	DocumentIntelligence *DocumentIntelligenceService
	logger               *slog.Logger
}

func NewService(db *pgxpool.Pool, openaiClient OpenAIClient, cfg config.Config, logger *slog.Logger) (*Service, error) {
	// Initialize Document Intelligence service
	docIntelConfig := cfg.GetDocumentIntelligenceConfig()
	docIntelService := NewDocumentIntelligenceService(
		docIntelConfig.Endpoint,
		docIntelConfig.APIKey,
		docIntelConfig.APIVersion,
		docIntelConfig.Model,
	)

	// Validate configuration if endpoints are provided
	if docIntelConfig.Endpoint != "" || docIntelConfig.APIKey != "" {
		if err := docIntelService.ValidateConfiguration(); err != nil {
			logger.Warn("Document Intelligence service not configured properly", "error", err)
		}
	}

	return &Service{
		db:                   db,
		openaiClient:         openaiClient,
		DocumentIntelligence: docIntelService,
		logger:               logger,
	}, nil
}

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
		                         quantity_unit, notes, language_code, confidence_score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, standardized_name, category, subcategory, quantity_value, quantity_unit, 
		         notes, language_code, confidence_score, created_at, updated_at
	`

	var item ParsedItem
	err = s.db.QueryRow(ctx, insertQuery,
		result.StandardizedName,
		result.Category,
		result.Subcategory,
		result.QuantityValue,
		result.QuantityUnit,
		result.Notes,
		languageCode,
		result.ConfidenceScore,
	).Scan(
		&item.ID,
		&item.StandardizedName,
		&item.Category,
		&item.Subcategory,
		&item.QuantityValue,
		&item.QuantityUnit,
		&item.Notes,
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
		ON CONFLICT (original_item_id, parsed_item_id) DO NOTHING
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
		notesValue := "nil"
		if result.Notes != nil {
			notesValue = fmt.Sprintf("'%s'", *result.Notes)
		}
		s.logger.Info("AI parsed item",
			"index", i+1,
			"standardized_name", result.StandardizedName,
			"category", result.Category,
			"subcategory", result.Subcategory,
			"quantity_value", result.QuantityValue,
			"quantity_unit", result.QuantityUnit,
			"notes", notesValue,
			"confidence_score", result.ConfidenceScore)
	}

	// Apply confidence threshold filtering - items with confidence < 0.60 should be treated as notes
	const confidenceThreshold = 0.60
	var filteredResults []*ParsedResult
	var lowConfidenceItems []string

	for _, result := range parsedResults {
		if result.ConfidenceScore >= confidenceThreshold {
			filteredResults = append(filteredResults, result)
		} else {
			s.logger.Info("Low confidence item excluded as separate item",
				"item", result.StandardizedName,
				"confidence", result.ConfidenceScore,
				"threshold", confidenceThreshold)
			lowConfidenceItems = append(lowConfidenceItems, result.StandardizedName)
		}
	}

	// If we have valid items and low confidence items, add low confidence items as notes to the first valid item
	if len(filteredResults) > 0 && len(lowConfidenceItems) > 0 {
		firstItem := filteredResults[0]

		// Combine existing notes with low confidence items
		var notes []string
		if firstItem.Notes != nil && *firstItem.Notes != "" {
			notes = append(notes, *firstItem.Notes)
		}
		notes = append(notes, lowConfidenceItems...)

		combinedNotes := strings.Join(notes, ", ")
		firstItem.Notes = &combinedNotes

		s.logger.Info("Added low confidence items as notes to first item",
			"main_item", firstItem.StandardizedName,
			"notes", combinedNotes)
	}

	// If all items have low confidence, keep the highest confidence one
	if len(filteredResults) == 0 && len(parsedResults) > 0 {
		var highestConfidenceItem *ParsedResult
		for _, result := range parsedResults {
			if highestConfidenceItem == nil || result.ConfidenceScore > highestConfidenceItem.ConfidenceScore {
				highestConfidenceItem = result
			}
		}
		filteredResults = append(filteredResults, highestConfidenceItem)
		s.logger.Info("All items had low confidence, keeping highest confidence item",
			"item", highestConfidenceItem.StandardizedName,
			"confidence", highestConfidenceItem.ConfidenceScore)
	}

	// Store training data for each filtered item - each gets its own original item record
	var results []*ParsedResult
	for i, parsedResult := range filteredResults {
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
			"total_items", len(filteredResults))

		// Attach the IDs to the parsed result so they can be used by the shopping service
		parsedResult.OriginalItemID = &originalItem.ID
		parsedResult.ParsedItemID = &parsedItem.ID

		results = append(results, parsedResult)
	}

	s.logger.Info("Successfully parsed and stored items with AI",
		"raw_input", rawText,
		"language", languageCode,
		"total_ai_parsed", len(parsedResults),
		"filtered_results", len(filteredResults),
		"successfully_stored", len(results))

	return results, nil
}

func (s *Service) storeItemMapping(ctx context.Context, originalItemID, parsedItemID uuid.UUID, mappingMethod string, confidence float64, userID uuid.UUID) error {
	query := `
		INSERT INTO item_mappings (original_item_id, parsed_item_id, mapping_method, is_validated, created_at, updated_at)
		VALUES ($1, $2, $3, false, NOW(), NOW())
		ON CONFLICT (original_item_id, parsed_item_id) DO UPDATE SET
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

func containsDigit(s string) bool {
	for _, char := range s {
		if char >= '0' && char <= '9' {
			return true
		}
	}
	return false
}

func (s *Service) DetectProductList(ctx context.Context, text string) (*ProductListDetectionResult, error) {
	ctx, span := tracer.Start(ctx, "ai.DetectProductList")
	defer span.End()

	result, err := s.openaiClient.DetectProductList(ctx, text)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to detect product list with AI", "error", err, "text", text)
		return nil, fmt.Errorf("failed to detect product list: %w", err)
	}

	s.logger.Info("Successfully detected product list with AI",
		"text", text,
		"is_product_list", result.IsProductList,
		"confidence", result.Confidence,
		"detected_items_count", result.DetectedItemsCount)

	return result, nil
}

func (s *Service) DetectLanguage(ctx context.Context, text string) (string, error) {
	return s.openaiClient.DetectLanguage(ctx, text)
}

func (s *Service) Translate(ctx context.Context, originalText, originalLanguage, targetLanguage string) (*TranslationResult, error) {
	return s.openaiClient.Translate(ctx, originalText, originalLanguage, targetLanguage)
}

func (s *Service) BatchTranslateReceiptItems(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	ctx, span := tracer.Start(ctx, "ai.BatchTranslateReceiptItems")
	defer span.End()

	s.logger.Info("Starting batch translation of receipt items with cache lookup",
		"items_count", len(req.Items),
		"target_locale", req.TargetLocale)

	// Step 1: Check cache for existing translations
	cachedResults, uncachedItems, err := s.getCachedTranslations(ctx, req.Items, req.TargetLocale)
	if err != nil {
		s.logger.Warn("Failed to lookup translation cache, proceeding without cache",
			"error", err)
		uncachedItems = req.Items
	}

	// Step 2: Process uncached items with AI if any
	var aiResult *BatchTranslationResult
	if len(uncachedItems) > 0 {
		uncachedReq := &BatchTranslationRequest{
			Items:        uncachedItems,
			TargetLocale: req.TargetLocale,
		}

		aiResult, err = s.openaiClient.BatchTranslateReceiptItems(ctx, uncachedReq)
		if err != nil {
			span.RecordError(err)
			s.logger.Error("Failed to batch translate receipt items with AI",
				"error", err,
				"items_count", len(uncachedItems),
				"target_locale", req.TargetLocale)
			return nil, fmt.Errorf("failed to batch translate receipt items: %w", err)
		}

		// Step 3: Store AI results in cache
		for _, item := range uncachedItems {
			if err := s.storeCachedTranslation(ctx, item, req.TargetLocale, aiResult); err != nil {
				s.logger.Warn("Failed to store translation in cache", "item", item, "error", err)
			}
		}
	}

	// Step 4: Combine cached and AI results
	finalResult := &BatchTranslationResult{
		TargetLanguage: req.TargetLocale,
		Translations:   make([]ReceiptItemTranslation, 0),
	}

	// Add cached translations
	for _, cachedResult := range cachedResults {
		finalResult.Translations = append(finalResult.Translations, cachedResult.Translations...)
		if finalResult.DetectedLanguage == "" {
			finalResult.DetectedLanguage = cachedResult.DetectedLanguage
		}
		finalResult.Confidence = cachedResult.Confidence // Use last confidence
	}

	// Add AI translations
	if aiResult != nil {
		finalResult.Translations = append(finalResult.Translations, aiResult.Translations...)
		if finalResult.DetectedLanguage == "" {
			finalResult.DetectedLanguage = aiResult.DetectedLanguage
		}
		finalResult.Confidence = aiResult.Confidence // Use AI confidence if available
	}

	s.logger.Info("Successfully completed batch translation with cache optimization",
		"total_items", len(req.Items),
		"cached_items", len(cachedResults),
		"ai_processed_items", len(uncachedItems),
		"target_locale", req.TargetLocale,
		"detected_language", finalResult.DetectedLanguage,
		"final_translations_count", len(finalResult.Translations),
		"confidence", finalResult.Confidence)

	return finalResult, nil
}

// normalizeItem normalizes an item name for consistent cache lookups
func (s *Service) normalizeItem(item string) string {
	// Normalize to lowercase and trim spaces for consistent hashing
	return strings.TrimSpace(strings.ToLower(item))
}

// hashItem creates a SHA256 hash of the normalized item name
func (s *Service) hashItem(item string) string {
	normalized := s.normalizeItem(item)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// getCachedTranslations looks up cached translations for items
func (s *Service) getCachedTranslations(ctx context.Context, items []string, targetLocale string) (map[string]*BatchTranslationResult, []string, error) {
	cached := make(map[string]*BatchTranslationResult)
	var uncachedItems []string

	for _, item := range items {
		itemHash := s.hashItem(item)

		var cache TranslationCache
		query := `SELECT id, original_item, item_hash, target_locale, ai_response, detected_language, confidence, created_at, updated_at
				  FROM translation_cache
				  WHERE item_hash = $1 AND target_locale = $2`

		err := s.db.QueryRow(ctx, query, itemHash, targetLocale).Scan(
			&cache.ID, &cache.OriginalItem, &cache.ItemHash, &cache.TargetLocale,
			&cache.AIResponse, &cache.DetectedLanguage, &cache.Confidence,
			&cache.CreatedAt, &cache.UpdatedAt)

		if err != nil {
			// Not found in cache, add to uncached list
			uncachedItems = append(uncachedItems, item)
			continue
		}

		// Parse cached AI response
		var result BatchTranslationResult
		if err := json.Unmarshal([]byte(cache.AIResponse), &result); err != nil {
			s.logger.Warn("Failed to parse cached translation, will re-translate",
				"item", item, "error", err)
			uncachedItems = append(uncachedItems, item)
			continue
		}

		cached[item] = &result
		s.logger.Debug("Found cached translation", "item", item, "target_locale", targetLocale)
	}

	s.logger.Info("Translation cache lookup completed",
		"total_items", len(items),
		"cached_items", len(cached),
		"uncached_items", len(uncachedItems),
		"target_locale", targetLocale)

	return cached, uncachedItems, nil
}

// storeCachedTranslation stores a translation result in the cache
func (s *Service) storeCachedTranslation(ctx context.Context, item string, targetLocale string, result *BatchTranslationResult) error {
	itemHash := s.hashItem(item)
	normalized := s.normalizeItem(item)

	// Find the specific translation for this item in the batch result
	var itemTranslation *ReceiptItemTranslation
	for _, translation := range result.Translations {
		if s.normalizeItem(translation.OriginalText) == normalized {
			itemTranslation = &translation
			break
		}
	}

	if itemTranslation == nil {
		return fmt.Errorf("translation not found for item '%s' in batch result", item)
	}

	// Create individual translation result for this specific item
	individualResult := &BatchTranslationResult{
		DetectedLanguage: result.DetectedLanguage,
		TargetLanguage:   result.TargetLanguage,
		Translations:     []ReceiptItemTranslation{*itemTranslation},
		Confidence:       itemTranslation.Confidence, // Use item-specific confidence
	}

	// Serialize individual AI response to JSON
	aiResponseBytes, err := json.Marshal(individualResult)
	if err != nil {
		return fmt.Errorf("failed to serialize AI response: %w", err)
	}

	query := `INSERT INTO translation_cache (original_item, item_hash, target_locale, ai_response, detected_language, confidence)
			  VALUES ($1, $2, $3, $4, $5, $6)
			  ON CONFLICT (item_hash, target_locale) DO UPDATE SET
			  ai_response = EXCLUDED.ai_response,
			  detected_language = EXCLUDED.detected_language,
			  confidence = EXCLUDED.confidence,
			  updated_at = CURRENT_TIMESTAMP`

	var detectedLang *string
	if result.DetectedLanguage != "" {
		detectedLang = &result.DetectedLanguage
	}

	var confidence *float64
	if itemTranslation.Confidence > 0 {
		confidence = &itemTranslation.Confidence
	}

	_, err = s.db.Exec(ctx, query, normalized, itemHash, targetLocale, string(aiResponseBytes), detectedLang, confidence)
	if err != nil {
		return fmt.Errorf("failed to store cached translation: %w", err)
	}

	s.logger.Debug("Stored individual translation in cache",
		"item", item,
		"target_locale", targetLocale,
		"translated_text", itemTranslation.TranslatedText,
		"confidence", itemTranslation.Confidence)
	return nil
}
