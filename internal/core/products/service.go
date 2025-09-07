package products

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("products-service")

type Product struct {
	ID          uuid.UUID `json:"id" db:"id"`
	NameEn      string    `json:"name_en" db:"name_en"`
	NameRu      string    `json:"name_ru" db:"name_ru"`
	NameUk      string    `json:"name_uk" db:"name_uk"`
	Category    string    `json:"category" db:"category"`
	Subcategory string    `json:"subcategory" db:"subcategory"`
	Aliases     []string  `json:"aliases" db:"aliases"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Service struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewService(db *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// GetAllProducts retrieves all products from the database for prompt building
func (s *Service) GetAllProducts(ctx context.Context) ([]*Product, error) {
	ctx, span := tracer.Start(ctx, "products.GetAllProducts")
	defer span.End()

	query := `
		SELECT id, name_en, name_ru, name_uk, category, subcategory, aliases, created_at, updated_at
		FROM products
		ORDER BY category, subcategory, name_en
	`

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []*Product
	for rows.Next() {
		var product Product
		err := rows.Scan(
			&product.ID,
			&product.NameEn,
			&product.NameRu,
			&product.NameUk,
			&product.Category,
			&product.Subcategory,
			&product.Aliases,
			&product.CreatedAt,
			&product.UpdatedAt,
		)
		if err != nil {
			s.logger.Error("Failed to scan product row", "error", err)
			continue
		}
		products = append(products, &product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating products: %w", err)
	}

	s.logger.Debug("Retrieved products from database", "count", len(products))
	return products, nil
}

// GetProductByID retrieves a specific product by ID
func (s *Service) GetProductByID(ctx context.Context, id uuid.UUID) (*Product, error) {
	ctx, span := tracer.Start(ctx, "products.GetProductByID")
	defer span.End()

	query := `
		SELECT id, name_en, name_ru, name_uk, category, subcategory, aliases, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	var product Product
	err := s.db.QueryRow(ctx, query, id).Scan(
		&product.ID,
		&product.NameEn,
		&product.NameRu,
		&product.NameUk,
		&product.Category,
		&product.Subcategory,
		&product.Aliases,
		&product.CreatedAt,
		&product.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	return &product, nil
}

// GetProductsByCategory retrieves all products in a specific category
func (s *Service) GetProductsByCategory(ctx context.Context, category string) ([]*Product, error) {
	ctx, span := tracer.Start(ctx, "products.GetProductsByCategory")
	defer span.End()

	query := `
		SELECT id, name_en, name_ru, name_uk, category, subcategory, aliases, created_at, updated_at
		FROM products
		WHERE category = $1
		ORDER BY subcategory, name_en
	`

	rows, err := s.db.Query(ctx, query, category)
	if err != nil {
		return nil, fmt.Errorf("failed to query products by category: %w", err)
	}
	defer rows.Close()

	var products []*Product
	for rows.Next() {
		var product Product
		err := rows.Scan(
			&product.ID,
			&product.NameEn,
			&product.NameRu,
			&product.NameUk,
			&product.Category,
			&product.Subcategory,
			&product.Aliases,
			&product.CreatedAt,
			&product.UpdatedAt,
		)
		if err != nil {
			s.logger.Error("Failed to scan product row", "error", err)
			continue
		}
		products = append(products, &product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating products: %w", err)
	}

	return products, nil
}

// SearchProducts searches for products by name or alias
func (s *Service) SearchProducts(ctx context.Context, searchTerm string, languageCode string) ([]*Product, error) {
	ctx, span := tracer.Start(ctx, "products.SearchProducts")
	defer span.End()

	var query string
	var args []interface{}

	switch languageCode {
	case "ru", "russian":
		query = `
			SELECT id, name_en, name_ru, name_uk, category, subcategory, aliases, created_at, updated_at
			FROM products
			WHERE name_ru ILIKE $1 OR $2 = ANY(aliases)
			ORDER BY 
				CASE 
					WHEN name_ru ILIKE $3 THEN 1
					WHEN name_ru ILIKE $4 THEN 2
					ELSE 3
				END,
				name_ru
		`
		args = []interface{}{
			"%" + searchTerm + "%",
			searchTerm,
			searchTerm,       // exact match
			searchTerm + "%", // starts with
		}
	case "uk", "ukrainian":
		query = `
			SELECT id, name_en, name_ru, name_uk, category, subcategory, aliases, created_at, updated_at
			FROM products
			WHERE name_uk ILIKE $1 OR $2 = ANY(aliases)
			ORDER BY 
				CASE 
					WHEN name_uk ILIKE $3 THEN 1
					WHEN name_uk ILIKE $4 THEN 2
					ELSE 3
				END,
				name_uk
		`
		args = []interface{}{
			"%" + searchTerm + "%",
			searchTerm,
			searchTerm,       // exact match
			searchTerm + "%", // starts with
		}
	default: // English
		query = `
			SELECT id, name_en, name_ru, name_uk, category, subcategory, aliases, created_at, updated_at
			FROM products
			WHERE name_en ILIKE $1 OR $2 = ANY(aliases)
			ORDER BY 
				CASE 
					WHEN name_en ILIKE $3 THEN 1
					WHEN name_en ILIKE $4 THEN 2
					ELSE 3
				END,
				name_en
		`
		args = []interface{}{
			"%" + searchTerm + "%",
			searchTerm,
			searchTerm,       // exact match
			searchTerm + "%", // starts with
		}
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search products: %w", err)
	}
	defer rows.Close()

	var products []*Product
	for rows.Next() {
		var product Product
		err := rows.Scan(
			&product.ID,
			&product.NameEn,
			&product.NameRu,
			&product.NameUk,
			&product.Category,
			&product.Subcategory,
			&product.Aliases,
			&product.CreatedAt,
			&product.UpdatedAt,
		)
		if err != nil {
			s.logger.Error("Failed to scan product row in search", "error", err)
			continue
		}
		products = append(products, &product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return products, nil
}
