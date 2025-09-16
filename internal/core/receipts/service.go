package receipts

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log/slog"
	netURL "net/url"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/ai"
	"github.com/PocketPalCo/shopping-service/internal/core/cloud"
	"github.com/PocketPalCo/shopping-service/internal/core/translations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("receipts-service")

// aiServiceAdapter adapts ai.Service to work with translations.AIService interface
type aiServiceAdapter struct {
	aiService *ai.Service
}

func (a *aiServiceAdapter) Translate(ctx context.Context, originalText, originalLanguage, targetLanguage string) (*translations.TranslationAIResult, error) {
	result, err := a.aiService.Translate(ctx, originalText, originalLanguage, targetLanguage)
	if err != nil {
		return nil, err
	}
	return &translations.TranslationAIResult{
		TranslatedText: result.TranslatedText,
		Confidence:     result.Confidence,
	}, nil
}

// Service handles receipt-related operations
type Service struct {
	db                 *pgxpool.Pool
	cloudService       *cloud.Service
	aiService          *ai.Service
	translationService *translations.TranslationService
	logger             *slog.Logger
}

func NewService(db *pgxpool.Pool, cloudService *cloud.Service, aiService *ai.Service, logger *slog.Logger) *Service {
	aiAdapter := &aiServiceAdapter{aiService: aiService}
	return &Service{
		db:                 db,
		cloudService:       cloudService,
		aiService:          aiService,
		translationService: translations.NewTranslationService(db, logger, aiAdapter),
		logger:             logger,
	}
}

