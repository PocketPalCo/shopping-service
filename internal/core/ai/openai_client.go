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

	return &openAIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 45 * time.Second, // Increased timeout for reasoning models
		},
		logger:         logger,
		promptBuilder:  promptBuilder,
		productService: productService,
	}
}

func (c *openAIClient) ParseItem(ctx context.Context, rawText, languageCode string) (*ParsedResult, error) {
	if c.config.UseResponsesAPI {
		return c.parseItemWithResponsesAPI(ctx, rawText, languageCode)
	}
	return c.parseItemWithChatCompletions(ctx, rawText, languageCode)
}

func (c *openAIClient) ParseItems(ctx context.Context, rawText, languageCode string) ([]*ParsedResult, error) {
	if c.config.UseResponsesAPI {
		return c.parseItemsWithResponsesAPI(ctx, rawText, languageCode)
	}
	return c.parseItemsWithChatCompletions(ctx, rawText, languageCode)
}

func (c *openAIClient) parseItemWithResponsesAPI(ctx context.Context, rawText, languageCode string) (*ParsedResult, error) {
	prompt := c.buildPrompt(rawText, languageCode)

	// Split prompt into instructions and input for better structure
	instructions := "You are an AI assistant that standardizes shopping list items. Parse the given item and extract structured information. Respond ONLY with valid JSON."

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
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/responses", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make responses request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read responses: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make chat completion request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read chat completion response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
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

	// Split prompt into instructions and input for better structure
	instructions := "You are an AI assistant that intelligently separates and standardizes shopping list items. Parse the input text and extract multiple items if present. Respond ONLY with valid JSON array."

	reqBody := ResponsesRequest{
		Model: c.config.Model,
		Input: []ResponsesMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf("%s\n\n%s\n\nInput: %s\nLanguage: %s", instructions, prompt, rawText, languageCode),
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	// Use a simple prompt to detect language
	prompt := fmt.Sprintf(`Detect the language of the following text and respond with ONLY the language code (ru, uk, or en):

Text: "%s"

Response format: just the 2-letter language code (ru for Russian, uk for Ukrainian, en for English)`, text)

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

// Helper function for min operation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
