# AI Parsing Prompts

This directory contains modular prompt files for the AI-powered shopping list item parsing system. The prompts are designed to be easily customizable and maintainable.

## File Structure

### Core Prompt Files

- **`single_item_system_prompt.txt`** - System prompt for parsing single items with JSON output format
- **`multi_item_system_prompt.txt`** - System prompt for intelligently separating and parsing multiple items 
- **`single_item_task_template.txt`** - Task template for single item parsing requests
- **`multi_item_task_template.txt`** - Task template for multi-item parsing requests
- **`categories_context.txt`** - Comprehensive list of categories, subcategories, and quantity units
- **`ambiguous_mappings.txt`** - Guidance for handling ambiguous items and edge cases

### Language-Specific Examples

- **`english_examples.txt`** - Examples and patterns for English language items
- **`russian_examples.txt`** - Examples for Russian language items (ru)
- **`ukrainian_examples.txt`** - Examples for Ukrainian language items (uk)

## How It Works

The `PromptBuilder` automatically combines these files to create comprehensive prompts:

1. **System Prompt** - Sets up the AI's role and output format (single-item or multi-item)
2. **Categories Context** - Provides standardized categories and units
3. **Language Examples** - Shows language-specific patterns and corrections
4. **Ambiguous Mappings** - Handles edge cases and low-confidence items
5. **Task Template** - Adds the specific parsing task with placeholders for input text and language

## Customization

### Adding New Languages

1. Create a new file: `{language_code}_examples.txt`
2. Follow the format of existing example files
3. Include common items, typos, and regional variations
4. The system will automatically detect and use the new language

Example for German (`german_examples.txt`):
```
## German Language Examples (de):

### Basic Items:
- "milch" → {"standardized_name": "milch", "category": "dairy", "subcategory": "milk", "quantity_unit": "pieces", "confidence_score": 0.95}
- "brot" → {"standardized_name": "brot", "category": "bakery", "subcategory": "bread", "quantity_unit": "pieces", "confidence_score": 0.95}
```

### Modifying Categories

Edit `categories_context.txt` to:
- Add new product categories
- Modify subcategory groupings  
- Add new quantity units
- Adjust category hierarchies

### Handling New Ambiguous Cases

Edit `ambiguous_mappings.txt` to:
- Add new ambiguous item mappings
- Modify confidence score guidelines
- Add context-based resolution strategies
- Handle regional variations

### Customizing System Behavior

Edit system prompt files to:
- **`single_item_system_prompt.txt`** - Modify single-item parsing behavior, output format, rules
- **`multi_item_system_prompt.txt`** - Modify multi-item separation logic, array output format
- **Task templates** - Adjust specific instructions and examples for each parsing type

## Configuration

### Using Custom Prompts Directory

```go
// Use custom prompts directory
client := ai.NewOpenAIClientWithPrompts(config, logger, "/path/to/custom/prompts")
```

### Environment-Specific Prompts

You can maintain different prompt sets for different environments:

```
prompts/
├── production/
│   ├── base_system_prompt.txt
│   └── ...
├── development/
│   ├── base_system_prompt.txt  
│   └── ...
└── testing/
    ├── base_system_prompt.txt
    └── ...
```

## Validation

The system validates that all required files exist on startup:
- `single_item_system_prompt.txt` (required)
- `multi_item_system_prompt.txt` (required)
- `single_item_task_template.txt` (required)
- `multi_item_task_template.txt` (required)
- `categories_context.txt` (required) 
- `english_examples.txt` (required - fallback language)
- `ambiguous_mappings.txt` (required)

Missing files will trigger an error and prevent AI parsing from working properly.

## Best Practices

### Writing Examples
- Include both correct and incorrect variations
- Show quantity extraction patterns
- Cover regional/cultural differences
- Provide confidence score guidance

### Category Design
- Use English for categories (consistency)
- Preserve original language in item names
- Group related items logically
- Consider user shopping patterns

### Ambiguity Handling
- Use lower confidence for unclear items
- Provide multiple interpretation strategies
- Document common edge cases
- Include fallback logic

## Testing Prompts

Create test cases to verify prompt effectiveness:

```go
testCases := []struct{
    input    string
    language string  
    expected ParsedResult
}{
    {"raw", "en", ParsedResult{StandardizedName: "milk", Category: "dairy"}},
    {"coffe", "en", ParsedResult{StandardizedName: "coffee", Category: "beverages"}},
}
```

## Performance Considerations

- **Prompt Length**: Longer prompts cost more tokens but provide better context
- **Example Count**: More examples improve accuracy but increase cost  
- **Language Coverage**: Add only languages with significant user base
- **Update Frequency**: Monitor parsing accuracy and update prompts based on user feedback

## Monitoring and Analytics

Track these metrics to optimize prompts:
- Parsing accuracy by language
- Confidence score distributions  
- Most common parsing failures
- User correction patterns

Use this data to iteratively improve prompt quality and coverage.