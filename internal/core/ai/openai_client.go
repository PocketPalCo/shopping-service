package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/products"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type ProductService interface {
	GetAllProducts(ctx context.Context) ([]*products.Product, error)
}

type openAIClient struct {
	config         config.OpenAIConfig
	httpClient     *http.Client
	logger         *slog.Logger
	promptBuilder  *PromptBuilder
	productService ProductService

	// Metrics
	parseRequestsTotal       metric.Int64Counter
	parseRequestDuration     metric.Float64Histogram
	parseRequestErrors       metric.Int64Counter
	apiRequestsTotal         metric.Int64Counter
	apiRequestDuration       metric.Float64Histogram
	apiRequestErrors         metric.Int64Counter
	tokensUsed               metric.Int64Counter
	confidenceScoreHistogram metric.Float64Histogram
}

// Chat Completions API structures (legacy)
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	Store       *bool     `json:"store,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Responses API structures (new)
type ResponsesRequest struct {
	Model     string              `json:"model"`
	Input     []ResponsesMessage  `json:"input"`
	Store     *bool               `json:"store,omitempty"`
	Reasoning *ResponsesReasoning `json:"reasoning,omitempty"`
}

type ResponsesMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponsesReasoning struct {
	Effort string `json:"effort"`
}

type ResponsesResponse struct {
	ID        string                `json:"id"`
	Object    string                `json:"object"`
	CreatedAt int64                 `json:"created_at"`
	Model     string                `json:"model"`
	Output    []ResponsesOutputItem `json:"output"`
	Usage     Usage                 `json:"usage"`
}

type ResponsesOutputItem struct {
	ID      string                   `json:"id"`
	Type    string                   `json:"type"`
	Status  string                   `json:"status,omitempty"`
	Role    string                   `json:"role,omitempty"`
	Content []ResponsesOutputContent `json:"content,omitempty"`
}

type ResponsesOutputContent struct {
	Type        string        `json:"type"`
	Text        string        `json:"text,omitempty"`
	Annotations []interface{} `json:"annotations,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewOpenAIClient(cfg config.OpenAIConfig, logger *slog.Logger, productService ProductService) OpenAIClient {
	return NewOpenAIClientWithPrompts(cfg, logger, "prompts", productService)
}

func NewOpenAIClientWithPrompts(cfg config.OpenAIConfig, logger *slog.Logger, promptsDir string, productService ProductService) OpenAIClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-5-nano" // Default to GPT-5 nano
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 300 // Appropriate for JSON responses
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.1 // Low temperature for consistent parsing
	}
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = "medium" // Default reasoning level
	}
	// Default to using Responses API for new models
	if cfg.Model == "gpt-5" || cfg.Model == "gpt-5-nano" {
		cfg.UseResponsesAPI = true
		cfg.Store = true // Enable stateful context for better reasoning
	}

	promptBuilder := NewPromptBuilder(promptsDir)

	// Validate prompt files on initialization
	if err := promptBuilder.ValidatePromptFiles(); err != nil {
		logger.Warn("Prompt files validation failed, falling back to built-in prompts", "error", err)
		promptBuilder = nil // Will use fallback prompts
	}

	// Initialize metrics
	meter := otel.Meter("ai-service")

	parseRequestsTotal, _ := meter.Int64Counter(
		"ai_parse_requests_total",
		metric.WithDescription("Total number of AI parsing requests"),
	)

	parseRequestDuration, _ := meter.Float64Histogram(
		"ai_parse_request_duration_seconds",
		metric.WithDescription("Duration of AI parsing requests"),
		metric.WithUnit("s"),
	)

	parseRequestErrors, _ := meter.Int64Counter(
		"ai_parse_request_errors_total",
		metric.WithDescription("Total number of AI parsing request errors"),
	)

	apiRequestsTotal, _ := meter.Int64Counter(
		"ai_api_requests_total",
		metric.WithDescription("Total number of OpenAI API requests"),
	)

	apiRequestDuration, _ := meter.Float64Histogram(
		"ai_api_request_duration_seconds",
		metric.WithDescription("Duration of OpenAI API requests"),
		metric.WithUnit("s"),
	)

	apiRequestErrors, _ := meter.Int64Counter(
		"ai_api_request_errors_total",
		metric.WithDescription("Total number of OpenAI API request errors"),
	)

	tokensUsed, _ := meter.Int64Counter(
		"ai_tokens_used_total",
		metric.WithDescription("Total number of tokens used"),
	)

	confidenceScoreHistogram, _ := meter.Float64Histogram(
		"ai_confidence_score",
		metric.WithDescription("Confidence scores of AI parsing results"),
	)

	return &openAIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 45 * time.Second, // Increased timeout for reasoning models
		},
		logger:         logger,
		promptBuilder:  promptBuilder,
		productService: productService,

		// Metrics
		parseRequestsTotal:       parseRequestsTotal,
		parseRequestDuration:     parseRequestDuration,
		parseRequestErrors:       parseRequestErrors,
		apiRequestsTotal:         apiRequestsTotal,
		apiRequestDuration:       apiRequestDuration,
		apiRequestErrors:         apiRequestErrors,
		tokensUsed:               tokensUsed,
		confidenceScoreHistogram: confidenceScoreHistogram,
	}
}