func (s *Service) CreateReceipt(ctx context.Context, req CreateReceiptRequest) (*Receipt, error) {
	ctx, span := tracer.Start(ctx, "receipts.CreateReceipt")
	defer span.End()

	if !s.isValidFileType(req.ContentType) {
		return nil, fmt.Errorf("invalid content type: %s. Supported types: images and PDF documents", req.ContentType)
	}

	ext := s.getFileExtension(req.ContentType)
	fileID := uuid.New().String()
	fileName := fmt.Sprintf("receipt_%s%s", fileID, ext)

	uploadChan := make(chan *cloud.FileUploadResult, 1)
	uploadErrChan := make(chan error, 1)
	processChan := make(chan *ai.ReceiptData, 1)
	processErrChan := make(chan error, 1)

	go func() {
		uploadResult, err := s.cloudService.UploadReceiptFile(
			ctx,
			req.UserID.String(),
			fileName,
			bytes.NewReader(req.FileData),
			req.ContentType,
			req.FileSize,
		)
		if err != nil {
			uploadErrChan <- err
			return
		}
		uploadChan <- uploadResult
	}()

	go func() {
		receiptData, err := s.aiService.DocumentIntelligence.AnalyzeReceipt(ctx, req.FileData, req.ContentType)
		if err != nil {
			processErrChan <- err
			return
		}

		// Use AI to detect language and batch translate item descriptions
		if len(receiptData.Items) > 0 {
			itemDescriptions := make([]string, 0, len(receiptData.Items))
			for _, item := range receiptData.Items {
				if item.Name != "" {
					itemDescriptions = append(itemDescriptions, item.Name)
				}
			}

			if len(itemDescriptions) > 0 {
				// Get user's locale for translation target
				userLocale, err := s.getUserLocale(ctx, req.UserID)
				if err != nil {
					s.logger.Warn("Failed to get user locale, using default",
						"error", err, "user_id", req.UserID, "default_locale", "en")
					userLocale = "en"
				}

				// Use batch translation service to detect language and translate all items
				batchReq := &ai.BatchTranslationRequest{
					Items:        itemDescriptions,
					TargetLocale: userLocale,
				}

				batchResult, err := s.aiService.BatchTranslateReceiptItems(ctx, batchReq)
				if err != nil {
					s.logger.Error("Failed to batch translate receipt items - translation is required",
						"error", err, "items_count", len(itemDescriptions))
					processErrChan <- fmt.Errorf("failed to translate receipt items: %w", err)
					return
				}

				// Store the batch translation results
				receiptData.DetectedLanguage = batchResult.DetectedLanguage
				receiptData.ContentLocale = s.generateContentLocale(batchResult.DetectedLanguage, receiptData.CountryRegion)
				receiptData.ItemTranslations = batchResult.Translations

				s.logger.Info("Successfully batch translated receipt items during upload",
					"receipt_items_count", len(itemDescriptions),
					"detected_language", batchResult.DetectedLanguage,
					"target_locale", userLocale,
					"translations_count", len(batchResult.Translations),
					"confidence", batchResult.Confidence)
			}
		}

		processChan <- receiptData
	}()

	var uploadResult *cloud.FileUploadResult
	var receiptData *ai.ReceiptData
	var uploadErr, processErr error

	for i := 0; i < 2; i++ {
		select {
		case uploadResult = <-uploadChan:
		case uploadErr = <-uploadErrChan:
		case receiptData = <-processChan:
		case processErr = <-processErrChan:
		case <-ctx.Done():
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		}
	}

	if uploadErr != nil {
		span.RecordError(uploadErr)
		s.logger.Error("Failed to upload receipt file",
			"error", uploadErr,
			"user_id", req.UserID,
			"file_name", req.FileName)
		return nil, fmt.Errorf("failed to upload receipt file: %w", uploadErr)
	}

	if processErr != nil {
		s.logger.Warn("Failed to process receipt during upload, will retry later",
			"error", processErr,
			"user_id", req.UserID,
			"file_name", req.FileName)
	}

	fileURL := uploadResult.PublicURL

	receipt := &Receipt{
		ID:             uuid.New(),
		UserID:         req.UserID,
		FileURL:        fileURL,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
		ContentType:    req.ContentType,
		TelegramFileID: req.TelegramFileID,
		Processed:      processErr == nil && receiptData != nil,
		ItemsCount:     0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if processErr == nil && receiptData != nil {
		receipt.ItemsCount = len(receiptData.Items)

		// Always set these fields even if empty, to show they were processed
		if receiptData.MerchantName != "" {
			receipt.MerchantName = &receiptData.MerchantName
		}
		if receiptData.Total > 0 {
			receipt.TotalAmount = &receiptData.Total
		}
		if receiptData.MerchantAddress != "" {
			receipt.MerchantAddress = &receiptData.MerchantAddress
		}
		if receiptData.MerchantPhone != "" {
			receipt.MerchantPhone = &receiptData.MerchantPhone
		}
		if receiptData.Currency != "" {
			receipt.CurrencyCode = &receiptData.Currency
		}
		if receiptData.Tax >= 0 {
			receipt.TotalTax = &receiptData.Tax
		}
		if receiptData.Subtotal >= 0 {
			receipt.NetAmount = &receiptData.Subtotal
		}
		if receiptData.ReceiptType != "" {
			receipt.ReceiptType = &receiptData.ReceiptType
		}
		if receiptData.CountryRegion != "" {
			receipt.CountryRegion = &receiptData.CountryRegion
		}
		if !receiptData.TransactionDate.IsZero() {
			receipt.TransactionDate = &receiptData.TransactionDate
		}
		if !receiptData.TransactionTime.IsZero() {
			receipt.TransactionTime = &receiptData.TransactionTime
		}

		// Use real AI confidence from response
		if receiptData.Confidence > 0 {
			receipt.AIConfidence = &receiptData.Confidence
		}

		// Use real model version from AI response
		if receiptData.ModelID != "" {
			// Combine model with API version for full context
			modelVersion := fmt.Sprintf("%s-%s", receiptData.ModelID, receiptData.APIVersion)
			receipt.ExtractionModelVersion = &modelVersion
		}

		// Store detected language and content locale
		if receiptData.DetectedLanguage != "" {
			receipt.DetectedLanguage = &receiptData.DetectedLanguage
		}
		if receiptData.ContentLocale != "" {
			receipt.ContentLocale = &receiptData.ContentLocale
		}

		// Store the raw AI response for debugging (could be made optional)
		// Note: This is for debugging purposes, might want to make this conditional
		s.logger.Info("AI processing results - detailed",
			"merchant_name", receiptData.MerchantName,
			"merchant_address", receiptData.MerchantAddress,
			"merchant_phone", receiptData.MerchantPhone,
			"total", receiptData.Total,
			"subtotal", receiptData.Subtotal,
			"tax", receiptData.Tax,
			"currency", receiptData.Currency,
			"transaction_date", receiptData.TransactionDate,
			"transaction_time", receiptData.TransactionTime,
			"items_count", len(receiptData.Items),
			"confidence", receiptData.Confidence,
			"model_id", receiptData.ModelID,
			"api_version", receiptData.APIVersion,
			"country_region", receiptData.CountryRegion,
			"receipt_type", receiptData.ReceiptType,
			"detected_language", receiptData.DetectedLanguage,
			"content_locale", receiptData.ContentLocale)

		// Log first few items to see their content
		for i, item := range receiptData.Items {
			if i >= 3 { // Only log first 3 items to avoid spam
				break
			}
			s.logger.Info("AI extracted item",
				"item_index", i,
				"name", item.Name,
				"quantity", item.Quantity,
				"price", item.Price,
				"total_price", item.TotalPrice,
				"category", item.Category,
				"description", item.Description)
		}

		s.logger.Info("Receipt processed successfully during upload",
			"receipt_id", receipt.ID,
			"user_id", req.UserID,
			"merchant_name", receiptData.MerchantName,
			"total_amount", receiptData.Total,
			"items_count", len(receiptData.Items))
	} else if processErr != nil {
		// Set processing error for later retry
		errorMsg := processErr.Error()
		receipt.ProcessingError = &errorMsg
	}

	query := `
		INSERT INTO users_receipts (
			id, user_id, file_url, file_name, file_size, content_type,
			telegram_file_id, processed, items_count, merchant_name, total_amount,
			transaction_date, merchant_address, merchant_phone, country_region, transaction_time,
			currency_code, total_tax, net_amount, receipt_type, processing_error,
			ai_confidence, extraction_model_version, detected_language, content_locale,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27)
		RETURNING id, user_id, file_url, file_name, file_size, content_type,
		         telegram_file_id, processed, processing_error, merchant_name,
		         total_amount, transaction_date, items_count, created_at, updated_at
	`

	err := s.db.QueryRow(ctx, query,
		receipt.ID, receipt.UserID, receipt.FileURL, receipt.FileName,
		receipt.FileSize, receipt.ContentType, receipt.TelegramFileID,
		receipt.Processed, receipt.ItemsCount, receipt.MerchantName, receipt.TotalAmount,
		receipt.TransactionDate, receipt.MerchantAddress, receipt.MerchantPhone, receipt.CountryRegion, receipt.TransactionTime,
		receipt.CurrencyCode, receipt.TotalTax, receipt.NetAmount, receipt.ReceiptType, receipt.ProcessingError,
		receipt.AIConfidence, receipt.ExtractionModelVersion, receipt.DetectedLanguage, receipt.ContentLocale,
		receipt.CreatedAt, receipt.UpdatedAt,
	).Scan(
		&receipt.ID, &receipt.UserID, &receipt.FileURL, &receipt.FileName,
		&receipt.FileSize, &receipt.ContentType, &receipt.TelegramFileID,
		&receipt.Processed, &receipt.ProcessingError, &receipt.MerchantName,
		&receipt.TotalAmount, &receipt.TransactionDate, &receipt.ItemsCount,
		&receipt.CreatedAt, &receipt.UpdatedAt,
	)

	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to create receipt record",
			"error", err,
			"user_id", req.UserID,
			"file_url", fileURL)

		// Try to clean up uploaded file
		if deleteErr := s.cloudService.DeleteFile(ctx, uploadResult.FileID); deleteErr != nil {
			s.logger.Warn("Failed to clean up uploaded file after database error",
				"error", deleteErr,
				"file_id", uploadResult.FileID)
		}

		return nil, fmt.Errorf("failed to create receipt record: %w", err)
	}

	// Store receipt items if processing was successful
	if processErr == nil && receiptData != nil && len(receiptData.Items) > 0 {
		if err := s.storeReceiptItemsWithTranslations(ctx, receipt.ID, req.UserID, receiptData.Items, receiptData.ItemTranslations, receiptData.DetectedLanguage); err != nil {
			s.logger.Error("Failed to store receipt items with translations",
				"error", err,
				"receipt_id", receipt.ID,
				"items_count", len(receiptData.Items))
			// Don't fail the entire operation, just log the error
		} else {
			s.logger.Info("Receipt items stored successfully with translations",
				"receipt_id", receipt.ID,
				"items_count", len(receiptData.Items),
				"translations_count", len(receiptData.ItemTranslations))
		}
	}

	s.logger.Info("Receipt uploaded successfully",
		"receipt_id", receipt.ID,
		"user_id", req.UserID,
		"file_name", req.FileName,
		"file_size", req.FileSize)

	return receipt, nil
}

