package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DocumentIntelligenceService handles Azure Document Intelligence API operations
type DocumentIntelligenceService struct {
	endpoint   string
	apiKey     string
	apiVersion string
	model      string
	httpClient *http.Client

	// Metrics
	analyzeRequestsTotal     metric.Int64Counter
	analyzeRequestDuration   metric.Float64Histogram
	analyzeRequestErrors     metric.Int64Counter
	apiRequestsTotal         metric.Int64Counter
	apiRequestDuration       metric.Float64Histogram
	apiRequestErrors         metric.Int64Counter
	confidenceScoreHistogram metric.Float64Histogram
}

// ReceiptItem represents an item extracted from a receipt
type ReceiptItem struct {
	Name        string  `json:"name"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
	TotalPrice  float64 `json:"totalPrice"`
	Category    string  `json:"category,omitempty"`
	Description string  `json:"description,omitempty"`
}

// ReceiptData represents the parsed receipt data
type ReceiptData struct {
	MerchantName    string        `json:"merchantName"`
	MerchantAddress string        `json:"merchantAddress"`
	MerchantPhone   string        `json:"merchantPhone"`
	TransactionDate time.Time     `json:"transactionDate"`
	TransactionTime time.Time     `json:"transactionTime"`
	Items           []ReceiptItem `json:"items"`
	Subtotal        float64       `json:"subtotal"`
	Tax             float64       `json:"tax"`
	Total           float64       `json:"total"`
	Currency        string        `json:"currency"`
	ReceiptType     string        `json:"receiptType"`
	CountryRegion   string        `json:"countryRegion"`

	// AI metadata
	Confidence       float64 `json:"confidence"`
	APIVersion       string  `json:"apiVersion"`
	ModelID          string  `json:"modelId"`
	DetectedLanguage string  `json:"detectedLanguage"`
	ContentLocale    string  `json:"contentLocale"`

	// Translation data for receipt items
	ItemTranslations []ReceiptItemTranslation `json:"itemTranslations,omitempty"`
}

// AnalyzeDocumentRequest represents the request payload for document analysis
type analyzeDocumentRequest struct {
	Base64Source string `json:"base64Source"`
}

// Azure Document Intelligence API response structures
type documentIntelligenceResponse struct {
	Status              string                     `json:"status"`
	CreatedDateTime     string                     `json:"createdDateTime"`
	LastUpdatedDateTime string                     `json:"lastUpdatedDateTime"`
	AnalyzeResult       *analyzeResult             `json:"analyzeResult,omitempty"`
	Error               *documentIntelligenceError `json:"error,omitempty"`
}

type documentIntelligenceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type analyzeResult struct {
	ApiVersion      string     `json:"apiVersion"`
	ModelId         string     `json:"modelId"`
	StringIndexType string     `json:"stringIndexType"`
	Content         string     `json:"content"`
	Documents       []document `json:"documents"`
}

type document struct {
	DocType         string                   `json:"docType"`
	BoundingRegions []boundingRegion         `json:"boundingRegions"`
	Fields          map[string]documentField `json:"fields"`
	Confidence      float64                  `json:"confidence"`
}

type boundingRegion struct {
	PageNumber int       `json:"pageNumber"`
	Polygon    []float64 `json:"polygon"`
}

type documentField struct {
	Type            string                   `json:"type"`
	ValueString     *string                  `json:"valueString,omitempty"`
	ValueNumber     *float64                 `json:"valueNumber,omitempty"`
	ValueArray      []documentField          `json:"valueArray,omitempty"`
	ValueObject     map[string]documentField `json:"valueObject,omitempty"`
	ValueCurrency   *currencyValue           `json:"valueCurrency,omitempty"`
	Content         string                   `json:"content"`
	BoundingRegions []boundingRegion         `json:"boundingRegions"`
	Confidence      float64                  `json:"confidence"`
}

type currencyValue struct {
	Amount       float64 `json:"amount"`
	CurrencyCode string  `json:"currencyCode"`
}

// NewDocumentIntelligenceService creates a new Document Intelligence service client
func NewDocumentIntelligenceService(endpoint, apiKey, apiVersion, model string) *DocumentIntelligenceService {
	meter := otel.Meter("azure_document_intelligence")

	// Initialize metrics
	analyzeRequestsTotal, _ := meter.Int64Counter(
		"azure_document_intelligence_analyze_requests_total",
		metric.WithDescription("Total number of document analysis requests"),
		metric.WithUnit("1"),
	)

	analyzeRequestDuration, _ := meter.Float64Histogram(
		"azure_document_intelligence_analyze_duration_seconds",
		metric.WithDescription("Duration of document analysis requests"),
		metric.WithUnit("s"),
	)

	analyzeRequestErrors, _ := meter.Int64Counter(
		"azure_document_intelligence_analyze_errors_total",
		metric.WithDescription("Total number of document analysis errors"),
		metric.WithUnit("1"),
	)

	apiRequestsTotal, _ := meter.Int64Counter(
		"azure_document_intelligence_api_requests_total",
		metric.WithDescription("Total number of API requests to Azure Document Intelligence"),
		metric.WithUnit("1"),
	)

	apiRequestDuration, _ := meter.Float64Histogram(
		"azure_document_intelligence_api_duration_seconds",
		metric.WithDescription("Duration of API requests to Azure Document Intelligence"),
		metric.WithUnit("s"),
	)

	apiRequestErrors, _ := meter.Int64Counter(
		"azure_document_intelligence_api_errors_total",
		metric.WithDescription("Total number of API errors from Azure Document Intelligence"),
		metric.WithUnit("1"),
	)

	confidenceScoreHistogram, _ := meter.Float64Histogram(
		"azure_document_intelligence_confidence_score",
		metric.WithDescription("Confidence scores from Azure Document Intelligence"),
		metric.WithUnit("1"),
	)

	return &DocumentIntelligenceService{
		endpoint:   endpoint,
		apiKey:     apiKey,
		apiVersion: apiVersion,
		model:      model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},

		// Metrics
		analyzeRequestsTotal:     analyzeRequestsTotal,
		analyzeRequestDuration:   analyzeRequestDuration,
		analyzeRequestErrors:     analyzeRequestErrors,
		apiRequestsTotal:         apiRequestsTotal,
		apiRequestDuration:       apiRequestDuration,
		apiRequestErrors:         apiRequestErrors,
		confidenceScoreHistogram: confidenceScoreHistogram,
	}
}

func (dis *DocumentIntelligenceService) AnalyzeReceipt(ctx context.Context, imageData []byte, contentType string) (*ReceiptData, error) {
	startTime := time.Now()

	// Record analyze request metrics
	attrs := []attribute.KeyValue{
		attribute.String("model", dis.model),
		attribute.String("content_type", contentType),
		attribute.Int("image_size_bytes", len(imageData)),
		attribute.String("operation", "analyze_receipt"),
	}
	dis.analyzeRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	operationLocation, err := dis.startAnalysis(ctx, imageData, contentType)
	if err != nil {
		// Record analyze error
		errorAttrs := append(attrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.String("error_stage", "start_analysis"),
		)
		dis.analyzeRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
		dis.analyzeRequestDuration.Record(ctx, time.Since(startTime).Seconds(), metric.WithAttributes(errorAttrs...))

		return nil, fmt.Errorf("failed to start analysis: %w", err)
	}

	result, err := dis.pollForResults(ctx, operationLocation)
	if err != nil {
		// Record polling error
		errorAttrs := append(attrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.String("error_stage", "poll_results"),
		)
		dis.analyzeRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
		dis.analyzeRequestDuration.Record(ctx, time.Since(startTime).Seconds(), metric.WithAttributes(errorAttrs...))

		return nil, fmt.Errorf("failed to get analysis results: %w", err)
	}

	receiptData, err := dis.parseReceiptResponse(result)
	if err != nil {
		// Record parsing error
		errorAttrs := append(attrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.String("error_stage", "parse_response"),
		)
		dis.analyzeRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
		dis.analyzeRequestDuration.Record(ctx, time.Since(startTime).Seconds(), metric.WithAttributes(errorAttrs...))

		return nil, fmt.Errorf("failed to parse receipt data: %w", err)
	}

	// Record success metrics
	duration := time.Since(startTime)
	successAttrs := append(attrs,
		attribute.String("outcome", "success"),
		attribute.Float64("confidence", receiptData.Confidence),
		attribute.Int("items_extracted", len(receiptData.Items)),
		attribute.String("merchant_name", receiptData.MerchantName),
		attribute.String("currency", receiptData.Currency),
	)
	dis.analyzeRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(successAttrs...))
	dis.confidenceScoreHistogram.Record(ctx, receiptData.Confidence, metric.WithAttributes(successAttrs...))

	return receiptData, nil
}

func (dis *DocumentIntelligenceService) startAnalysis(ctx context.Context, imageData []byte, contentType string) (string, error) {
	url := fmt.Sprintf("%s/documentintelligence/documentModels/%s:analyze?api-version=%s",
		dis.endpoint, dis.model, dis.apiVersion)

	body := bytes.NewReader(imageData)

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Ocp-Apim-Subscription-Key", dis.apiKey)

	// Record API request metrics
	apiStartTime := time.Now()
	apiAttrs := []attribute.KeyValue{
		attribute.String("endpoint", "analyze"),
		attribute.String("method", "POST"),
		attribute.String("model", dis.model),
		attribute.String("api_version", dis.apiVersion),
	}
	dis.apiRequestsTotal.Add(ctx, 1, metric.WithAttributes(apiAttrs...))

	resp, err := dis.httpClient.Do(req)
	apiDuration := time.Since(apiStartTime)
	dis.apiRequestDuration.Record(ctx, apiDuration.Seconds(), metric.WithAttributes(apiAttrs...))

	if err != nil {
		// Record API error
		errorAttrs := append(apiAttrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.String("error_category", "http_request"),
		)
		dis.apiRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)

		// Record API error for non-202 status
		errorAttrs := append(apiAttrs,
			attribute.String("error_type", "api_error"),
			attribute.String("error_category", "status_error"),
			attribute.Int("status_code", resp.StatusCode),
		)
		dis.apiRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		return "", fmt.Errorf("analysis request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	operationLocation := resp.Header.Get("Operation-Location")
	if operationLocation == "" {
		return "", fmt.Errorf("operation-location header not found in response")
	}

	return operationLocation, nil
}

func (dis *DocumentIntelligenceService) pollForResults(ctx context.Context, operationLocation string) (*documentIntelligenceResponse, error) {
	maxAttempts := 30
	pollInterval := 5 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", operationLocation, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create status request: %w", err)
		}

		req.Header.Set("Ocp-Apim-Subscription-Key", dis.apiKey)

		resp, err := dis.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to check status: %w", err)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var result documentIntelligenceResponse
		if err := json.Unmarshal(bodyBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if result.Error != nil {
			return nil, fmt.Errorf("document intelligence error: %s - %s", result.Error.Code, result.Error.Message)
		}

		switch result.Status {
		case "succeeded":
			return &result, nil
		case "failed":
			return nil, fmt.Errorf("document analysis failed")
		case "running", "notStarted":
		default:
			return nil, fmt.Errorf("unexpected status: %s", result.Status)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return nil, fmt.Errorf("polling timeout exceeded")
}

func (dis *DocumentIntelligenceService) parseReceiptResponse(response *documentIntelligenceResponse) (*ReceiptData, error) {
	if response.AnalyzeResult == nil || len(response.AnalyzeResult.Documents) == 0 {
		return nil, fmt.Errorf("no documents found in response")
	}

	doc := response.AnalyzeResult.Documents[0]
	receiptData := &ReceiptData{
		Items:      make([]ReceiptItem, 0),
		Confidence: doc.Confidence,
		APIVersion: response.AnalyzeResult.ApiVersion,
		ModelID:    response.AnalyzeResult.ModelId,
	}

	if field, ok := doc.Fields["MerchantName"]; ok && field.ValueString != nil {
		receiptData.MerchantName = *field.ValueString
	}

	if field, ok := doc.Fields["MerchantAddress"]; ok && field.Content != "" {
		receiptData.MerchantAddress = field.Content
	}

	if field, ok := doc.Fields["MerchantPhoneNumber"]; ok && field.Content != "" {
		receiptData.MerchantPhone = field.Content
	}

	if field, ok := doc.Fields["TransactionDate"]; ok && field.Content != "" {
		// Comprehensive date format support for international receipts
		formats := []string{
			// European formats (DD.MM.YYYY and DD.MM.YY)
			"02.01.2006", // DD.MM.YYYY (German, Spanish, etc.)
			"02.01.06",   // DD.MM.YY (2-digit year)
			"2.1.2006",   // D.M.YYYY (single digits)
			"2.1.06",     // D.M.YY
			"02.1.2006",  // DD.M.YYYY (mixed)
			"2.01.2006",  // D.MM.YYYY (mixed)

			// European slash formats (DD/MM/YYYY and DD/MM/YY)
			"02/01/2006", // DD/MM/YYYY (UK, EU)
			"02/01/06",   // DD/MM/YY (2-digit year)
			"2/1/2006",   // D/M/YYYY (single digits)
			"2/1/06",     // D/M/YY
			"02/1/2006",  // DD/M/YYYY (mixed)
			"2/01/2006",  // D/MM/YYYY (mixed)

			// US formats (MM/DD/YYYY and MM/DD/YY)
			"01/02/2006", // MM/DD/YYYY (US standard)
			"01/02/06",   // MM/DD/YY (US 2-digit year)
			"1/2/2006",   // M/D/YYYY (single digits)
			"1/2/06",     // M/D/YY
			"01/2/2006",  // MM/D/YYYY (mixed)
			"1/02/2006",  // M/DD/YYYY (mixed)

			// ISO and dash formats
			"2006-01-02", // YYYY-MM-DD (ISO standard)
			"06-01-02",   // YY-MM-DD (2-digit year)
			"2006-1-2",   // YYYY-M-D (single digits)
			"06-1-2",     // YY-M-D
			"02-01-2006", // DD-MM-YYYY (European dash)
			"02-01-06",   // DD-MM-YY
			"2-1-2006",   // D-M-YYYY
			"2-1-06",     // D-M-YY

			// Space-separated formats
			"02 01 2006", // DD MM YYYY
			"02 01 06",   // DD MM YY
			"2 1 2006",   // D M YYYY
			"2 1 06",     // D M YY
			"2006 01 02", // YYYY MM DD
			"06 01 02",   // YY MM DD

			// Alternative separators
			"02_01_2006", // DD_MM_YYYY (underscore)
			"02_01_06",   // DD_MM_YY
			"2006_01_02", // YYYY_MM_DD
			"06_01_02",   // YY_MM_DD

			// Month names (common in some receipts)
			"02 Jan 2006", // DD Mon YYYY
			"02 Jan 06",   // DD Mon YY
			"2 Jan 2006",  // D Mon YYYY
			"2 Jan 06",    // D Mon YY
			"Jan 02 2006", // Mon DD YYYY
			"Jan 02 06",   // Mon DD YY
			"Jan 2 2006",  // Mon D YYYY
			"Jan 2 06",    // Mon D YY

			// Full month names
			"02 January 2006", // DD Month YYYY
			"02 January 06",   // DD Month YY
			"2 January 2006",  // D Month YYYY
			"2 January 06",    // D Month YY
			"January 02 2006", // Month DD YYYY
			"January 02 06",   // Month DD YY
			"January 2 2006",  // Month D YYYY
			"January 2 06",    // Month D YY

			// ISO datetime formats (with time)
			"2006-01-02T15:04:05Z", // ISO with Z
			"2006-01-02T15:04:05",  // ISO without Z
			"2006-01-02 15:04:05",  // ISO with space
			"02.01.2006 15:04:05",  // European with time
			"02/01/2006 15:04:05",  // European slash with time
			"01/02/2006 15:04:05",  // US with time

			// Special cases found in receipts
			"020106",   // DDMMYY (compact)
			"02012006", // DDMMYYYY (compact)
			"20060102", // YYYYMMDD (compact)
			"060102",   // YYMMDD (compact)
		}

		for _, format := range formats {
			if date, err := time.Parse(format, field.Content); err == nil {
				receiptData.TransactionDate = date
				break
			}
		}
	}

	if field, ok := doc.Fields["TransactionTime"]; ok && field.Content != "" {
		// Comprehensive time format support
		formats := []string{
			// 24-hour format
			"15:04:05", // HH:MM:SS (full)
			"15:04",    // HH:MM (most common)
			"15.04.05", // HH.MM.SS (European dots)
			"15.04",    // HH.MM (European dots)
			"15-04-05", // HH-MM-SS (dash)
			"15-04",    // HH-MM (dash)
			"15 04 05", // HH MM SS (spaces)
			"15 04",    // HH MM (spaces)
			"1504",     // HHMM (compact)
			"150405",   // HHMMSS (compact)

			// 12-hour format with AM/PM
			"3:04:05 PM",  // H:MM:SS PM (with seconds)
			"3:04 PM",     // H:MM PM (most common)
			"03:04:05 PM", // HH:MM:SS PM (padded)
			"03:04 PM",    // HH:MM PM (padded)
			"3:04:05PM",   // H:MM:SSPM (no space)
			"3:04PM",      // H:MM PM (no space)
			"03:04:05PM",  // HH:MM:SSPM (no space, padded)
			"03:04PM",     // HH:MMPM (no space, padded)

			// Alternative AM/PM formats
			"3:04:05 am", // Lowercase am/pm
			"3:04 am",
			"3:04:05am",
			"3:04am",
			"3:04:05 p.m.", // With periods
			"3:04 p.m.",
			"3:04:05 a.m.",
			"3:04 a.m.",

			// Single digit hours
			"3:04:05", // H:MM:SS (no AM/PM, single digit)
			"3:04",    // H:MM (no AM/PM, single digit)
			"3.04.05", // H.MM.SS (European)
			"3.04",    // H.MM (European)

			// Alternative separators
			"15_04_05", // HH_MM_SS (underscore)
			"15_04",    // HH_MM (underscore)
			"3_04_05",  // H_MM_SS (underscore)
			"3_04",     // H_MM (underscore)

			// With milliseconds
			"15:04:05.000",   // HH:MM:SS.mmm
			"3:04:05.000 PM", // H:MM:SS.mmm PM
		}

		for _, format := range formats {
			if timeVal, err := time.Parse(format, field.Content); err == nil {
				receiptData.TransactionTime = timeVal
				break
			}
		}
	}

	if field, ok := doc.Fields["Subtotal"]; ok {
		if field.ValueNumber != nil {
			receiptData.Subtotal = *field.ValueNumber
		} else if field.ValueCurrency != nil {
			receiptData.Subtotal = field.ValueCurrency.Amount
		}
	}

	// Handle TotalTax field (direct tax field)
	if field, ok := doc.Fields["TotalTax"]; ok {
		if field.ValueNumber != nil {
			receiptData.Tax = *field.ValueNumber
		} else if field.ValueCurrency != nil {
			receiptData.Tax = field.ValueCurrency.Amount
		}
	} else if taxDetails, ok := doc.Fields["TaxDetails"]; ok && taxDetails.ValueArray != nil {
		// Handle TaxDetails array (sum all tax amounts)
		var totalTax float64
		for _, taxDetail := range taxDetails.ValueArray {
			if taxDetail.ValueObject != nil {
				if amountField, exists := taxDetail.ValueObject["Amount"]; exists {
					if amountField.ValueCurrency != nil {
						totalTax += amountField.ValueCurrency.Amount
					} else if amountField.ValueNumber != nil {
						totalTax += *amountField.ValueNumber
					}
				}
			}
		}
		receiptData.Tax = totalTax
	}

	if field, ok := doc.Fields["Total"]; ok {
		if field.ValueNumber != nil {
			receiptData.Total = *field.ValueNumber
		} else if field.ValueCurrency != nil {
			receiptData.Total = field.ValueCurrency.Amount
		}
	}

	if field, ok := doc.Fields["Total"]; ok && field.ValueCurrency != nil {
		receiptData.Currency = field.ValueCurrency.CurrencyCode
	}

	if field, ok := doc.Fields["CountryRegion"]; ok && field.Content != "" {
		receiptData.CountryRegion = field.Content
	}

	if field, ok := doc.Fields["ReceiptType"]; ok && field.ValueString != nil {
		receiptData.ReceiptType = *field.ValueString
	}

	if field, ok := doc.Fields["Items"]; ok && field.ValueArray != nil {
		for _, itemField := range field.ValueArray {
			if itemField.ValueObject == nil {
				continue
			}

			item := ReceiptItem{}

			if descField, ok := itemField.ValueObject["Description"]; ok && descField.ValueString != nil {
				item.Name = *descField.ValueString
			}

			if qtyField, ok := itemField.ValueObject["Quantity"]; ok && qtyField.ValueNumber != nil {
				item.Quantity = int(*qtyField.ValueNumber)
			}

			if priceField, ok := itemField.ValueObject["Price"]; ok {
				if priceField.ValueCurrency != nil {
					item.Price = priceField.ValueCurrency.Amount
				} else if priceField.ValueNumber != nil {
					item.Price = *priceField.ValueNumber
				}
			}

			if totalField, ok := itemField.ValueObject["TotalPrice"]; ok {
				if totalField.ValueCurrency != nil {
					item.TotalPrice = totalField.ValueCurrency.Amount
				} else if totalField.ValueNumber != nil {
					item.TotalPrice = *totalField.ValueNumber
				}
			}

			if item.Price == 0 && item.TotalPrice > 0 && item.Quantity > 0 {
				item.Price = item.TotalPrice / float64(item.Quantity)
			}

			if catField, ok := itemField.ValueObject["Category"]; ok && catField.ValueString != nil {
				item.Category = *catField.ValueString
			}

			receiptData.Items = append(receiptData.Items, item)
		}
	}

	// Calculate subtotal if not provided (Total - Tax)
	if receiptData.Subtotal == 0 && receiptData.Total > 0 {
		receiptData.Subtotal = receiptData.Total - receiptData.Tax
	}

	return receiptData, nil
}

func (dis *DocumentIntelligenceService) ValidateConfiguration() error {
	if dis.endpoint == "" {
		return fmt.Errorf("document intelligence endpoint is required")
	}
	if dis.apiKey == "" {
		return fmt.Errorf("document intelligence API key is required")
	}
	if dis.apiVersion == "" {
		return fmt.Errorf("document intelligence API version is required")
	}
	if dis.model == "" {
		return fmt.Errorf("document intelligence model is required")
	}
	return nil
}