func (c *openAIClient) ParseItem(ctx context.Context, rawText, languageCode string) (*ParsedResult, error) {
	startTime := time.Now()

	// Record parse request metrics
	apiType := map[bool]string{true: "responses_api", false: "chat_completions"}[c.config.UseResponsesAPI]
	attrs := []attribute.KeyValue{
		attribute.String("api_type", apiType),
		attribute.String("language_code", languageCode),
		attribute.String("operation", "parse_single_item"),
	}

	c.parseRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	var result *ParsedResult
	var err error

	if c.config.UseResponsesAPI {
		result, err = c.parseItemWithResponsesAPI(ctx, rawText, languageCode)
	} else {
		result, err = c.parseItemWithChatCompletions(ctx, rawText, languageCode)
	}

	duration := time.Since(startTime)
	c.parseRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if err != nil {
		// Record error metrics
		errorAttrs := append(attrs, attribute.String("error_type", fmt.Sprintf("%T", err)))
		c.parseRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		c.logger.Error("Item parsing failed",
			"raw_text", rawText,
			"language_code", languageCode,
			"processing_time_ms", duration.Milliseconds(),
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err),
			"api_type", apiType,
			"component", "ai_parsing")
		return nil, err
	}

	// Record success metrics
	successAttrs := append(attrs,
		attribute.String("outcome", "success"),
		attribute.String("category", result.Category),
	)
	c.confidenceScoreHistogram.Record(ctx, result.ConfidenceScore, metric.WithAttributes(successAttrs...))

	c.logger.Info("Item parsing completed successfully",
		"raw_text", rawText,
		"language_code", languageCode,
		"parsed_name", result.StandardizedName,
		"parsed_quantity_value", result.QuantityValue,
		"parsed_quantity_unit", result.QuantityUnit,
		"parsed_category", result.Category,
		"confidence_score", result.ConfidenceScore,
		"processing_time_ms", duration.Milliseconds(),
		"api_type", apiType,
		"component", "ai_parsing")

	return result, nil
}

func (c *openAIClient) ParseItems(ctx context.Context, rawText, languageCode string) ([]*ParsedResult, error) {
	startTime := time.Now()

	// Record parse request metrics
	apiType := map[bool]string{true: "responses_api", false: "chat_completions"}[c.config.UseResponsesAPI]
	attrs := []attribute.KeyValue{
		attribute.String("api_type", apiType),
		attribute.String("language_code", languageCode),
		attribute.String("operation", "parse_multiple_items"),
		attribute.Int("text_length", len(rawText)),
	}

	c.parseRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	var results []*ParsedResult
	var err error

	if c.config.UseResponsesAPI {
		results, err = c.parseItemsWithResponsesAPI(ctx, rawText, languageCode)
	} else {
		results, err = c.parseItemsWithChatCompletions(ctx, rawText, languageCode)
	}

	duration := time.Since(startTime)
	c.parseRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if err != nil {
		// Record error metrics
		errorAttrs := append(attrs, attribute.String("error_type", fmt.Sprintf("%T", err)))
		c.parseRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		c.logger.Error("Multi-item parsing failed",
			"raw_text", rawText,
			"language_code", languageCode,
			"text_length", len(rawText),
			"processing_time_ms", duration.Milliseconds(),
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err),
			"api_type", apiType,
			"component", "ai_parsing")
		return nil, err
	}

	// Record success metrics
	itemNames := make([]string, len(results))
	totalConfidence := 0.0
	for i, result := range results {
		itemNames[i] = result.StandardizedName
		totalConfidence += result.ConfidenceScore

		// Record individual item confidence scores
		itemAttrs := append(attrs,
			attribute.String("outcome", "success"),
			attribute.String("category", result.Category),
			attribute.Int("item_index", i),
		)
		c.confidenceScoreHistogram.Record(ctx, result.ConfidenceScore, metric.WithAttributes(itemAttrs...))
	}

	avgConfidence := totalConfidence / float64(len(results))

	// Record batch processing metrics
	successAttrs := append(attrs,
		attribute.String("outcome", "success"),
		attribute.Int("items_parsed", len(results)),
		attribute.Float64("average_confidence", avgConfidence),
		attribute.Float64("items_per_second", float64(len(results))/duration.Seconds()),
	)

	// Record overall parsing success (could add more metrics here if needed)
	_ = successAttrs // Mark as used for future extension

	c.logger.Info("Multi-item parsing completed successfully",
		"raw_text", rawText,
		"language_code", languageCode,
		"text_length", len(rawText),
		"items_parsed", len(results),
		"parsed_items", itemNames,
		"average_confidence", avgConfidence,
		"processing_time_ms", duration.Milliseconds(),
		"items_per_second", float64(len(results))/duration.Seconds(),
		"api_type", apiType,
		"component", "ai_parsing")

	return results, nil
}

