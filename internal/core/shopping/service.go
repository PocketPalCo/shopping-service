package shopping

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("shopping-service")

type ShoppingList struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description *string    `json:"description" db:"description"`
	OwnerID     uuid.UUID  `json:"owner_id" db:"owner_id"`
	FamilyID    *uuid.UUID `json:"family_id" db:"family_id"` // Optional family association
	IsShared    bool       `json:"is_shared" db:"is_shared"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

type ShoppingItem struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	ListID         uuid.UUID  `json:"list_id" db:"list_id"`
	Name           string     `json:"name" db:"name"`
	Quantity       *string    `json:"quantity" db:"quantity"`
	IsCompleted    bool       `json:"is_completed" db:"is_completed"`
	AddedBy        uuid.UUID  `json:"added_by" db:"added_by"`
	CompletedBy    *uuid.UUID `json:"completed_by" db:"completed_by"`
	CompletedAt    *time.Time `json:"completed_at" db:"completed_at"`
	OriginalItemID *uuid.UUID `json:"original_item_id" db:"original_item_id"`
	ParsedItemID   *uuid.UUID `json:"parsed_item_id" db:"parsed_item_id"`
	DisplayName    *string    `json:"display_name" db:"display_name"`    // Original user input for display
	ParsedName     *string    `json:"parsed_name" db:"parsed_name"`      // AI-parsed name for buttons
	ParsingStatus  string     `json:"parsing_status" db:"parsing_status"` // 'pending', 'parsed', 'failed'
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type CreateShoppingListRequest struct {
	Name        string
	Description *string
	OwnerID     uuid.UUID
	FamilyID    *uuid.UUID // Optional family association
	IsShared    bool
}

// AIService interface for item parsing
type AIService interface {
	GetOrCreateParsedItem(ctx context.Context, rawText, languageCode string, userID uuid.UUID) (*ai.ParsedResult, error)
	ParseAndStoreItems(ctx context.Context, rawText, languageCode string, userID uuid.UUID) ([]*ai.ParsedResult, error)
}

type Service struct {
	db        *pgxpool.Pool
	aiService AIService
	logger    *slog.Logger
}

func NewService(db *pgxpool.Pool, aiService AIService) *Service {
	logger := slog.Default().With("component", "shopping-service")
	return &Service{
		db:        db,
		aiService: aiService,
		logger:    logger,
	}
}

// CreateShoppingList creates a new shopping list
func (s *Service) CreateShoppingList(ctx context.Context, req CreateShoppingListRequest) (*ShoppingList, error) {
	ctx, span := tracer.Start(ctx, "shopping.CreateShoppingList")
	defer span.End()

	query := `
		INSERT INTO shopping_lists (name, description, owner_id, family_id, is_shared)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, owner_id, family_id, is_shared, created_at, updated_at`

	var list ShoppingList
	err := s.db.QueryRow(ctx, query,
		req.Name,
		req.Description,
		req.OwnerID,
		req.FamilyID,
		req.IsShared,
	).Scan(
		&list.ID,
		&list.Name,
		&list.Description,
		&list.OwnerID,
		&list.FamilyID,
		&list.IsShared,
		&list.CreatedAt,
		&list.UpdatedAt,
	)

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create shopping list: %w", err)
	}

	return &list, nil
}

func (s *Service) GetUserShoppingLists(ctx context.Context, userID uuid.UUID) ([]*ShoppingList, error) {
	ctx, span := tracer.Start(ctx, "shopping.GetUserShoppingLists")
	defer span.End()

	query := `
		SELECT DISTINCT sl.id, sl.name, sl.description, sl.owner_id, sl.family_id, sl.is_shared, sl.created_at, sl.updated_at
		FROM shopping_lists sl
		LEFT JOIN family_members fm ON sl.family_id = fm.family_id
		WHERE sl.owner_id = $1 
		   OR sl.is_shared = true 
		   OR (sl.family_id IS NOT NULL AND fm.user_id = $1)
		ORDER BY sl.created_at DESC
	`

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get user shopping lists: %w", err)
	}
	defer rows.Close()

	var lists []*ShoppingList
	for rows.Next() {
		var list ShoppingList
		err := rows.Scan(
			&list.ID,
			&list.Name,
			&list.Description,
			&list.OwnerID,
			&list.FamilyID,
			&list.IsShared,
			&list.CreatedAt,
			&list.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan shopping list: %w", err)
		}
		lists = append(lists, &list)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over shopping lists: %w", err)
	}

	return lists, nil
}

// ShoppingListWithFamily represents a shopping list with optional family information
type ShoppingListWithFamily struct {
	*ShoppingList
	FamilyName *string `json:"family_name,omitempty"`
}

// GetUserShoppingListsWithFamilies returns shopping lists with family names and user family count
func (s *Service) GetUserShoppingListsWithFamilies(ctx context.Context, userID uuid.UUID) ([]*ShoppingListWithFamily, int, error) {
	ctx, span := tracer.Start(ctx, "shopping.GetUserShoppingListsWithFamilies")
	defer span.End()

	// First, get the user's family count
	familyCountQuery := `
		SELECT COUNT(DISTINCT f.id)
		FROM families f
		JOIN family_members fm ON f.id = fm.family_id
		WHERE fm.user_id = $1
	`
	
	var familyCount int
	err := s.db.QueryRow(ctx, familyCountQuery, userID).Scan(&familyCount)
	if err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("failed to get user family count: %w", err)
	}

	// Get shopping lists with family names
	query := `
		SELECT DISTINCT sl.id, sl.name, sl.description, sl.owner_id, sl.family_id, sl.is_shared, 
		       sl.created_at, sl.updated_at, f.name as family_name
		FROM shopping_lists sl
		LEFT JOIN family_members fm ON sl.family_id = fm.family_id
		LEFT JOIN families f ON sl.family_id = f.id
		WHERE sl.owner_id = $1 
		   OR sl.is_shared = true 
		   OR (sl.family_id IS NOT NULL AND fm.user_id = $1)
		ORDER BY sl.created_at DESC
	`

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("failed to get user shopping lists with families: %w", err)
	}
	defer rows.Close()

	var lists []*ShoppingListWithFamily
	for rows.Next() {
		var list ShoppingList
		var familyName *string
		
		err := rows.Scan(
			&list.ID,
			&list.Name,
			&list.Description,
			&list.OwnerID,
			&list.FamilyID,
			&list.IsShared,
			&list.CreatedAt,
			&list.UpdatedAt,
			&familyName,
		)
		if err != nil {
			span.RecordError(err)
			return nil, 0, fmt.Errorf("failed to scan shopping list with family: %w", err)
		}
		
		listWithFamily := &ShoppingListWithFamily{
			ShoppingList: &list,
			FamilyName:   familyName,
		}
		lists = append(lists, listWithFamily)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("error iterating over shopping lists with families: %w", err)
	}

	return lists, familyCount, nil
}

func (s *Service) GetShoppingListByID(ctx context.Context, listID uuid.UUID) (*ShoppingList, error) {
	ctx, span := tracer.Start(ctx, "shopping.GetShoppingListByID")
	defer span.End()

	query := `
		SELECT id, name, description, owner_id, family_id, is_shared, created_at, updated_at
		FROM shopping_lists 
		WHERE id = $1
	`

	var list ShoppingList
	err := s.db.QueryRow(ctx, query, listID).Scan(
		&list.ID,
		&list.Name,
		&list.Description,
		&list.OwnerID,
		&list.FamilyID,
		&list.IsShared,
		&list.CreatedAt,
		&list.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get shopping list by ID %d: %w", listID, err)
	}

	return &list, nil
}

func (s *Service) AddItemToList(ctx context.Context, listID uuid.UUID, name, quantity string, addedBy uuid.UUID) (*ShoppingItem, error) {
	return s.AddItemToListWithLanguage(ctx, listID, name, quantity, "en", addedBy)
}

func (s *Service) AddItemToListWithLanguage(ctx context.Context, listID uuid.UUID, name, quantity, languageCode string, addedBy uuid.UUID) (*ShoppingItem, error) {
	ctx, span := tracer.Start(ctx, "shopping.AddItemToListWithLanguage")
	defer span.End()

	// Prepare the raw text for AI parsing
	rawText := name
	if quantity != "" {
		rawText = fmt.Sprintf("%s (%s)", name, quantity)
	}

	// Set initial display name and parsing status
	displayName := rawText
	parsingStatus := "pending"
	var parsedName *string

	// Attempt AI parsing if AI service is available
	if s.aiService != nil {
		parsedResult, err := s.aiService.GetOrCreateParsedItem(ctx, rawText, languageCode, addedBy)
		if err != nil {
			span.RecordError(err)
			// Log error but continue with original name
			parsingStatus = "failed"
		} else {
			parsingStatus = "parsed"
			parsedName = &parsedResult.StandardizedName
		}
	}

	// Insert the item with AI parsing data
	query := `
		INSERT INTO shopping_items (list_id, name, quantity, is_completed, added_by, 
		                           display_name, parsed_name, parsing_status, created_at, updated_at)
		VALUES ($1, $2, $3, false, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, list_id, name, quantity, is_completed, added_by, completed_by, completed_at,
		         original_item_id, parsed_item_id, display_name, parsed_name, parsing_status,
		         created_at, updated_at
	`

	var item ShoppingItem
	err := s.db.QueryRow(ctx, query, listID, name, quantity, addedBy, displayName, parsedName, parsingStatus).Scan(
		&item.ID,
		&item.ListID,
		&item.Name,
		&item.Quantity,
		&item.IsCompleted,
		&item.AddedBy,
		&item.CompletedBy,
		&item.CompletedAt,
		&item.OriginalItemID,
		&item.ParsedItemID,
		&item.DisplayName,
		&item.ParsedName,
		&item.ParsingStatus,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to add item to list: %w", err)
	}

	return &item, nil
}

func (s *Service) GetListItems(ctx context.Context, listID uuid.UUID) ([]*ShoppingItem, error) {
	ctx, span := tracer.Start(ctx, "shopping.GetListItems")
	defer span.End()

	query := `
		SELECT id, list_id, name, quantity, is_completed, added_by, completed_by, completed_at,
		       original_item_id, parsed_item_id, display_name, parsed_name, parsing_status,
		       created_at, updated_at
		FROM shopping_items 
		WHERE list_id = $1
		ORDER BY is_completed ASC, created_at ASC
	`

	rows, err := s.db.Query(ctx, query, listID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get list items: %w", err)
	}
	defer rows.Close()

	var items []*ShoppingItem
	for rows.Next() {
		var item ShoppingItem
		err := rows.Scan(
			&item.ID,
			&item.ListID,
			&item.Name,
			&item.Quantity,
			&item.IsCompleted,
			&item.AddedBy,
			&item.CompletedBy,
			&item.CompletedAt,
			&item.OriginalItemID,
			&item.ParsedItemID,
			&item.DisplayName,
			&item.ParsedName,
			&item.ParsingStatus,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan shopping item: %w", err)
		}
		items = append(items, &item)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over shopping items: %w", err)
	}

	return items, nil
}

func (s *Service) CompleteItem(ctx context.Context, itemID uuid.UUID, completedBy uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "shopping.CompleteItem")
	defer span.End()

	query := `
		UPDATE shopping_items 
		SET is_completed = true, completed_by = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`

	result, err := s.db.Exec(ctx, query, itemID, completedBy)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to complete item %d: %w", itemID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("item with ID %d not found", itemID)
	}

	return nil
}

func (s *Service) UncompleteItem(ctx context.Context, itemID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "shopping.UncompleteItem")
	defer span.End()

	query := `
		UPDATE shopping_items 
		SET is_completed = false, completed_by = NULL, completed_at = NULL, updated_at = NOW()
		WHERE id = $1
	`

	result, err := s.db.Exec(ctx, query, itemID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to uncomplete item %d: %w", itemID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("item with ID %d not found", itemID)
	}

	return nil
}

func (s *Service) DeleteItem(ctx context.Context, itemID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "shopping.DeleteItem")
	defer span.End()

	query := `DELETE FROM shopping_items WHERE id = $1`

	result, err := s.db.Exec(ctx, query, itemID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete item %d: %w", itemID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("item with ID %d not found", itemID)
	}

	return nil
}

func (s *Service) DeleteShoppingList(ctx context.Context, listID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "shopping.DeleteShoppingList")
	defer span.End()

	// Start transaction to delete both items and list
	tx, err := s.db.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete all items first
	_, err = tx.Exec(ctx, "DELETE FROM shopping_items WHERE list_id = $1", listID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete shopping items: %w", err)
	}

	// Delete the list
	result, err := tx.Exec(ctx, "DELETE FROM shopping_lists WHERE id = $1", listID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete shopping list: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("shopping list with ID %d not found", listID)
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetFamilyShoppingLists returns all shopping lists for a specific family
func (s *Service) GetFamilyShoppingLists(ctx context.Context, familyID uuid.UUID) ([]*ShoppingList, error) {
	ctx, span := tracer.Start(ctx, "shopping.GetFamilyShoppingLists")
	defer span.End()

	query := `
		SELECT id, name, description, owner_id, family_id, is_shared, created_at, updated_at
		FROM shopping_lists 
		WHERE family_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(ctx, query, familyID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get family shopping lists: %w", err)
	}
	defer rows.Close()

	var lists []*ShoppingList
	for rows.Next() {
		var list ShoppingList
		err := rows.Scan(
			&list.ID,
			&list.Name,
			&list.Description,
			&list.OwnerID,
			&list.FamilyID,
			&list.IsShared,
			&list.CreatedAt,
			&list.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan shopping list: %w", err)
		}
		lists = append(lists, &list)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over shopping lists: %w", err)
	}

	return lists, nil
}

