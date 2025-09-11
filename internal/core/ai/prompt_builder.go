package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/products"
)

type PromptBuilder struct {
	promptsDir string
}

func NewPromptBuilder(promptsDir string) *PromptBuilder {
	if promptsDir == "" {
		promptsDir = "prompts" // Default directory
	}
	return &PromptBuilder{
		promptsDir: promptsDir,
	}
}

func (pb *PromptBuilder) BuildPrompt(rawText, languageCode string) (string, error) {
	// Load single-item system prompt
	systemPrompt, err := pb.loadPromptFile("single_item_system_prompt.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load single-item system prompt: %w", err)
	}

	// Load categories context
	categoriesContext, err := pb.loadPromptFile("categories_context.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load categories context: %w", err)
	}

	// Load language-specific examples
	examplesFile := fmt.Sprintf("%s_examples.txt", languageCode)
	languageExamples, err := pb.loadPromptFile(examplesFile)
	if err != nil {
		return "", fmt.Errorf("failed to load language examples for %s: %w", languageCode, err)
	}

	// Load ambiguous mappings
	ambiguousContext, err := pb.loadPromptFile("ambiguous_mappings.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load ambiguous mappings: %w", err)
	}

	// Load task template
	taskTemplate, err := pb.loadPromptFile("single_item_task_template.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load single-item task template: %w", err)
	}

	// Replace placeholders in task template
	task := strings.Replace(taskTemplate, "{raw_text}", rawText, -1)
	task = strings.Replace(task, "{language_code}", languageCode, -1)

	// Build the complete prompt
	prompt := fmt.Sprintf(`%s

%s

%s

%s

%s`,
		systemPrompt,
		categoriesContext,
		languageExamples,
		ambiguousContext,
		task)

	return prompt, nil
}

func (pb *PromptBuilder) BuildMultiItemPromptWithProducts(rawText, languageCode string, productsList []*products.Product) (string, error) {
	// Build the products context for the AI
	productsContext := pb.buildProductsContext(productsList, languageCode)

	// Load multi-item system prompt and add products context to it
	systemPrompt, err := pb.loadPromptFile("multi_item_system_prompt.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load multi-item system prompt: %w", err)
	}

	// Enhance system prompt with products context
	enhancedSystemPrompt := systemPrompt + "\n\n" + productsContext

	// Load categories context (now less relevant since we have products)
	categoriesContext, err := pb.loadPromptFile("categories_context.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load categories context: %w", err)
	}

	// Load language-specific examples
	examplesFile := fmt.Sprintf("%s_examples.txt", languageCode)
	languageExamples, err := pb.loadPromptFile(examplesFile)
	if err != nil {
		return "", fmt.Errorf("failed to load language examples for %s: %w", languageCode, err)
	}

	// Load ambiguous mappings
	ambiguousContext, err := pb.loadPromptFile("ambiguous_mappings.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load ambiguous mappings: %w", err)
	}

	// Load task template
	taskTemplate, err := pb.loadPromptFile("multi_item_task_template.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load multi-item task template: %w", err)
	}

	// Replace placeholders in task template
	task := strings.Replace(taskTemplate, "{raw_text}", rawText, -1)
	task = strings.Replace(task, "{language_code}", languageCode, -1)

	// Build the complete prompt
	prompt := fmt.Sprintf(`%s

%s

%s

%s

%s`,
		enhancedSystemPrompt,
		categoriesContext,
		languageExamples,
		ambiguousContext,
		task)

	return prompt, nil
}

func (pb *PromptBuilder) BuildMultiItemPrompt(rawText, languageCode string) (string, error) {
	// Load multi-item system prompt
	systemPrompt, err := pb.loadPromptFile("multi_item_system_prompt.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load multi-item system prompt: %w", err)
	}

	// Load categories context
	categoriesContext, err := pb.loadPromptFile("categories_context.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load categories context: %w", err)
	}

	// Load language-specific examples
	examplesFile := fmt.Sprintf("%s_examples.txt", languageCode)
	languageExamples, err := pb.loadPromptFile(examplesFile)
	if err != nil {
		return "", fmt.Errorf("failed to load language examples for %s: %w", languageCode, err)
	}

	// Load ambiguous mappings
	ambiguousContext, err := pb.loadPromptFile("ambiguous_mappings.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load ambiguous mappings: %w", err)
	}

	// Load task template
	taskTemplate, err := pb.loadPromptFile("multi_item_task_template.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load multi-item task template: %w", err)
	}

	// Replace placeholders in task template
	task := strings.Replace(taskTemplate, "{raw_text}", rawText, -1)
	task = strings.Replace(task, "{language_code}", languageCode, -1)

	// Build the complete prompt
	prompt := fmt.Sprintf(`%s

%s

%s

%s

%s`,
		systemPrompt,
		categoriesContext,
		languageExamples,
		ambiguousContext,
		task)

	return prompt, nil
}

func (pb *PromptBuilder) loadPromptFile(filename string) (string, error) {
	// Validate filename to prevent path traversal attacks
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return "", fmt.Errorf("invalid filename: %s", filename)
	}
	
	filePath := filepath.Join(pb.promptsDir, filename)
	
	// Ensure the resolved path is still within the prompts directory
	absPromptsDir, err := filepath.Abs(pb.promptsDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve prompts directory: %w", err)
	}
	
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}
	
	if !strings.HasPrefix(absFilePath, absPromptsDir) {
		return "", fmt.Errorf("file path outside prompts directory: %s", filename)
	}
	
	content, err := os.ReadFile(filePath) // #nosec G304 - path validated for traversal attacks
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %s: %w", filename, err)
	}
	return strings.TrimSpace(string(content)), nil
}