// ProcessReceipt processes a receipt using AI document intelligence
// This method is now primarily used as a fallback for receipts that failed processing during upload
func (s *Service) ProcessReceipt(ctx context.Context, receiptID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "receipts.ProcessReceipt")
	defer span.End()

	// Get receipt from database
	receipt, err := s.GetReceipt(ctx, receiptID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to get receipt: %w", err)
	}

	if receipt.Processed {
		return fmt.Errorf("receipt %s is already processed", receiptID)
	}

	// Download file from cloud storage using the cloud service (fallback method)
	fileID, err := s.extractFileIDFromURL(receipt.FileURL)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to extract file ID from URL",
			"error", err,
			"receipt_id", receiptID,
			"file_url", receipt.FileURL)

		// Update with error
		s.updateProcessingStatus(ctx, receiptID, true, fmt.Sprintf("Failed to extract file ID: %v", err), nil)
		return fmt.Errorf("failed to extract file ID from URL: %w", err)
	}

	fileData, err := s.cloudService.DownloadFile(ctx, fileID)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to download receipt file for processing",
			"error", err,
			"receipt_id", receiptID,
			"file_id", fileID,
			"file_url", receipt.FileURL)

		// Update with error
		s.updateProcessingStatus(ctx, receiptID, true, fmt.Sprintf("Failed to download file: %v", err), nil)
		return fmt.Errorf("failed to download receipt file: %w", err)
	}

	// Process with Document Intelligence
	receiptData, err := s.aiService.DocumentIntelligence.AnalyzeReceipt(ctx, fileData, receipt.ContentType)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to process receipt with AI",
			"error", err,
			"receipt_id", receiptID)

		// Update with error
		s.updateProcessingStatus(ctx, receiptID, true, fmt.Sprintf("AI processing failed: %v", err), nil)
		return fmt.Errorf("failed to process receipt with AI: %w", err)
	}

	// Create enhanced update request with all available data
	updateReq := UpdateReceiptRequest{
		ID:           receiptID,
		Processed:    true,
		MerchantName: &receiptData.MerchantName,
		TotalAmount:  &receiptData.Total,
		ItemsCount:   len(receiptData.Items),
	}

	// Add enhanced fields
	if receiptData.MerchantAddress != "" {
		updateReq.MerchantAddress = &receiptData.MerchantAddress
	}
	if receiptData.Currency != "" {
		updateReq.CurrencyCode = &receiptData.Currency
	}
	if receiptData.Tax >= 0 {
		updateReq.TotalTax = &receiptData.Tax
	}
	if receiptData.Subtotal >= 0 {
		updateReq.NetAmount = &receiptData.Subtotal
	}
	if receiptData.ReceiptType != "" {
		updateReq.ReceiptType = &receiptData.ReceiptType
	}
	if !receiptData.TransactionDate.IsZero() {
		updateReq.TransactionDate = &receiptData.TransactionDate
	}

	// Set AI processing metadata
	confidence := 0.85 // Default confidence for Document Intelligence
	updateReq.AIConfidence = &confidence
	modelVersion := "prebuilt-receipt-2024-11-30"
	updateReq.ExtractionModelVersion = &modelVersion
	detectedLang := "en" // Default to English, could be enhanced with language detection
	updateReq.DetectedLanguage = &detectedLang

	// Update receipt with enhanced data
	err = s.UpdateReceipt(ctx, updateReq)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to update receipt with processing results",
			"error", err,
			"receipt_id", receiptID)
		return fmt.Errorf("failed to update receipt: %w", err)
	}

	// Process and store individual receipt items with localization support
	err = s.processReceiptItems(ctx, receiptID, receiptData.Items, detectedLang)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to process receipt items",
			"error", err,
			"receipt_id", receiptID)
		// Don't fail the entire operation, just log the error
	}

	s.logger.Info("Receipt processed successfully",
		"receipt_id", receiptID,
		"merchant", receiptData.MerchantName,
		"total", receiptData.Total,
		"items_count", len(receiptData.Items))

	return nil
}

// GetReceipt retrieves a receipt by ID
func (s *Service) GetReceipt(ctx context.Context, id uuid.UUID) (*Receipt, error) {
	ctx, span := tracer.Start(ctx, "receipts.GetReceipt")
	defer span.End()

	query := `
		SELECT id, user_id, file_url, file_name, file_size, content_type,
		       telegram_file_id, processed, processing_error, merchant_name,
		       total_amount, transaction_date, items_count, created_at, updated_at,
		       merchant_address, merchant_phone, country_region, transaction_time,
		       receipt_type, currency_code, total_tax, net_amount, ai_confidence,
		       extraction_model_version, raw_ai_response, detected_language,
		       content_locale, auto_translation_enabled, last_translation_update
		FROM users_receipts
		WHERE id = $1
	`

	var receipt Receipt
	err := s.db.QueryRow(ctx, query, id).Scan(
		&receipt.ID, &receipt.UserID, &receipt.FileURL, &receipt.FileName,
		&receipt.FileSize, &receipt.ContentType, &receipt.TelegramFileID,
		&receipt.Processed, &receipt.ProcessingError, &receipt.MerchantName,
		&receipt.TotalAmount, &receipt.TransactionDate, &receipt.ItemsCount,
		&receipt.CreatedAt, &receipt.UpdatedAt, &receipt.MerchantAddress,
		&receipt.MerchantPhone, &receipt.CountryRegion, &receipt.TransactionTime,
		&receipt.ReceiptType, &receipt.CurrencyCode, &receipt.TotalTax,
		&receipt.NetAmount, &receipt.AIConfidence, &receipt.ExtractionModelVersion,
		&receipt.RawAIResponse, &receipt.DetectedLanguage, &receipt.ContentLocale,
		&receipt.AutoTranslationEnabled, &receipt.LastTranslationUpdate,
	)

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get receipt: %w", err)
	}

	return &receipt, nil
}

