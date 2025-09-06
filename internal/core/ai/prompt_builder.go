package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