// GetAvailableLanguages returns list of available language codes
func (pb *PromptBuilder) GetAvailableLanguages() ([]string, error) {
	files, err := os.ReadDir(pb.promptsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts directory: %w", err)
	}

	var languages []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), "_examples.txt") {
			lang := strings.TrimSuffix(strings.TrimSuffix(file.Name(), "_examples.txt"), "")
			languages = append(languages, lang)
		}
	}

	return languages, nil
}

// ValidatePromptFiles checks that all required prompt files exist
func (pb *PromptBuilder) ValidatePromptFiles() error {
	requiredFiles := []string{
		"single_item_system_prompt.txt",
		"multi_item_system_prompt.txt",
		"single_item_task_template.txt",
		"multi_item_task_template.txt",
		"categories_context.txt",
		"english_examples.txt",
		"ambiguous_mappings.txt",
		"language_detection_prompt.txt",
		"single_item_instructions.txt",
		"multi_item_instructions.txt",
	}

	for _, file := range requiredFiles {
		filepath := filepath.Join(pb.promptsDir, file)
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			return fmt.Errorf("required prompt file missing: %s", file)
		}
	}

	return nil
}

// BuildLanguageDetectionPrompt creates a language detection prompt
func (pb *PromptBuilder) BuildLanguageDetectionPrompt(text string) (string, error) {
	// Load language detection prompt template
	promptTemplate, err := pb.loadPromptFile("language_detection_prompt.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load language detection prompt: %w", err)
	}

	// Replace placeholder with actual text
	prompt := strings.Replace(promptTemplate, "{text}", text, -1)
	
	return prompt, nil
}

// LoadSingleItemInstructions loads the instructions for single item parsing
func (pb *PromptBuilder) LoadSingleItemInstructions() (string, error) {
	return pb.loadPromptFile("single_item_instructions.txt")
}

// LoadMultiItemInstructions loads the instructions for multi-item parsing
func (pb *PromptBuilder) LoadMultiItemInstructions() (string, error) {
	return pb.loadPromptFile("multi_item_instructions.txt")
}

// BuildProductListDetectionPrompt creates a product list detection prompt
func (pb *PromptBuilder) BuildProductListDetectionPrompt(text string) (string, error) {
	// Load product list detection prompt template
	promptTemplate, err := pb.loadPromptFile("product_list_detection_prompt.txt")
	if err != nil {
		return "", fmt.Errorf("failed to load product list detection prompt: %w", err)
	}

	// Replace placeholder with actual text
	prompt := strings.Replace(promptTemplate, "{text}", text, -1)
	
	return prompt, nil
}

// buildProductsContext creates a context string with products for the AI to use
func (pb *PromptBuilder) buildProductsContext(productsList []*products.Product, languageCode string) string {
	var context strings.Builder
	context.WriteString("IMPORTANT - PRODUCT REFERENCE TABLE:\n")
	context.WriteString("When user input matches products in this table, use the standardized names from the appropriate language column. ")
	context.WriteString("When user input does NOT match any product in this table, generate standardized names in the SAME language as the input. ")
	context.WriteString("NEVER translate items to English - always use the input language.\n\n")

	// Group products by category for better organization
	categoryMap := make(map[string][]*products.Product)
	for _, product := range productsList {
		categoryMap[product.Category] = append(categoryMap[product.Category], product)
	}

	for category, categoryProducts := range categoryMap {
		context.WriteString(fmt.Sprintf("=== %s ===\n", category))

		for _, product := range categoryProducts {
			// Show the product name in the requested language plus alternatives
			var mainName string
			switch languageCode {
			case "ru", "russian":
				mainName = product.NameRu
			case "uk", "ukrainian":
				mainName = product.NameUk
			default:
				mainName = product.NameEn
			}

			context.WriteString(fmt.Sprintf("• %s", mainName))

			// Add alternative names and aliases
			alternatives := []string{}
			if languageCode != "en" && product.NameEn != mainName {
				alternatives = append(alternatives, product.NameEn)
			}
			if languageCode != "ru" && product.NameRu != mainName {
				alternatives = append(alternatives, product.NameRu)
			}
			if languageCode != "uk" && product.NameUk != mainName {
				alternatives = append(alternatives, product.NameUk)
			}

			// Add aliases
			for _, alias := range product.Aliases {
				if alias != mainName {
					alternatives = append(alternatives, alias)
				}
			}

			if len(alternatives) > 0 {
				context.WriteString(fmt.Sprintf(" (also: %s)", strings.Join(alternatives, ", ")))
			}

			context.WriteString(fmt.Sprintf(" [%s → %s]\n", product.Subcategory, product.Category))
		}
		context.WriteString("\n")
	}

	context.WriteString("MATCHING RULES:\n")
	context.WriteString("1. User input 'морква филе' should match 'морква' AND 'куряче філе' as separate items\n")
	context.WriteString("2. Use the EXACT standardized name from the table when there's a match\n")
	context.WriteString("3. If input doesn't match any product, generate proper standardized name in the same language with lower confidence\n")
	context.WriteString("4. Consider aliases when matching (e.g., 'філе' matches 'куряче філе')\n")
	context.WriteString("5. 'кролик,кролик' should result in ONE item 'кролик', not duplicates\n")
	context.WriteString("6. CRITICAL: салфетки → 'серветки' (Ukrainian, NOT 'toilet paper'), малина → 'малина' (NOT 'berries')\n")
	context.WriteString("7. Always maintain input language - Ukrainian input = Ukrainian output, Russian input = Russian output\n\n")

	return context.String()
}
