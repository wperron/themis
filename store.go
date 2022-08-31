package themis

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/init.sql
var initScript string

type ClaimType string

func ClaimTypeFromString(s string) (ClaimType, error) {
	switch s {
	case CLAIM_TYPE_AREA:
		return CLAIM_TYPE_AREA, nil
	case CLAIM_TYPE_REGION:
		return CLAIM_TYPE_REGION, nil
	case CLAIM_TYPE_TRADE:
		return CLAIM_TYPE_TRADE, nil
	}
	return "", fmt.Errorf("no claim type matching '%s'", s)
}

const (
	CLAIM_TYPE_AREA   = "area"
	CLAIM_TYPE_REGION = "region"
	CLAIM_TYPE_TRADE  = "trade"
)

var claimTypeToColumn = map[ClaimType]string{
	CLAIM_TYPE_AREA:   "area",
	CLAIM_TYPE_REGION: "region",
	CLAIM_TYPE_TRADE:  "trade_node",
}

type Store struct {
	db *sql.DB
}

type Claim struct {
	ID     int
	Player string
	Name   string
	Type   ClaimType
}

func (c Claim) String() string {
	return fmt.Sprintf("id=%d player=%s claim_type=%s name=%s", c.ID, c.Player, c.Type, c.Name)
}

type ErrConflict struct {
	Conflicts []string
}

func (ec ErrConflict) Error() string {
	return fmt.Sprintf("found conflicting provinces: %s", strings.Join(ec.Conflicts, ", "))
}

func NewStore(conn string) (*Store, error) {
	db, err := sql.Open("sqlite3", conn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	_, err = db.Exec(initScript)
	if err != nil {
		return nil, fmt.Errorf("failed to run init script: %w", err)
	}

	return &Store{
		db: db,
	}, nil
}

func (s *Store) Claim(ctx context.Context, player, province string, claimType ClaimType) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Commit()

	// Check conflicts
	stmt, err := s.db.PrepareContext(ctx, fmt.Sprintf(`SELECT provinces.name FROM provinces WHERE provinces.%s = ? and provinces.name in (
	SELECT provinces.name FROM claims LEFT JOIN provinces ON claims.val = provinces.trade_node WHERE claims.claim_type = 'trade'
	UNION SELECT provinces.name from claims LEFT JOIN provinces ON claims.val = provinces.region WHERE claims.claim_type = 'region'
	UNION SELECT provinces.name from claims LEFT JOIN provinces ON claims.val = provinces.area WHERE claims.claim_type = 'area'
)`, claimTypeToColumn[claimType]))
	if err != nil {
		return fmt.Errorf("failed to prepare conflicts query: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, province)
	if err != nil {
		return fmt.Errorf("failed to get conflicting provinces: %w", err)
	}

	conflicts := make([]string, 0)
	for rows.Next() {
		var p string
		err = rows.Scan(&p)
		if err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}
		conflicts = append(conflicts, p)
	}

	if len(conflicts) > 0 {
		return ErrConflict{Conflicts: conflicts}
	}

	stmt, err = s.db.PrepareContext(ctx, "INSERT INTO claims (player, claim_type, val) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare claim query: %w", err)
	}

	_, err = stmt.ExecContext(ctx, player, claimType, province)
	if err != nil {
		return fmt.Errorf("failed to insert claim: %w", err)
	}

	return nil
}

func (s *Store) ListClaims(ctx context.Context) ([]Claim, error) {
	stmt, err := s.db.PrepareContext(ctx, `SELECT id, player, claim_type, val FROM claims`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	claims := make([]Claim, 0)
	for rows.Next() {
		c := Claim{}
		var rawType string
		err = rows.Scan(&c.ID, &c.Player, &rawType, &c.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		cl, err := ClaimTypeFromString(rawType)
		if err != nil {
			return nil, fmt.Errorf("unexpected error converting raw claim type: %w", err)
		}
		c.Type = cl

		claims = append(claims, c)
	}

	return claims, nil
}

type ClaimDetail struct {
	Claim
	Provinces []string
}

func (cd ClaimDetail) String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("%s\n", cd.Claim))
	for _, p := range cd.Provinces {
		sb.WriteString(fmt.Sprintf("  - %s\n", p))
	}
	return sb.String()
}

func (s *Store) DescribeClaim(ctx context.Context, ID int) (ClaimDetail, error) {
	stmt, err := s.db.PrepareContext(ctx, `SELECT id, player, claim_type, val FROM claims WHERE id = ?`)
	if err != nil {
		return ClaimDetail{}, fmt.Errorf("failed to get claim: %w", err)
	}

	row := stmt.QueryRowContext(ctx, ID)

	c := Claim{}
	var rawType string
	err = row.Scan(&c.ID, &c.Player, &rawType, &c.Name)
	if err != nil {
		return ClaimDetail{}, fmt.Errorf("failed to scan row: %w", err)
	}
	cl, err := ClaimTypeFromString(rawType)
	if err != nil {
		return ClaimDetail{}, fmt.Errorf("unexpected error converting raw claim type: %w", err)
	}
	c.Type = cl

	stmt, err = s.db.PrepareContext(ctx, fmt.Sprintf(`SELECT name FROM provinces where provinces.%s = ?`, claimTypeToColumn[cl]))
	if err != nil {
		return ClaimDetail{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, c.Name)
	if err != nil {
		return ClaimDetail{}, fmt.Errorf("failed to execute query: %w", err)
	}

	provinces := make([]string, 0)
	for rows.Next() {
		var p string
		err = rows.Scan(&p)
		if err != nil {
			return ClaimDetail{}, fmt.Errorf("failed to scan result set: %w", err)
		}
		provinces = append(provinces, p)
	}

	return ClaimDetail{
		Claim:     c,
		Provinces: provinces,
	}, nil
}