// GetUserReceipts retrieves all receipts for a user
func (s *Service) GetUserReceipts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Receipt, error) {
	ctx, span := tracer.Start(ctx, "receipts.GetUserReceipts")
	defer span.End()

	query := `
		SELECT id, user_id, file_url, file_name, file_size, content_type,
		       telegram_file_id, processed, processing_error, merchant_name,
		       total_amount, transaction_date, items_count, created_at, updated_at,
		       merchant_address, merchant_phone, country_region, transaction_time,
		       receipt_type, currency_code, total_tax, net_amount, ai_confidence,
		       extraction_model_version, raw_ai_response, detected_language,
		       content_locale, auto_translation_enabled, last_translation_update
		FROM users_receipts
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get user receipts: %w", err)
	}
	defer rows.Close()

	var receipts []*Receipt
	for rows.Next() {
		var receipt Receipt
		err := rows.Scan(
			&receipt.ID, &receipt.UserID, &receipt.FileURL, &receipt.FileName,
			&receipt.FileSize, &receipt.ContentType, &receipt.TelegramFileID,
			&receipt.Processed, &receipt.ProcessingError, &receipt.MerchantName,
			&receipt.TotalAmount, &receipt.TransactionDate, &receipt.ItemsCount,
			&receipt.CreatedAt, &receipt.UpdatedAt, &receipt.MerchantAddress,
			&receipt.MerchantPhone, &receipt.CountryRegion, &receipt.TransactionTime,
			&receipt.ReceiptType, &receipt.CurrencyCode, &receipt.TotalTax,
			&receipt.NetAmount, &receipt.AIConfidence, &receipt.ExtractionModelVersion,
			&receipt.RawAIResponse, &receipt.DetectedLanguage, &receipt.ContentLocale,
			&receipt.AutoTranslationEnabled, &receipt.LastTranslationUpdate,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan receipt: %w", err)
		}
		receipts = append(receipts, &receipt)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate receipts: %w", err)
	}

	return receipts, nil
}

// UpdateReceipt updates a receipt with processing results
func (s *Service) UpdateReceipt(ctx context.Context, req UpdateReceiptRequest) error {
	ctx, span := tracer.Start(ctx, "receipts.UpdateReceipt")
	defer span.End()

	query := `
		UPDATE users_receipts
		SET processed = $2, processing_error = $3, merchant_name = $4,
		    total_amount = $5, transaction_date = $6, items_count = $7,
		    merchant_address = $8, merchant_phone = $9, country_region = $10,
		    transaction_time = $11, receipt_type = $12, currency_code = $13,
		    total_tax = $14, net_amount = $15, ai_confidence = $16,
		    extraction_model_version = $17, raw_ai_response = $18,
		    detected_language = $19, content_locale = $20,
		    updated_at = NOW()
		WHERE id = $1
	`

	_, err := s.db.Exec(ctx, query,
		req.ID, req.Processed, req.ProcessingError, req.MerchantName,
		req.TotalAmount, req.TransactionDate, req.ItemsCount,
		req.MerchantAddress, req.MerchantPhone, req.CountryRegion,
		req.TransactionTime, req.ReceiptType, req.CurrencyCode,
		req.TotalTax, req.NetAmount, req.AIConfidence,
		req.ExtractionModelVersion, req.RawAIResponse,
		req.DetectedLanguage, req.ContentLocale,
	)

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to update receipt: %w", err)
	}

	return nil
}

// updateProcessingStatus is a helper to update processing status with error
func (s *Service) updateProcessingStatus(ctx context.Context, receiptID uuid.UUID, processed bool, errorMsg string, receiptData *ai.ReceiptData) {
	updateReq := UpdateReceiptRequest{
		ID:              receiptID,
		Processed:       processed,
		ProcessingError: &errorMsg,
	}

	if receiptData != nil {
		updateReq.MerchantName = &receiptData.MerchantName
		updateReq.TotalAmount = &receiptData.Total
		updateReq.ItemsCount = len(receiptData.Items)
		if !receiptData.TransactionDate.IsZero() {
			updateReq.TransactionDate = &receiptData.TransactionDate
		}
	}

	if err := s.UpdateReceipt(ctx, updateReq); err != nil {
		s.logger.Error("Failed to update receipt processing status",
			"error", err,
			"receipt_id", receiptID)
	}
}

// isValidFileType checks if the content type is supported for receipt processing
func (s *Service) isValidFileType(contentType string) bool {
	validTypes := []string{
		// Image formats (supported by Azure Document Intelligence)
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/webp",
		"image/bmp",
		"image/gif",
		// Document formats (supported by Azure Document Intelligence)
		"application/pdf",
	}

	contentType = strings.ToLower(contentType)
	for _, validType := range validTypes {
		if contentType == validType {
			return true
		}
	}
	return false
}

// getFileExtension returns the appropriate file extension for a content type
func (s *Service) getFileExtension(contentType string) string {
	switch strings.ToLower(contentType) {
	// Image formats
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/gif":
		return ".gif"
	// Document formats
	case "application/pdf":
		return ".pdf"
	default:
		return ".jpg" // default for unknown types
	}
}

// extractFileIDFromURL extracts the file ID from an Azure Blob Storage URL
func (s *Service) extractFileIDFromURL(url string) (string, error) {
	// Azure blob URLs typically look like:
	// https://accountname.blob.core.windows.net/container/path/to/file.ext
	// We need to extract "path/to/file.ext" as the file ID

	// Parse the URL
	parsedURL, err := netURL.Parse(url)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Get the path and remove the leading slash and container name
	path := strings.TrimPrefix(parsedURL.Path, "/")

	// Split by "/" and skip the first part (container name)
	pathParts := strings.Split(path, "/")
	if len(pathParts) < 2 {
		return "", fmt.Errorf("invalid Azure Blob Storage URL format: %s", url)
	}

	// Join all parts except the first (container name) to get the file ID
	fileID := strings.Join(pathParts[1:], "/")

	if fileID == "" {
		return "", fmt.Errorf("could not extract file ID from URL: %s", url)
	}

	return fileID, nil
}

// CreateReceiptItem creates a new receipt item
func (s *Service) CreateReceiptItem(ctx context.Context, req CreateReceiptItemRequest) (*ReceiptItem, error) {
	ctx, span := tracer.Start(ctx, "receipts.CreateReceiptItem")
	defer span.End()

	item := &ReceiptItem{
		ID:                   uuid.New(),
		ReceiptID:            req.ReceiptID,
		ItemOrder:            req.ItemOrder,
		OriginalDescription:  req.OriginalDescription,
		OriginalLanguage:     req.OriginalLanguage,
		LocalizedDescription: req.LocalizedDescription,
		UserLocale:           req.UserLocale,
		Quantity:             req.Quantity,
		UnitPrice:            req.UnitPrice,
		TotalPrice:           req.TotalPrice,
		CurrencyCode:         req.CurrencyCode,
		UserCategory:         req.UserCategory,
		UserNotes:            req.UserNotes,
		IsUserModified:       false,
		Confidence:           req.Confidence,
		BoundingRegions:      req.BoundingRegions,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	query := `
		INSERT INTO receipt_items (
			id, receipt_id, item_order, original_description, original_language,
			localized_description, user_locale, quantity, unit_price, total_price,
			currency_code, user_category, user_notes, is_user_modified,
			confidence, bounding_regions, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, receipt_id, item_order, original_description, original_language,
		         localized_description, user_locale, quantity, unit_price, total_price,
		         currency_code, user_category, user_notes, is_user_modified,
		         confidence, bounding_regions, created_at, updated_at
	`

	err := s.db.QueryRow(ctx, query,
		item.ID, item.ReceiptID, item.ItemOrder, item.OriginalDescription,
		item.OriginalLanguage, item.LocalizedDescription, item.UserLocale,
		item.Quantity, item.UnitPrice, item.TotalPrice, item.CurrencyCode,
		item.UserCategory, item.UserNotes, item.IsUserModified,
		item.Confidence, item.BoundingRegions, item.CreatedAt, item.UpdatedAt,
	).Scan(
		&item.ID, &item.ReceiptID, &item.ItemOrder, &item.OriginalDescription,
		&item.OriginalLanguage, &item.LocalizedDescription, &item.UserLocale,
		&item.Quantity, &item.UnitPrice, &item.TotalPrice, &item.CurrencyCode,
		&item.UserCategory, &item.UserNotes, &item.IsUserModified,
		&item.Confidence, &item.BoundingRegions, &item.CreatedAt, &item.UpdatedAt,
	)

	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to create receipt item",
			"error", err,
			"receipt_id", req.ReceiptID,
			"description", req.OriginalDescription)
		return nil, fmt.Errorf("failed to create receipt item: %w", err)
	}

	return item, nil
}

// GetReceiptItems retrieves all items for a receipt
func (s *Service) GetReceiptItems(ctx context.Context, receiptID uuid.UUID) ([]*ReceiptItem, error) {
	ctx, span := tracer.Start(ctx, "receipts.GetReceiptItems")
	defer span.End()

	query := `
		SELECT id, receipt_id, item_order, original_description, original_language,
		       localized_description, user_locale, quantity, unit_price, total_price,
		       currency_code, user_category, user_notes, is_user_modified,
		       confidence, bounding_regions, created_at, updated_at
		FROM receipt_items
		WHERE receipt_id = $1
		ORDER BY item_order
	`

	rows, err := s.db.Query(ctx, query, receiptID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get receipt items: %w", err)
	}
	defer rows.Close()

	var items []*ReceiptItem
	for rows.Next() {
		var item ReceiptItem
		err := rows.Scan(
			&item.ID, &item.ReceiptID, &item.ItemOrder, &item.OriginalDescription,
			&item.OriginalLanguage, &item.LocalizedDescription, &item.UserLocale,
			&item.Quantity, &item.UnitPrice, &item.TotalPrice, &item.CurrencyCode,
			&item.UserCategory, &item.UserNotes, &item.IsUserModified,
			&item.Confidence, &item.BoundingRegions, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan receipt item: %w", err)
		}
		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate receipt items: %w", err)
	}

	return items, nil
}

// UpdateReceiptItem updates a receipt item with user modifications
func (s *Service) UpdateReceiptItem(ctx context.Context, req UpdateReceiptItemRequest) error {
	ctx, span := tracer.Start(ctx, "receipts.UpdateReceiptItem")
	defer span.End()

	query := `
		UPDATE receipt_items
		SET localized_description = $2, user_locale = $3, user_category = $4,
		    user_notes = $5, is_user_modified = $6, updated_at = NOW()
		WHERE id = $1
	`

	_, err := s.db.Exec(ctx, query,
		req.ID, req.LocalizedDescription, req.UserLocale,
		req.UserCategory, req.UserNotes, req.IsUserModified,
	)

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to update receipt item: %w", err)
	}

	return nil
}

// FindItemTranslation looks up a translation for an item description
func (s *Service) FindItemTranslation(ctx context.Context, originalText, originalLang, targetLang string) (*ItemTranslation, error) {
	ctx, span := tracer.Start(ctx, "receipts.FindItemTranslation")
	defer span.End()

	query := `
		SELECT id, original_text, original_language, translated_text, target_language,
		       category, confidence, translation_source, usage_count, created_at, updated_at
		FROM item_translations
		WHERE original_text = $1 AND original_language = $2 AND target_language = $3
		ORDER BY usage_count DESC, confidence DESC
		LIMIT 1
	`

	var translation ItemTranslation
	err := s.db.QueryRow(ctx, query, originalText, originalLang, targetLang).Scan(
		&translation.ID, &translation.OriginalText, &translation.OriginalLanguage,
		&translation.TranslatedText, &translation.TargetLanguage, &translation.Category,
		&translation.Confidence, &translation.TranslationSource, &translation.UsageCount,
		&translation.CreatedAt, &translation.UpdatedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil // No translation found
		}
		span.RecordError(err)
		return nil, fmt.Errorf("failed to find item translation: %w", err)
	}

	// Update usage count
	_, err = s.db.Exec(ctx, "UPDATE item_translations SET usage_count = usage_count + 1 WHERE id = $1", translation.ID)
	if err != nil {
		s.logger.Warn("Failed to update translation usage count", "error", err, "id", translation.ID)
	}

	return &translation, nil
}

// CreateItemTranslation creates or updates an item translation
func (s *Service) CreateItemTranslation(ctx context.Context, originalText, originalLang, translatedText, targetLang string, category *string, confidence *float64, source string) (*ItemTranslation, error) {
	ctx, span := tracer.Start(ctx, "receipts.CreateItemTranslation")
	defer span.End()

	translation := &ItemTranslation{
		ID:                uuid.New(),
		OriginalText:      originalText,
		OriginalLanguage:  originalLang,
		TranslatedText:    translatedText,
		TargetLanguage:    targetLang,
		Category:          category,
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
		         category, confidence, translation_source, usage_count, created_at, updated_at
	`

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
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create item translation: %w", err)
	}

	return translation, nil
}