func (c *openAIClient) parseItemWithResponsesAPI(ctx context.Context, rawText, languageCode string) (*ParsedResult, error) {
	prompt := c.buildPrompt(rawText, languageCode)

	// Load instructions from file
	var instructions string
	if c.promptBuilder != nil {
		var err error
		instructions, err = c.promptBuilder.LoadSingleItemInstructions()
		if err != nil {
			c.logger.Warn("Failed to load single item instructions from file, using fallback", "error", err)
			instructions = "You are an AI assistant that standardizes shopping list items. Parse the given item and extract structured information. Respond ONLY with valid JSON."
		}
	} else {
		instructions = "You are an AI assistant that standardizes shopping list items. Parse the given item and extract structured information. Respond ONLY with valid JSON."
	}

	reqBody := ResponsesRequest{
		Model: c.config.Model,
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf("%s\n\n%s\n\nItem: %s\nLanguage: %s", instructions, prompt, rawText, languageCode),
			},
		},
		Reasoning: &ResponsesReasoning{
			Effort: c.config.ReasoningEffort,
		},
	}

	// Enable storage for better reasoning if configured
	if c.config.Store {
		reqBody.Store = &c.config.Store
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		c.logger.Error("Failed to marshal OpenAI request",
			"error", err.Error(),
			"raw_text", rawText,
			"language_code", languageCode,
			"component", "ai_parsing",
			"error_category", "json_marshal")
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewBuffer(jsonBody))
	if err != nil {
		c.logger.Error("Failed to create HTTP request",
			"error", err.Error(),
			"url", c.config.BaseURL+"/responses",
			"raw_text", rawText,
			"language_code", languageCode,
			"component", "ai_parsing",
			"error_category", "http_request_creation")
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	requestStart := time.Now()

	// Record API request metrics
	apiAttrs := []attribute.KeyValue{
		attribute.String("api_endpoint", "responses"),
		attribute.String("language_code", languageCode),
		attribute.String("model", c.config.Model),
	}
	c.apiRequestsTotal.Add(ctx, 1, metric.WithAttributes(apiAttrs...))

	resp, err := c.httpClient.Do(req)
	requestDuration := time.Since(requestStart)
	c.apiRequestDuration.Record(ctx, requestDuration.Seconds(), metric.WithAttributes(apiAttrs...))

	if err != nil {
		// Record API error metrics
		errorAttrs := append(apiAttrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.String("error_category", "http_request_failed"),
		)
		c.apiRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		c.logger.Error("HTTP request to OpenAI failed",
			"error", err.Error(),
			"url", c.config.BaseURL+"/responses",
			"request_duration_ms", requestDuration.Milliseconds(),
			"raw_text", rawText,
			"language_code", languageCode,
			"component", "ai_parsing",
			"error_category", "http_request_failed")
		return nil, fmt.Errorf("failed to make responses request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read response body",
			"error", err.Error(),
			"status_code", resp.StatusCode,
			"request_duration_ms", requestDuration.Milliseconds(),
			"raw_text", rawText,
			"language_code", languageCode,
			"component", "ai_parsing",
			"error_category", "response_read")
		return nil, fmt.Errorf("failed to read responses: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Record API error metrics for non-200 status codes
		errorAttrs := append(apiAttrs,
			attribute.String("error_type", "api_error"),
			attribute.String("error_category", "api_error"),
			attribute.Int("status_code", resp.StatusCode),
		)
		c.apiRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		c.logger.Error("OpenAI API returned error status",
			"status_code", resp.StatusCode,
			"response_body", string(body),
			"request_duration_ms", requestDuration.Milliseconds(),
			"url", c.config.BaseURL+"/responses",
			"raw_text", rawText,
			"language_code", languageCode,
			"component", "ai_parsing",
			"error_category", "api_error")
		return nil, fmt.Errorf("responses API error: %d - %s", resp.StatusCode, string(body))
	}

	var responsesResp ResponsesResponse
	if err := json.Unmarshal(body, &responsesResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal responses: %w", err)
	}

	// Extract text from responses output
	outputText, err := c.extractOutputText(responsesResp.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract output text: %w", err)
	}

	// Parse the AI response
	result, err := c.parseAIResponse(outputText)
	if err != nil {
		c.logger.Error("Failed to parse AI response from Responses API", "error", err, "response", outputText)
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Record token usage metrics
	tokenAttrs := append(apiAttrs,
		attribute.String("token_type", "total"),
		attribute.String("outcome", "success"),
	)
	c.tokensUsed.Add(ctx, int64(responsesResp.Usage.TotalTokens), metric.WithAttributes(tokenAttrs...))

	c.logger.Info("Successfully parsed item with Responses API",
		"raw_text", rawText,
		"language", languageCode,
		"model", c.config.Model,
		"standardized_name", result.StandardizedName,
		"confidence", result.ConfidenceScore,
		"tokens_used", responsesResp.Usage.TotalTokens,
		"reasoning_effort", c.config.ReasoningEffort)

	return result, nil
}

func (c *openAIClient) parseItemWithChatCompletions(ctx context.Context, rawText, languageCode string) (*ParsedResult, error) {
	prompt := c.buildPrompt(rawText, languageCode)

	reqBody := ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    []Message{{Role: "user", Content: prompt}},
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
	}

	if c.config.Store {
		reqBody.Store = &c.config.Store
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	requestStart := time.Now()

	// Record API request metrics
	apiAttrs := []attribute.KeyValue{
		attribute.String("api_endpoint", "chat_completions"),
		attribute.String("language_code", languageCode),
		attribute.String("model", c.config.Model),
	}
	c.apiRequestsTotal.Add(ctx, 1, metric.WithAttributes(apiAttrs...))

	resp, err := c.httpClient.Do(req)
	requestDuration := time.Since(requestStart)
	c.apiRequestDuration.Record(ctx, requestDuration.Seconds(), metric.WithAttributes(apiAttrs...))

	if err != nil {
		// Record API error metrics
		errorAttrs := append(apiAttrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.String("error_category", "http_request_failed"),
		)
		c.apiRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		return nil, fmt.Errorf("failed to make chat completion request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read chat completion response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Record API error metrics for non-200 status codes
		errorAttrs := append(apiAttrs,
			attribute.String("error_type", "api_error"),
			attribute.String("error_category", "api_error"),
			attribute.Int("status_code", resp.StatusCode),
		)
		c.apiRequestErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))

		return nil, fmt.Errorf("chat completion API error: %d - %s", resp.StatusCode, string(body))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chat completion response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in chat completion response")
	}

	// Parse the AI response
	result, err := c.parseAIResponse(chatResp.Choices[0].Message.Content)
	if err != nil {
		c.logger.Error("Failed to parse AI response from Chat Completions", "error", err, "response", chatResp.Choices[0].Message.Content)
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Record token usage metrics
	tokenAttrs := append(apiAttrs,
		attribute.String("token_type", "total"),
		attribute.String("outcome", "success"),
	)
	c.tokensUsed.Add(ctx, int64(chatResp.Usage.TotalTokens), metric.WithAttributes(tokenAttrs...))

	c.logger.Info("Successfully parsed item with Chat Completions",
		"raw_text", rawText,
		"language", languageCode,
		"model", c.config.Model,
		"standardized_name", result.StandardizedName,
		"confidence", result.ConfidenceScore,
		"tokens_used", chatResp.Usage.TotalTokens)

	return result, nil
}

func (c *openAIClient) extractOutputText(output []ResponsesOutputItem) (string, error) {
	for _, item := range output {
		if item.Type == "message" && item.Role == "assistant" {
			for _, content := range item.Content {
				if content.Type == "output_text" || content.Type == "text" {
					return content.Text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no output text found in responses")
}

func (c *openAIClient) buildPrompt(rawText, languageCode string) string {
	if c.promptBuilder == nil {
		c.logger.Error("Prompt builder not available - cannot build prompt")
		return ""
	}

	prompt, err := c.promptBuilder.BuildPrompt(rawText, languageCode)
	if err != nil {
		c.logger.Error("Failed to build prompt from files", "error", err)
		return ""
	}

	return prompt
}

func (c *openAIClient) parseAIResponse(content string) (*ParsedResult, error) {
	// Clean up the response - remove any markdown formatting or extra text
	content = strings.TrimSpace(content)

	// Find JSON in the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")

	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no valid JSON found in response: %s", content)
	}

	jsonStr := content[start : end+1]

	var result ParsedResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w - content: %s", err, jsonStr)
	}

	// Validate required fields
	if result.StandardizedName == "" {
		return nil, fmt.Errorf("standardized_name is required")
	}
	if result.ConfidenceScore < 0.0 || result.ConfidenceScore > 1.0 {
		result.ConfidenceScore = 0.5 // Default confidence
	}

	return &result, nil
}

func (c *openAIClient) parseItemsWithResponsesAPI(ctx context.Context, rawText, languageCode string) ([]*ParsedResult, error) {
	prompt := c.buildMultiItemPrompt(rawText, languageCode)

	if prompt == "" {
		return nil, fmt.Errorf("failed to build multi-item prompt")
	}

	reqBody := ResponsesRequest{
		Model: c.config.Model,
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Reasoning: &ResponsesReasoning{
			Effort: c.config.ReasoningEffort,
		},
	}

	// Enable storage for better reasoning if configured
	if c.config.Store {
		reqBody.Store = &c.config.Store
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	// Log the full request being sent to OpenAI
	c.logger.Info("[MULTI] Sending request to OpenAI Responses API",
		"url", c.config.BaseURL+"/responses",
		"model", c.config.Model,
		"api_key_prefix", c.config.APIKey[:min(10, len(c.config.APIKey))]+"...", // Only log first 10 chars for security
		"raw_text", rawText,
		"language", languageCode,
		"request_body", string(jsonBody))

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("[MULTI] HTTP request to OpenAI failed", "error", err)
		return nil, fmt.Errorf("failed to make responses request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read responses: %w", err)
	}

	// Log the raw response from OpenAI
	c.logger.Info("[MULTI] Received response from OpenAI Responses API",
		"status_code", resp.StatusCode,
		"response_body", string(body))

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("[MULTI] OpenAI API error", "status_code", resp.StatusCode, "response", string(body))
		return nil, fmt.Errorf("responses API error: %d - %s", resp.StatusCode, string(body))
	}

	var responsesResp ResponsesResponse
	if err := json.Unmarshal(body, &responsesResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal responses: %w", err)
	}

	// Extract text from responses output
	outputText, err := c.extractOutputText(responsesResp.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract output text: %w", err)
	}

	// Parse the AI response as array
	results, err := c.parseAIResponseAsArray(outputText)
	if err != nil {
		c.logger.Error("Failed to parse AI response from Responses API", "error", err, "response", outputText)
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	c.logger.Info("Successfully parsed multiple items with Responses API",
		"raw_text", rawText,
		"language", languageCode,
		"model", c.config.Model,
		"items_count", len(results),
		"tokens_used", responsesResp.Usage.TotalTokens,
		"reasoning_effort", c.config.ReasoningEffort)

	return results, nil
}

func (c *openAIClient) parseItemsWithChatCompletions(ctx context.Context, rawText, languageCode string) ([]*ParsedResult, error) {
	prompt := c.buildMultiItemPrompt(rawText, languageCode)

	reqBody := ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    []Message{{Role: "user", Content: prompt}},
		MaxTokens:   c.config.MaxTokens * 2, // Double tokens for multiple items
		Temperature: c.config.Temperature,
	}

	if c.config.Store {
		reqBody.Store = &c.config.Store
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat completion request: %w", err)
	}

	// Log the full request being sent to OpenAI
	c.logger.Info("[MULTI-CHAT] Sending request to OpenAI Chat Completions API",
		"url", c.config.BaseURL+"/chat/completions",
		"model", c.config.Model,
		"api_key_prefix", c.config.APIKey[:min(10, len(c.config.APIKey))]+"...",
		"raw_text", rawText,
		"language", languageCode,
		"request_body", string(jsonBody))

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("[MULTI-CHAT] HTTP request to OpenAI failed", "error", err)
		return nil, fmt.Errorf("failed to make chat completion request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read chat completion response: %w", err)
	}

	// Log the raw response from OpenAI
	c.logger.Info("[MULTI-CHAT] Received response from OpenAI Chat Completions API",
		"status_code", resp.StatusCode,
		"response_body", string(body))

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("[MULTI-CHAT] OpenAI API error", "status_code", resp.StatusCode, "response", string(body))
		return nil, fmt.Errorf("chat completion API error: %d - %s", resp.StatusCode, string(body))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chat completion response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in chat completion response")
	}

	// Parse the AI response as array
	results, err := c.parseAIResponseAsArray(chatResp.Choices[0].Message.Content)
	if err != nil {
		c.logger.Error("Failed to parse AI response from Chat Completions", "error", err, "response", chatResp.Choices[0].Message.Content)
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	c.logger.Info("Successfully parsed multiple items with Chat Completions",
		"raw_text", rawText,
		"language", languageCode,
		"model", c.config.Model,
		"items_count", len(results),
		"tokens_used", chatResp.Usage.TotalTokens)

	return results, nil
}

func (c *openAIClient) buildMultiItemPrompt(rawText, languageCode string) string {
	if c.promptBuilder == nil {
		c.logger.Error("Prompt builder not available - cannot build multi-item prompt")
		return ""
	}

	// First, try to get products from the database
	ctx := context.Background() // Using a background context for this operation
	products, err := c.productService.GetAllProducts(ctx)
	if err != nil {
		c.logger.Warn("Failed to fetch products for AI prompt, falling back to basic prompt", "error", err)
		// Fall back to the old prompt without products
		prompt, err := c.promptBuilder.BuildMultiItemPrompt(rawText, languageCode)
		if err != nil {
			c.logger.Error("Failed to build fallback multi-item prompt from files", "error", err)
			return ""
		}
		return prompt
	}

	// Use the enhanced prompt with products
	prompt, err := c.promptBuilder.BuildMultiItemPromptWithProducts(rawText, languageCode, products)
	if err != nil {
		c.logger.Error("Failed to build multi-item prompt with products from files", "error", err)
		return ""
	}

	// Log the built prompt for debugging
	c.logger.Info("Built enhanced multi-item prompt for AI with products",
		"raw_text", rawText,
		"language", languageCode,
		"products_count", len(products),
		"prompt_length", len(prompt),
		"prompt_preview", prompt[:min(500, len(prompt))]+"...") // First 500 chars

	return prompt
}

func (c *openAIClient) parseAIResponseAsArray(content string) ([]*ParsedResult, error) {
	// Clean up the response - remove any markdown formatting or extra text
	content = strings.TrimSpace(content)

	// Find JSON array in the response
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")

	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no valid JSON array found in response: %s", content)
	}

	jsonStr := content[start : end+1]

	var results []*ParsedResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON array: %w - content: %s", err, jsonStr)
	}

	// Validate each result
	for i, result := range results {
		if result.StandardizedName == "" {
			return nil, fmt.Errorf("standardized_name is required for item %d", i)
		}
		if result.ConfidenceScore < 0.0 || result.ConfidenceScore > 1.0 {
			result.ConfidenceScore = 0.5 // Default confidence
		}
	}

	return results, nil
}

