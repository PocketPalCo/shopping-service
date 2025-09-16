package receipts

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Receipt represents a user's uploaded receipt
type Receipt struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	FileURL         string     `json:"file_url" db:"file_url"`
	FileName        string     `json:"file_name" db:"file_name"`
	FileSize        int64      `json:"file_size" db:"file_size"`
	ContentType     string     `json:"content_type" db:"content_type"`
	TelegramFileID  *string    `json:"telegram_file_id" db:"telegram_file_id"`
	Processed       bool       `json:"processed" db:"processed"`
	ProcessingError *string    `json:"processing_error" db:"processing_error"`
	MerchantName    *string    `json:"merchant_name" db:"merchant_name"`
	TotalAmount     *float64   `json:"total_amount" db:"total_amount"`
	TransactionDate *time.Time `json:"transaction_date" db:"transaction_date"`
	ItemsCount      int        `json:"items_count" db:"items_count"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`

	// Enhanced fields from AI analysis
	MerchantAddress        *string          `json:"merchant_address" db:"merchant_address"`
	MerchantPhone          *string          `json:"merchant_phone" db:"merchant_phone"`
	CountryRegion          *string          `json:"country_region" db:"country_region"`
	TransactionTime        *time.Time       `json:"transaction_time" db:"transaction_time"`
	ReceiptType            *string          `json:"receipt_type" db:"receipt_type"`
	CurrencyCode           *string          `json:"currency_code" db:"currency_code"`
	TotalTax               *float64         `json:"total_tax" db:"total_tax"`
	NetAmount              *float64         `json:"net_amount" db:"net_amount"`
	AIConfidence           *float64         `json:"ai_confidence" db:"ai_confidence"`
	ExtractionModelVersion *string          `json:"extraction_model_version" db:"extraction_model_version"`
	RawAIResponse          *json.RawMessage `json:"raw_ai_response" db:"raw_ai_response"`
	DetectedLanguage       *string          `json:"detected_language" db:"detected_language"`
	ContentLocale          *string          `json:"content_locale" db:"content_locale"`
	AutoTranslationEnabled *bool            `json:"auto_translation_enabled" db:"auto_translation_enabled"`
	LastTranslationUpdate  *time.Time       `json:"last_translation_update" db:"last_translation_update"`
}

// ReceiptItem represents an individual item from a receipt
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

// ItemTranslation represents a translation dictionary entry
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

// ItemCategory represents product categories for items
type ItemCategory struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	Name             string     `json:"name" db:"name"`
	NameEn           *string    `json:"name_en" db:"name_en"`
	NameUk           *string    `json:"name_uk" db:"name_uk"`
	NameRu           *string    `json:"name_ru" db:"name_ru"`
	ParentCategoryID *uuid.UUID `json:"parent_category_id" db:"parent_category_id"`
	Icon             *string    `json:"icon" db:"icon"`
	Color            *string    `json:"color" db:"color"`
	SortOrder        int        `json:"sort_order" db:"sort_order"`
	UsageCount       int        `json:"usage_count" db:"usage_count"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

// CreateReceiptRequest represents the data needed to create a new receipt
type CreateReceiptRequest struct {
	UserID         uuid.UUID
	FileName       string
	FileSize       int64
	ContentType    string
	TelegramFileID *string
	FileData       []byte
}

// UpdateReceiptRequest represents data for updating receipt processing results
type UpdateReceiptRequest struct {
	ID                     uuid.UUID
	Processed              bool
	ProcessingError        *string
	MerchantName           *string
	TotalAmount            *float64
	TransactionDate        *time.Time
	ItemsCount             int
	MerchantAddress        *string
	MerchantPhone          *string
	CountryRegion          *string
	TransactionTime        *time.Time
	ReceiptType            *string
	CurrencyCode           *string
	TotalTax               *float64
	NetAmount              *float64
	AIConfidence           *float64
	ExtractionModelVersion *string
	RawAIResponse          *json.RawMessage
	DetectedLanguage       *string
	ContentLocale          *string
}

// CreateReceiptItemRequest represents data for creating a receipt item
type CreateReceiptItemRequest struct {
	ReceiptID            uuid.UUID
	ItemOrder            int
	OriginalDescription  string
	OriginalLanguage     *string
	LocalizedDescription *string
	UserLocale           *string
	Quantity             *float64
	UnitPrice            *float64
	TotalPrice           float64
	CurrencyCode         *string
	UserCategory         *string
	UserNotes            *string
	Confidence           *float64
	BoundingRegions      *json.RawMessage
}

// UpdateReceiptItemRequest represents data for updating a receipt item
type UpdateReceiptItemRequest struct {
	ID                   uuid.UUID
	LocalizedDescription *string
	UserLocale           *string
	UserCategory         *string
	UserNotes            *string
	IsUserModified       bool
}

// ReceiptWithItems represents a receipt with all its items
type ReceiptWithItems struct {
	Receipt Receipt       `json:"receipt"`
	Items   []ReceiptItem `json:"items"`
}