// GetItemCategories retrieves all available item categories
func (s *Service) GetItemCategories(ctx context.Context, locale string) ([]*ItemCategory, error) {
	ctx, span := tracer.Start(ctx, "receipts.GetItemCategories")
	defer span.End()

	query := `
		SELECT id, name, name_en, name_uk, name_ru, parent_category_id,
		       icon, color, sort_order, usage_count, created_at, updated_at
		FROM item_categories
		ORDER BY sort_order, usage_count DESC
	`

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get item categories: %w", err)
	}
	defer rows.Close()

	var categories []*ItemCategory
	for rows.Next() {
		var category ItemCategory
		err := rows.Scan(
			&category.ID, &category.Name, &category.NameEn, &category.NameUk,
			&category.NameRu, &category.ParentCategoryID, &category.Icon,
			&category.Color, &category.SortOrder, &category.UsageCount,
			&category.CreatedAt, &category.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan item category: %w", err)
		}
		categories = append(categories, &category)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate item categories: %w", err)
	}

	return categories, nil
}

// processReceiptItems creates receipt items from AI extracted data with localization support
func (s *Service) processReceiptItems(ctx context.Context, receiptID uuid.UUID, items []ai.ReceiptItem, detectedLang string) error {
	ctx, span := tracer.Start(ctx, "receipts.processReceiptItems")
	defer span.End()

	for i, item := range items {
		// Create receipt item with original data
		itemReq := CreateReceiptItemRequest{
			ReceiptID:           receiptID,
			ItemOrder:           i + 1,
			OriginalDescription: item.Name,
			OriginalLanguage:    &detectedLang,
			Quantity:            &item.Price, // Use price as quantity for now, could be enhanced
			UnitPrice:           &item.Price,
			TotalPrice:          item.TotalPrice,
			CurrencyCode:        nil, // Will be set from receipt level
		}

		// Set quantity properly if available
		if item.Quantity > 0 {
			qty := float64(item.Quantity)
			itemReq.Quantity = &qty
		}

		// Set confidence based on item name length and content
		confidence := s.calculateItemConfidence(item.Name)
		itemReq.Confidence = &confidence

		// Try to find existing translation for this item
		if detectedLang != "en" {
			translation, err := s.FindItemTranslation(ctx, item.Name, detectedLang, "en")
			if err == nil && translation != nil {
				itemReq.LocalizedDescription = &translation.TranslatedText
				itemReq.UserLocale = &translation.TargetLanguage
			}
		}

		// Create the receipt item
		_, err := s.CreateReceiptItem(ctx, itemReq)
		if err != nil {
			s.logger.Error("Failed to create receipt item",
				"error", err,
				"receipt_id", receiptID,
				"item_name", item.Name,
				"item_order", i+1)
			continue // Continue with other items
		}

		s.logger.Debug("Created receipt item",
			"receipt_id", receiptID,
			"item_name", item.Name,
			"total_price", item.TotalPrice,
			"item_order", i+1)
	}

	return nil
}