// DetectLanguage detects the language of the given text using OpenAI
func (c *openAIClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	// Build prompt using PromptBuilder
	var prompt string
	if c.promptBuilder != nil {
		var err error
		prompt, err = c.promptBuilder.BuildLanguageDetectionPrompt(text)
		if err != nil {
			c.logger.Error("Failed to build language detection prompt from file", "error", err)
			return "", fmt.Errorf("language detection prompt file is required: %w", err)
		}
	} else {
		c.logger.Error("PromptBuilder is not available for language detection")
		return "", fmt.Errorf("prompt builder is required for language detection")
	}

	reqData := ResponsesRequest{
		Model: "gpt-5-nano",
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Store:     &[]bool{true}[0],
		Reasoning: &ResponsesReasoning{Effort: "low"},
	}

	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal language detection request: %w", err)
	}

	c.logger.Info("[LANG] Sending language detection request to OpenAI Responses API",
		"url", c.config.BaseURL+"/responses",
		"model", reqData.Model,
		"text", text,
		"request_body", string(reqJSON))

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewReader(reqJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create language detection request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("language detection API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read language detection response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("language detection API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp ResponsesResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal language detection response: %w", err)
	}

	if len(apiResp.Output) == 0 {
		return "", fmt.Errorf("no language detection output returned")
	}

	// Find the assistant message in the output
	var content string
	for _, output := range apiResp.Output {
		if output.Role == "assistant" && len(output.Content) > 0 {
			content = output.Content[0].Text
			break
		}
	}

	if content == "" {
		return "", fmt.Errorf("no assistant response found in language detection output")
	}

	detectedLanguage := strings.TrimSpace(strings.ToLower(content))

	c.logger.Info("Successfully detected language with Responses API",
		"text", text,
		"detected_language", detectedLanguage,
		"model", reqData.Model)

	return detectedLanguage, nil
}

