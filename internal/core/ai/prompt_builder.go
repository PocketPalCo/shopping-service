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
		// Fallback to English examples if language not found
		languageExamples, err = pb.loadPromptFile("english_examples.txt")
		if err != nil {
			return "", fmt.Errorf("failed to load language examples: %w", err)
		}
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
		// Fallback to English examples if language not found
		languageExamples, err = pb.loadPromptFile("english_examples.txt")
		if err != nil {
			return "", fmt.Errorf("failed to load language examples: %w", err)
		}
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
		// Fallback to English examples if language not found
		languageExamples, err = pb.loadPromptFile("english_examples.txt")
		if err != nil {
			return "", fmt.Errorf("failed to load language examples: %w", err)
		}
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
	filepath := filepath.Join(pb.promptsDir, filename)
	content, err := os.ReadFile(filepath)
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
	}

	for _, file := range requiredFiles {
		filepath := filepath.Join(pb.promptsDir, file)
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			return fmt.Errorf("required prompt file missing: %s", file)
		}
	}

	return nil
}

// buildProductsContext creates a context string with products for the AI to use
func (pb *PromptBuilder) buildProductsContext(productsList []*products.Product, languageCode string) string {
	var context strings.Builder
	context.WriteString("IMPORTANT - PRODUCT REFERENCE TABLE:\n")
	context.WriteString("You MUST use ONLY the standardized product names from this table. ")
	context.WriteString("Match user input to the closest product in the appropriate language column.\n\n")

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
	context.WriteString("1. User input 'морква филе' should match 'морковка' AND 'куриное филе' as separate items\n")
	context.WriteString("2. Use the EXACT standardized name from the table, don't modify it\n")
	context.WriteString("3. If input doesn't match any product, use the closest match with lower confidence\n")
	context.WriteString("4. Consider aliases when matching (e.g., 'филе' matches 'куриное филе')\n")
	context.WriteString("5. 'кролик,кролик' should result in ONE item 'кролик', not duplicates\n\n")

	return context.String()
}