// CanUserAccessList checks if a user can access a shopping list based on ownership, family membership, or sharing
func (s *Service) CanUserAccessList(ctx context.Context, listID, userID uuid.UUID) (bool, error) {
	ctx, span := tracer.Start(ctx, "shopping.CanUserAccessList")
	defer span.End()

	query := `
		SELECT COUNT(*) > 0
		FROM shopping_lists sl
		LEFT JOIN family_members fm ON sl.family_id = fm.family_id
		WHERE sl.id = $1 AND (
			sl.owner_id = $2 OR 
			sl.is_shared = true OR
			(sl.family_id IS NOT NULL AND fm.user_id = $2)
		)
	`

	var canAccess bool
	err := s.db.QueryRow(ctx, query, listID, userID).Scan(&canAccess)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("failed to check list access: %w", err)
	}

	return canAccess, nil
}

// AddItemsToListWithAI uses AI to intelligently parse and add multiple items to a shopping list
func (s *Service) AddItemsToListWithAI(ctx context.Context, listID uuid.UUID, rawText, languageCode string, addedBy uuid.UUID) ([]*ShoppingItem, []string, error) {
	ctx, span := tracer.Start(ctx, "shopping.AddItemsToListWithAI")
	defer span.End()

	var addedItems []*ShoppingItem
	var failedItems []string

	// Always use AI service to intelligently parse items (single or multiple)
	if s.aiService != nil {
		parsedResults, err := s.aiService.ParseAndStoreItems(ctx, rawText, languageCode, addedBy)
		if err != nil {
			s.logger.Error("AI parsing failed", "error", err)
			return nil, []string{rawText}, fmt.Errorf("failed to parse items with AI: %w", err)
		}

		// Add each parsed item to the list
		for _, parsedResult := range parsedResults {
			// Use the standardized name as display name (this preserves original language)
			// The StandardizedName should contain the clean version in original language
			itemName := parsedResult.StandardizedName
			displayName := parsedResult.StandardizedName // This should be in original language (молоко, not milk)
			quantityStr := ""
			if parsedResult.QuantityValue != nil {
				// Include quantities even if they are 0 (e.g., "морковка 0,5 кг")
				value := *parsedResult.QuantityValue
				var valueStr string
				if value == float64(int(value)) {
					// Whole number, don't show decimal
					valueStr = fmt.Sprintf("%.0f", value)
				} else {
					// Decimal number, show one decimal place
					valueStr = fmt.Sprintf("%.1f", value)
				}
				
				if parsedResult.QuantityUnit != "" {
					quantityStr = fmt.Sprintf("%s %s", valueStr, parsedResult.QuantityUnit)
				} else {
					quantityStr = valueStr
				}
			}

			// Create shopping item with AI parsing data
			query := `
				INSERT INTO shopping_items (list_id, name, quantity, is_completed, added_by, 
				                           original_item_id, parsed_item_id, display_name, parsed_name, parsing_status, created_at, updated_at)
				VALUES ($1, $2, $3, false, $4, $5, $6, $7, $8, 'parsed', NOW(), NOW())
				RETURNING id, list_id, name, quantity, is_completed, added_by, completed_by, completed_at,
				         original_item_id, parsed_item_id, display_name, parsed_name, parsing_status,
				         created_at, updated_at
			`

			var item ShoppingItem
			parsedName := parsedResult.StandardizedName // Use same for buttons
			
			// For quantity, use empty string if no quantity
			var quantityParam *string
			if quantityStr != "" {
				quantityParam = &quantityStr
			}

			err := s.db.QueryRow(ctx, query, listID, itemName, quantityParam, addedBy, 
				parsedResult.OriginalItemID, parsedResult.ParsedItemID, displayName, parsedName).Scan(
				&item.ID, &item.ListID, &item.Name, &item.Quantity, &item.IsCompleted,
				&item.AddedBy, &item.CompletedBy, &item.CompletedAt,
				&item.OriginalItemID, &item.ParsedItemID, &item.DisplayName, &item.ParsedName, &item.ParsingStatus,
				&item.CreatedAt, &item.UpdatedAt,
			)

			if err != nil {
				s.logger.Error("Failed to insert parsed shopping item", "error", err, "item", itemName)
				failedItems = append(failedItems, itemName)
				continue
			}

			addedItems = append(addedItems, &item)
			s.logger.Info("Successfully added AI-parsed item", 
				"original", itemName,
				"parsed", parsedResult.StandardizedName,
				"category", parsedResult.Category,
				"confidence", parsedResult.ConfidenceScore)
		}
	} else {
		// AI service is required for parsing
		return nil, []string{rawText}, fmt.Errorf("AI service is not available")
	}

	return addedItems, failedItems, nil
}