// DetectProductList detects if the given text contains a product/shopping list
func (c *openAIClient) DetectProductList(ctx context.Context, text string) (*ProductListDetectionResult, error) {
	// Build prompt using PromptBuilder
	var prompt string
	if c.promptBuilder != nil {
		var err error
		prompt, err = c.promptBuilder.BuildProductListDetectionPrompt(text)
		if err != nil {
			c.logger.Error("Failed to build product list detection prompt from file", "error", err)
			return nil, fmt.Errorf("product list detection prompt file is required: %w", err)
		}
	} else {
		c.logger.Error("PromptBuilder is not available for product list detection")
		return nil, fmt.Errorf("prompt builder is required for product list detection")
	}

	reqData := ResponsesRequest{
		Model: "gpt-5-nano",
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Store:     &[]bool{true}[0],
		Reasoning: &ResponsesReasoning{Effort: "low"},
	}

	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal product list detection request: %w", err)
	}

	c.logger.Info("[PRODUCT-LIST] Sending product list detection request to OpenAI Responses API",
		"url", c.config.BaseURL+"/responses",
		"model", reqData.Model,
		"text", text,
		"request_body", string(reqJSON))

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create product list detection request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("product list detection API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read product list detection response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product list detection API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp ResponsesResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal product list detection response: %w", err)
	}

	if len(apiResp.Output) == 0 {
		return nil, fmt.Errorf("no product list detection output returned")
	}

	// Find the assistant message in the output
	var content string
	for _, output := range apiResp.Output {
		if output.Role == "assistant" && len(output.Content) > 0 {
			content = output.Content[0].Text
			break
		}
	}

	if content == "" {
		return nil, fmt.Errorf("no assistant response found in product list detection output")
	}

	// Parse the JSON response
	content = strings.TrimSpace(content)

	// Find JSON in the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")

	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no valid JSON found in product list detection response: %s", content)
	}

	jsonStr := content[start : end+1]

	var result ProductListDetectionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal product list detection JSON: %w - content: %s", err, jsonStr)
	}

	// Validate the result
	if result.Confidence < 0.0 || result.Confidence > 1.0 {
		result.Confidence = 0.5 // Default confidence
	}

	c.logger.Info("Successfully detected product list with Responses API",
		"text", text,
		"is_product_list", result.IsProductList,
		"confidence", result.Confidence,
		"detected_items_count", result.DetectedItemsCount,
		"sample_items", result.SampleItems,
		"model", reqData.Model)

	return &result, nil
}

