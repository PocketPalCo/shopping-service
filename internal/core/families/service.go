package families

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("families-service")

type Family struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description *string   `json:"description" db:"description"`
	CreatedBy   uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type FamilyMember struct {
	ID       uuid.UUID `json:"id" db:"id"`
	FamilyID uuid.UUID `json:"family_id" db:"family_id"`
	UserID   uuid.UUID `json:"user_id" db:"user_id"`
	Role     string    `json:"role" db:"role"` // "admin", "member"
	AddedBy  uuid.UUID `json:"added_by" db:"added_by"`
	AddedAt  time.Time `json:"added_at" db:"added_at"`
}

type FamilyWithMembers struct {
	Family  Family         `json:"family"`
	Members []FamilyMember `json:"members"`
}

type CreateFamilyRequest struct {
	Name        string
	Description *string
	CreatedBy   uuid.UUID
	MemberIDs   []uuid.UUID
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		db: db,
	}
}

func (s *Service) CreateFamily(ctx context.Context, req CreateFamilyRequest) (*Family, error) {
	ctx, span := tracer.Start(ctx, "families.CreateFamily")
	defer span.End()

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create family
	familyQuery := `
		INSERT INTO families (name, description, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, name, description, created_by, created_at, updated_at
	`

	var family Family
	err = tx.QueryRow(ctx, familyQuery, req.Name, req.Description, req.CreatedBy).Scan(
		&family.ID,
		&family.Name,
		&family.Description,
		&family.CreatedBy,
		&family.CreatedAt,
		&family.UpdatedAt,
	)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create family: %w", err)
	}

	// Add creator as admin member
	_, err = tx.Exec(ctx, `
		INSERT INTO family_members (family_id, user_id, role, added_by, added_at)
		VALUES ($1, $2, 'admin', $2, NOW())
	`, family.ID, req.CreatedBy)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to add creator as admin: %w", err)
	}

	// Add other members
	for _, userID := range req.MemberIDs {
		if userID != req.CreatedBy { // Don't add creator twice
			_, err = tx.Exec(ctx, `
				INSERT INTO family_members (family_id, user_id, role, added_by, added_at)
				VALUES ($1, $2, 'member', $3, NOW())
			`, family.ID, userID, req.CreatedBy)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("failed to add member %s: %w", userID, err)
			}
		}
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &family, nil
}

func (s *Service) GetUserFamilies(ctx context.Context, userID uuid.UUID) ([]*Family, error) {
	ctx, span := tracer.Start(ctx, "families.GetUserFamilies")
	defer span.End()

	query := `
		SELECT f.id, f.name, f.description, f.created_by, f.created_at, f.updated_at
		FROM families f
		INNER JOIN family_members fm ON f.id = fm.family_id
		WHERE fm.user_id = $1
		ORDER BY f.created_at DESC
	`

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get user families: %w", err)
	}
	defer rows.Close()

	var families []*Family
	for rows.Next() {
		var family Family
		err := rows.Scan(
			&family.ID,
			&family.Name,
			&family.Description,
			&family.CreatedBy,
			&family.CreatedAt,
			&family.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan family: %w", err)
		}
		families = append(families, &family)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over families: %w", err)
	}

	return families, nil
}

func (s *Service) GetFamilyWithMembers(ctx context.Context, familyID uuid.UUID) (*FamilyWithMembers, error) {
	ctx, span := tracer.Start(ctx, "families.GetFamilyWithMembers")
	defer span.End()

	// Get family
	familyQuery := `
		SELECT id, name, description, created_by, created_at, updated_at
		FROM families
		WHERE id = $1
	`

	var family Family
	err := s.db.QueryRow(ctx, familyQuery, familyID).Scan(
		&family.ID,
		&family.Name,
		&family.Description,
		&family.CreatedBy,
		&family.CreatedAt,
		&family.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get family: %w", err)
	}

	// Get members
	membersQuery := `
		SELECT id, family_id, user_id, role, added_by, added_at
		FROM family_members
		WHERE family_id = $1
		ORDER BY added_at ASC
	`

	rows, err := s.db.Query(ctx, membersQuery, familyID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get family members: %w", err)
	}
	defer rows.Close()

	var members []FamilyMember
	for rows.Next() {
		var member FamilyMember
		err := rows.Scan(
			&member.ID,
			&member.FamilyID,
			&member.UserID,
			&member.Role,
			&member.AddedBy,
			&member.AddedAt,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan family member: %w", err)
		}
		members = append(members, member)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over family members: %w", err)
	}

	return &FamilyWithMembers{
		Family:  family,
		Members: members,
	}, nil
}