// calculateItemConfidence calculates confidence score for an item based on its characteristics
func (s *Service) calculateItemConfidence(itemName string) float64 {
	// Simple heuristic for confidence calculation
	confidence := 0.8 // Base confidence

	// Longer names tend to be more descriptive
	if len(itemName) > 10 {
		confidence += 0.1
	}

	// Names with numbers might be less reliable
	if strings.ContainsAny(itemName, "0123456789") {
		confidence -= 0.1
	}

	// Very short names might be incomplete
	if len(itemName) < 3 {
		confidence -= 0.2
	}

	// Ensure confidence stays within bounds
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

// storeReceiptItemsWithTranslations stores extracted receipt items with batch translation results
func (s *Service) storeReceiptItemsWithTranslations(ctx context.Context, receiptID uuid.UUID, userID uuid.UUID, items []ai.ReceiptItem, translations []ai.ReceiptItemTranslation, detectedLanguage string) error {
	ctx, span := tracer.Start(ctx, "receipts.storeReceiptItemsWithTranslations")
	defer span.End()

	if len(items) == 0 {
		return nil
	}

	s.logger.Info("Storing receipt items with batch translations",
		"receipt_id", receiptID,
		"user_id", userID,
		"items_count", len(items),
		"translations_count", len(translations),
		"detected_language", detectedLanguage)

	// Get user's locale for final validation
	userLocale, err := s.getUserLocale(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get user locale, using default",
			"error", err,
			"user_id", userID,
			"default_locale", "en")
		userLocale = "en" // fallback to English
	}

	// Create a translation map for quick lookup
	translationMap := make(map[string]ai.ReceiptItemTranslation)
	for _, translation := range translations {
		translationMap[translation.OriginalText] = translation
	}

	// Store items in order
	for i, item := range items {
		// Calculate confidence score
		confidence := s.calculateItemConfidence(item.Name)

		// Skip translation if item name is empty
		if item.Name == "" {
			s.logger.Debug("Skipping empty item name",
				"item_index", i,
				"receipt_id", receiptID)
			continue
		}

		// Get translation for this item
		var localizedDesc *string
		var finalLocale *string
		var originalLang *string

		translation, hasTranslation := translationMap[item.Name]
		if !hasTranslation {
			return fmt.Errorf("no translation found for item '%s' - all items must be translated", item.Name)
		}

		// Use the batch translation result
		localizedDesc = &translation.TranslatedText
		finalLocale = &translation.TargetLanguage
		originalLang = &translation.DetectedLanguage

		s.logger.Debug("Using batch translation for item",
			"original", item.Name,
			"translated", translation.TranslatedText,
			"confidence", translation.Confidence,
			"detected_language", translation.DetectedLanguage,
			"target_language", translation.TargetLanguage)

		// Update confidence with translation confidence
		confidence = (confidence + translation.Confidence) / 2.0

		// Convert AI item to database item
		createReq := CreateReceiptItemRequest{
			ReceiptID:            receiptID,
			ItemOrder:            i + 1, // 1-based ordering
			OriginalDescription:  item.Name,
			OriginalLanguage:     originalLang,
			LocalizedDescription: localizedDesc,
			UserLocale:           finalLocale,
			Quantity:             func() *float64 { q := float64(item.Quantity); return &q }(),
			UnitPrice:            &item.Price,
			TotalPrice:           item.TotalPrice,
			Confidence:           &confidence,
		}

		// Set category if available
		if item.Category != "" {
			createReq.UserCategory = &item.Category
		}

		// Create the item
		createdItem, err := s.CreateReceiptItem(ctx, createReq)
		if err != nil {
			s.logger.Error("Failed to create receipt item with translation",
				"error", err,
				"receipt_id", receiptID,
				"item_order", i+1,
				"item_name", item.Name)
			return fmt.Errorf("failed to create receipt item %d: %w", i+1, err)
		}

		s.logger.Debug("Receipt item created with translation",
			"item_id", createdItem.ID,
			"receipt_id", receiptID,
			"original_name", item.Name,
			"localized_name", *localizedDesc,
			"total_price", item.TotalPrice)
	}

	s.logger.Info("All receipt items stored successfully with translations",
		"receipt_id", receiptID,
		"items_count", len(items),
		"detected_language", detectedLanguage,
		"user_locale", userLocale)

	return nil
}