// Translate translates text from one language to another using OpenAI Responses API
func (c *openAIClient) Translate(ctx context.Context, originalText, originalLanguage, targetLanguage string) (*TranslationResult, error) {
	// Build prompt using PromptBuilder
	var prompt string
	if c.promptBuilder != nil {
		var err error
		prompt, err = c.promptBuilder.BuildTranslationPrompt(originalText, originalLanguage, targetLanguage)
		if err != nil {
			c.logger.Error("Failed to build translation prompt from file", "error", err)
			return nil, fmt.Errorf("translation prompt file is required: %w", err)
		}
	} else {
		c.logger.Error("PromptBuilder is not available for translation")
		return nil, fmt.Errorf("prompt builder is required for translation")
	}

	// Use Responses API only
	reqData := ResponsesRequest{
		Model: c.config.Model,
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Reasoning: &ResponsesReasoning{Effort: "low"}, // Translation doesn't need high reasoning
	}

	// Enable storage if configured
	if c.config.Store {
		reqData.Store = &c.config.Store
	}

	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal translation request: %w", err)
	}

	c.logger.Info("[TRANSLATE] Sending translation request to OpenAI Responses API",
		"url", c.config.BaseURL+"/responses",
		"model", reqData.Model,
		"original_text", originalText,
		"from", originalLanguage,
		"to", targetLanguage)

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create translation request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("translation API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read translation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("translation API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp ResponsesResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal translation response: %w", err)
	}

	// Extract text from responses output
	translatedText, err := c.extractOutputText(apiResp.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract translated text: %w", err)
	}

	// Clean up the response
	translatedText = strings.TrimSpace(translatedText)

	// Calculate confidence
	confidence := c.calculateTranslationConfidence(originalText, translatedText, originalLanguage, targetLanguage)

	c.logger.Info("Successfully translated text with Responses API",
		"original_text", originalText,
		"translated_text", translatedText,
		"from", originalLanguage,
		"to", targetLanguage,
		"confidence", confidence,
		"model", reqData.Model)

	return &TranslationResult{
		TranslatedText: translatedText,
		Confidence:     confidence,
	}, nil
}