func (s *Service) AddMemberToFamily(ctx context.Context, familyID, userID, addedBy uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "families.AddMemberToFamily")
	defer span.End()

	// Check if user is already a member
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM family_members WHERE family_id = $1 AND user_id = $2)`
	err := s.db.QueryRow(ctx, checkQuery, familyID, userID).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to check membership: %w", err)
	}
	if exists {
		return fmt.Errorf("user is already a member of this family")
	}

	// Add member
	insertQuery := `
		INSERT INTO family_members (family_id, user_id, role, added_by, added_at)
		VALUES ($1, $2, 'member', $3, NOW())
	`

	_, err = s.db.Exec(ctx, insertQuery, familyID, userID, addedBy)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to add member to family: %w", err)
	}

	return nil
}

func (s *Service) RemoveMemberFromFamily(ctx context.Context, familyID, userID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "families.RemoveMemberFromFamily")
	defer span.End()

	query := `DELETE FROM family_members WHERE family_id = $1 AND user_id = $2`

	result, err := s.db.Exec(ctx, query, familyID, userID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to remove member from family: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("member not found in family")
	}

	return nil
}

func (s *Service) IsUserFamilyAdmin(ctx context.Context, familyID, userID uuid.UUID) (bool, error) {
	ctx, span := tracer.Start(ctx, "families.IsUserFamilyAdmin")
	defer span.End()

	var role string
	query := `SELECT role FROM family_members WHERE family_id = $1 AND user_id = $2`

	err := s.db.QueryRow(ctx, query, familyID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("failed to check user role: %w", err)
	}

	return role == "admin", nil
}

func (s *Service) IsUserFamilyMember(ctx context.Context, familyID, userID uuid.UUID) (bool, error) {
	ctx, span := tracer.Start(ctx, "families.IsUserFamilyMember")
	defer span.End()

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM family_members WHERE family_id = $1 AND user_id = $2)`

	err := s.db.QueryRow(ctx, query, familyID, userID).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("failed to check membership: %w", err)
	}

	return exists, nil
}

func (s *Service) DeleteFamily(ctx context.Context, familyID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "families.DeleteFamily")
	defer span.End()

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete all members first
	_, err = tx.Exec(ctx, "DELETE FROM family_members WHERE family_id = $1", familyID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete family members: %w", err)
	}

	// Delete the family
	result, err := tx.Exec(ctx, "DELETE FROM families WHERE id = $1", familyID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete family: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("family not found")
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetFamilyByID retrieves a family by its ID
func (s *Service) GetFamilyByID(ctx context.Context, familyID uuid.UUID) (*Family, error) {
	ctx, span := tracer.Start(ctx, "families.GetFamilyByID")
	defer span.End()

	query := `
		SELECT id, name, description, created_by, created_at, updated_at 
		FROM families 
		WHERE id = $1`

	var family Family
	err := s.db.QueryRow(ctx, query, familyID).Scan(
		&family.ID,
		&family.Name,
		&family.Description,
		&family.CreatedBy,
		&family.CreatedAt,
		&family.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get family by ID: %w", err)
	}

	return &family, nil
}

// GetFamilyMemberUserIDs retrieves user IDs of all family members except the specified user
func (s *Service) GetFamilyMemberUserIDs(ctx context.Context, familyID, excludeUserID uuid.UUID) ([]uuid.UUID, error) {
	ctx, span := tracer.Start(ctx, "families.GetFamilyMemberUserIDs")
	defer span.End()

	query := `
		SELECT user_id
		FROM family_members
		WHERE family_id = $1 AND user_id != $2
		ORDER BY added_at ASC
	`

	rows, err := s.db.Query(ctx, query, familyID, excludeUserID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get family member user IDs: %w", err)
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var userID uuid.UUID
		err := rows.Scan(&userID)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan user ID: %w", err)
		}
		userIDs = append(userIDs, userID)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating over user IDs: %w", err)
	}

	return userIDs, nil
}