// storeReceiptItems stores extracted receipt items in the database with translations (legacy method for fallback)
func (s *Service) storeReceiptItems(ctx context.Context, receiptID uuid.UUID, userID uuid.UUID, items []ai.ReceiptItem, countryRegion string) error {
	ctx, span := tracer.Start(ctx, "receipts.storeReceiptItems")
	defer span.End()

	if len(items) == 0 {
		return nil
	}

	s.logger.Info("Storing receipt items",
		"receipt_id", receiptID,
		"user_id", userID,
		"items_count", len(items))

	// Get user's locale for translations
	userLocale, err := s.getUserLocale(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get user locale, using default",
			"error", err,
			"user_id", userID,
			"default_locale", "en")
		userLocale = "en" // fallback to English
	}

	// Store items in order
	for i, item := range items {
		// Calculate confidence score
		confidence := s.calculateItemConfidence(item.Name)

		// Detect original language from country region or fallback
		originalLang := s.detectLanguageFromCountry(countryRegion)
		if originalLang == "" {
			originalLang = "es" // Default fallback
		}

		// Skip translation if item name is empty
		if item.Name == "" {
			s.logger.Debug("Skipping empty item name",
				"item_index", i,
				"receipt_id", receiptID)
			// Skip this item or use a placeholder
			continue
		}

		// Translate item to user's locale
		var localizedDesc *string
		var finalLocale *string

		if userLocale != originalLang {
			translationReq := &translations.TranslationRequest{
				OriginalText:     item.Name,
				OriginalLanguage: originalLang,
				TargetLanguage:   userLocale,
				UserID:           userID,
			}

			if item.Category != "" {
				translationReq.Category = &item.Category
			}

			translationResult, err := s.translationService.TranslateItem(ctx, translationReq)
			if err != nil {
				s.logger.Warn("Failed to translate item, using original",
					"error", err,
					"item_name", item.Name,
					"target_locale", userLocale)
				localizedDesc = &item.Name
				finalLocale = &originalLang
			} else {
				localizedDesc = &translationResult.TranslatedText
				finalLocale = &userLocale

				s.logger.Debug("Item translated",
					"original", item.Name,
					"translated", translationResult.TranslatedText,
					"source", translationResult.Source,
					"confidence", translationResult.Confidence)
			}
		} else {
			// Same language, no translation needed
			localizedDesc = &item.Name
			finalLocale = &userLocale
		}

		// Convert AI item to database item
		createReq := CreateReceiptItemRequest{
			ReceiptID:            receiptID,
			ItemOrder:            i + 1, // 1-based ordering
			OriginalDescription:  item.Name,
			OriginalLanguage:     &originalLang,
			LocalizedDescription: localizedDesc,
			UserLocale:           finalLocale,
			Quantity:             func() *float64 { q := float64(item.Quantity); return &q }(),
			UnitPrice:            &item.Price,
			TotalPrice:           item.TotalPrice,
			Confidence:           &confidence,
		}

		// Set currency if available
		if item.Category != "" {
			createReq.UserCategory = &item.Category
		}

		// Create the item
		createdItem, err := s.CreateReceiptItem(ctx, createReq)
		if err != nil {
			s.logger.Error("Failed to create receipt item",
				"error", err,
				"receipt_id", receiptID,
				"item_order", i+1,
				"item_name", item.Name)
			return fmt.Errorf("failed to create receipt item %d: %w", i+1, err)
		}

		s.logger.Debug("Receipt item created",
			"item_id", createdItem.ID,
			"receipt_id", receiptID,
			"item_name", item.Name,
			"total_price", item.TotalPrice)
	}

	s.logger.Info("All receipt items stored successfully",
		"receipt_id", receiptID,
		"items_count", len(items))

	return nil
}

// getUserLocale retrieves the user's locale preference from the database
func (s *Service) getUserLocale(ctx context.Context, userID uuid.UUID) (string, error) {
	var locale string
	query := `SELECT locale FROM users WHERE id = $1`

	err := s.db.QueryRow(ctx, query, userID).Scan(&locale)
	if err != nil {
		return "", fmt.Errorf("failed to get user locale: %w", err)
	}

	// Normalize locale using translation service
	normalizedLocale := s.translationService.GetSupportedLanguages()
	for _, supportedLang := range normalizedLocale {
		if strings.ToLower(locale) == supportedLang {
			return supportedLang, nil
		}
	}

	// Return as-is if not in supported list, translation service will handle normalization
	return locale, nil
}