// calculateTranslationConfidence estimates translation confidence based on heuristics
func (c *openAIClient) calculateTranslationConfidence(originalText, translatedText, originalLanguage, targetLanguage string) float64 {
	// Base confidence for successful translation
	confidence := 0.8

	// Boost confidence if the translation is significantly different (indicates actual translation occurred)
	if originalText != translatedText {
		confidence += 0.1
	}

	// Reduce confidence if translation is empty or too similar when languages are different
	if strings.TrimSpace(translatedText) == "" {
		return 0.1
	}

	// Reduce confidence if translation is identical to original when languages should be different
	normalizedOrigLang := strings.ToLower(strings.TrimSpace(originalLanguage))
	normalizedTargetLang := strings.ToLower(strings.TrimSpace(targetLanguage))
	if normalizedOrigLang != normalizedTargetLang && strings.TrimSpace(originalText) == strings.TrimSpace(translatedText) {
		confidence = 0.6 // Lower confidence for likely non-translation
	}

	// Boost confidence for known high-quality model responses
	if c.config.Model == "gpt-5" || c.config.Model == "gpt-5-nano" {
		confidence += 0.05
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

// BatchTranslateReceiptItems translates multiple receipt items in a single request
func (c *openAIClient) BatchTranslateReceiptItems(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	if len(req.Items) == 0 {
		return &BatchTranslationResult{
			DetectedLanguage: "",
			TargetLanguage:   req.TargetLocale,
			Translations:     []ReceiptItemTranslation{},
			Confidence:       1.0,
		}, nil
	}

	// Build batch translation prompt
	prompt, err := c.buildBatchTranslationPrompt(req.Items, req.TargetLocale)
	if err != nil {
		return nil, fmt.Errorf("failed to build batch translation prompt: %w", err)
	}

	// Use configured model with low reasoning for translation
	reqData := ResponsesRequest{
		Model: c.config.Model, // Use configured model (gpt-5-nano)
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Reasoning: &ResponsesReasoning{Effort: "low"}, // Low effort is sufficient for translation
	}

	// Enable storage if configured
	if c.config.Store {
		reqData.Store = &c.config.Store
	}

	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch translation request: %w", err)
	}

	c.logger.Info("[BATCH-TRANSLATE] Sending batch translation request to OpenAI Responses API",
		"url", c.config.BaseURL+"/responses",
		"model", reqData.Model,
		"items_count", len(req.Items),
		"target_locale", req.TargetLocale,
		"items", req.Items)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create batch translation request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("batch translation API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch translation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch translation API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp ResponsesResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal batch translation response: %w", err)
	}

	// Extract text from responses output
	responseText, err := c.extractOutputText(apiResp.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract batch translation text: %w", err)
	}

	// Parse the batch translation response
	result, err := c.parseBatchTranslationResponse(responseText, req.Items, req.TargetLocale)
	if err != nil {
		c.logger.Error("Failed to parse batch translation response",
			"error", err,
			"response", responseText,
			"items_count", len(req.Items))
		return nil, fmt.Errorf("failed to parse batch translation response: %w", err)
	}

	c.logger.Info("Successfully batch translated receipt items",
		"items_count", len(req.Items),
		"target_locale", req.TargetLocale,
		"detected_language", result.DetectedLanguage,
		"translations_count", len(result.Translations),
		"confidence", result.Confidence,
		"model", reqData.Model)

	return result, nil
}

// buildBatchTranslationPrompt creates a prompt for batch translation of receipt items
func (c *openAIClient) buildBatchTranslationPrompt(items []string, targetLocale string) (string, error) {
	if c.promptBuilder == nil {
		return "", fmt.Errorf("prompt builder is not available - batch translation requires prompt files")
	}

	prompt, err := c.promptBuilder.BuildBatchTranslationPrompt(items, targetLocale)
	if err != nil {
		return "", fmt.Errorf("failed to build batch translation prompt from file: %w", err)
	}
	return prompt, nil
}

// parseBatchTranslationResponse parses the AI response for batch translation
func (c *openAIClient) parseBatchTranslationResponse(content string, originalItems []string, targetLocale string) (*BatchTranslationResult, error) {
	// Clean up the response
	content = strings.TrimSpace(content)

	// Find JSON in the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")

	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no valid JSON found in batch translation response: %s", content)
	}

	jsonStr := content[start : end+1]

	var result BatchTranslationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal batch translation JSON: %w - content: %s", err, jsonStr)
	}

	// Validate the result
	if result.Confidence < 0.0 || result.Confidence > 1.0 {
		result.Confidence = 0.8 // Default confidence
	}

	// Ensure we have translations for all items
	if len(result.Translations) != len(originalItems) {
		return nil, fmt.Errorf("batch translation returned %d translations for %d items - AI must translate all items", len(result.Translations), len(originalItems))
	}

	// Validate each translation
	for i := range result.Translations {
		if result.Translations[i].Confidence < 0.0 || result.Translations[i].Confidence > 1.0 {
			result.Translations[i].Confidence = 0.7 // Default confidence for individual items
		}
		if result.Translations[i].DetectedLanguage == "" {
			result.Translations[i].DetectedLanguage = result.DetectedLanguage
		}
		if result.Translations[i].TargetLanguage == "" {
			result.Translations[i].TargetLanguage = result.TargetLanguage
		}
	}

	return &result, nil
}

// Helper function for min operation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