// detectLanguageFromCountry attempts to detect language from country region code
func (s *Service) detectLanguageFromCountry(countryRegion string) string {
	if countryRegion == "" {
		return ""
	}

	// Map common country codes to languages
	countryToLanguage := map[string]string{
		"ESP": "es", // Spain -> Spanish
		"MEX": "es", // Mexico -> Spanish
		"ARG": "es", // Argentina -> Spanish
		"COL": "es", // Colombia -> Spanish
		"CHL": "es", // Chile -> Spanish
		"USA": "en", // United States -> English
		"GBR": "en", // Great Britain -> English
		"CAN": "en", // Canada -> English
		"AUS": "en", // Australia -> English
		"RUS": "ru", // Russia -> Russian
		"UKR": "uk", // Ukraine -> Ukrainian
		"FRA": "fr", // France -> French
		"DEU": "de", // Germany -> German
		"ITA": "it", // Italy -> Italian
	}

	if lang, exists := countryToLanguage[strings.ToUpper(countryRegion)]; exists {
		return lang
	}

	// Try with first 2 characters (common ISO codes)
	if len(countryRegion) >= 2 {
		prefix := strings.ToUpper(countryRegion[:2])
		switch prefix {
		case "ES":
			return "es"
		case "EN", "US", "GB", "AU", "CA":
			return "en"
		case "RU":
			return "ru"
		case "UA", "UK":
			return "uk"
		case "FR":
			return "fr"
		case "DE":
			return "de"
		case "IT":
			return "it"
		}
	}

	return "" // Unknown country
}

// generateContentLocale creates a content locale from detected language and country
func (s *Service) generateContentLocale(language, country string) string {
	if language == "" {
		return ""
	}

	// If we have country information, combine it
	if country != "" {
		return language + "-" + strings.ToUpper(country)
	}

	// Default country mappings for common languages
	switch language {
	case "es":
		return "es-ES"
	case "en":
		return "en-US"
	case "fr":
		return "fr-FR"
	case "de":
		return "de-DE"
	default:
		return language
	}
}

// GetUserReceiptsCount returns the total number of receipts for a user
func (s *Service) GetUserReceiptsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	ctx, span := tracer.Start(ctx, "receipts.GetUserReceiptsCount")
	defer span.End()

	var count int
	query := `SELECT COUNT(*) FROM users_receipts WHERE user_id = $1`

	err := s.db.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to count user receipts", "error", err, "user_id", userID)
		return 0, fmt.Errorf("failed to count user receipts: %w", err)
	}

	return count, nil
}

// GetReceiptWithItems retrieves a complete receipt with all its items
func (s *Service) GetReceiptWithItems(ctx context.Context, receiptID uuid.UUID, userID uuid.UUID) (*ReceiptWithItems, error) {
	ctx, span := tracer.Start(ctx, "receipts.GetReceiptWithItems")
	defer span.End()

	// First get the receipt
	var receipt Receipt
	query := `
		SELECT id, user_id, file_url, file_name, file_size, content_type,
		       telegram_file_id, processed, processing_error, merchant_name,
		       total_amount, transaction_date, items_count, created_at, updated_at,
		       merchant_address, merchant_phone, country_region, transaction_time,
		       receipt_type, currency_code, total_tax, net_amount, ai_confidence,
		       extraction_model_version, raw_ai_response, detected_language, content_locale,
		       auto_translation_enabled, last_translation_update
		FROM users_receipts
		WHERE id = $1 AND user_id = $2`

	err := s.db.QueryRow(ctx, query, receiptID, userID).Scan(
		&receipt.ID, &receipt.UserID, &receipt.FileURL, &receipt.FileName,
		&receipt.FileSize, &receipt.ContentType, &receipt.TelegramFileID,
		&receipt.Processed, &receipt.ProcessingError, &receipt.MerchantName,
		&receipt.TotalAmount, &receipt.TransactionDate, &receipt.ItemsCount,
		&receipt.CreatedAt, &receipt.UpdatedAt, &receipt.MerchantAddress,
		&receipt.MerchantPhone, &receipt.CountryRegion, &receipt.TransactionTime,
		&receipt.ReceiptType, &receipt.CurrencyCode, &receipt.TotalTax,
		&receipt.NetAmount, &receipt.AIConfidence, &receipt.ExtractionModelVersion,
		&receipt.RawAIResponse, &receipt.DetectedLanguage, &receipt.ContentLocale,
		&receipt.AutoTranslationEnabled, &receipt.LastTranslationUpdate)

	if err != nil {
		span.RecordError(err)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("receipt not found")
		}
		s.logger.Error("Failed to get receipt", "error", err, "receipt_id", receiptID, "user_id", userID)
		return nil, fmt.Errorf("failed to get receipt: %w", err)
	}

	// Get the receipt items
	itemsQuery := `
		SELECT id, receipt_id, item_order, original_description, original_language,
		       localized_description, user_locale, quantity, unit_price, total_price,
		       currency_code, user_category, user_notes, is_user_modified, confidence,
		       bounding_regions, created_at, updated_at
		FROM receipt_items
		WHERE receipt_id = $1
		ORDER BY item_order ASC, created_at ASC`

	rows, err := s.db.Query(ctx, itemsQuery, receiptID)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Failed to get receipt items", "error", err, "receipt_id", receiptID)
		return nil, fmt.Errorf("failed to get receipt items: %w", err)
	}
	defer rows.Close()

	var items []ReceiptItem
	for rows.Next() {
		var item ReceiptItem
		err := rows.Scan(
			&item.ID, &item.ReceiptID, &item.ItemOrder, &item.OriginalDescription,
			&item.OriginalLanguage, &item.LocalizedDescription, &item.UserLocale,
			&item.Quantity, &item.UnitPrice, &item.TotalPrice, &item.CurrencyCode,
			&item.UserCategory, &item.UserNotes, &item.IsUserModified, &item.Confidence,
			&item.BoundingRegions, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			span.RecordError(err)
			s.logger.Error("Failed to scan receipt item", "error", err, "receipt_id", receiptID)
			continue
		}
		items = append(items, item)
	}

	return &ReceiptWithItems{
		Receipt: receipt,
		Items:   items,
	}, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
