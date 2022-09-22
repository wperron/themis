package themis

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
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

func (ct ClaimType) String() string {
	switch ct {
	case CLAIM_TYPE_AREA:
		return "Area"
	case CLAIM_TYPE_REGION:
		return "Region"
	case CLAIM_TYPE_TRADE:
		return "Trade Node"
	}
	return ""
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
	Conflicts []Conflict
}

func (ec ErrConflict) Error() string {
	return fmt.Sprintf("found %d conflicting provinces", len(ec.Conflicts))
}

type Store struct {
	db *sql.DB
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

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Claim(ctx context.Context, userId, player, province string, claimType ClaimType) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Commit() //nolint:errcheck

	conflicts, err := s.FindConflicts(ctx, userId, province, claimType)
	if err != nil {
		return 0, fmt.Errorf("failed to run conflicts check: %w", err)
	}

	if len(conflicts) > 0 {
		return 0, ErrConflict{Conflicts: conflicts}
	}

	// check that provided name matches the claim type
	stmt, err := s.db.PrepareContext(ctx, fmt.Sprintf(`SELECT COUNT(1) FROM provinces WHERE LOWER(provinces.%s) = ?`, claimTypeToColumn[claimType]))
	if err != nil {
		return 0, fmt.Errorf("failed to prepare count query: %w", err)
	}

	row := stmt.QueryRowContext(ctx, strings.ToLower(province))
	var count int
	err = row.Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to scan: %w", err)
	}

	if count == 0 {
		return 0, fmt.Errorf("found no provinces for %s named %s", claimType, province)
	}

	stmt, err = s.db.PrepareContext(ctx, "INSERT INTO claims (player, claim_type, val, userid) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, fmt.Errorf("failed to prepare claim query: %w", err)
	}

	res, err := stmt.ExecContext(ctx, player, claimType, province, userId)
	if err != nil {
		return 0, fmt.Errorf("failed to insert claim: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last ID: %w", err)
	}

	return int(id), nil
}

func (s *Store) ListAvailability(ctx context.Context, claimType ClaimType, search ...string) ([]string, error) {
	queryParams := []any{string(claimType)}
	queryPattern := `SELECT DISTINCT(provinces.%[1]s)
	FROM provinces LEFT JOIN claims ON provinces.%[1]s = claims.val AND claims.claim_type = ?
	WHERE claims.val IS NULL
	AND provinces.typ = 'Land'`
	if len(search) > 0 && search[0] != "" {
		// only take one search param, ignore the rest
		queryPattern += `AND provinces.%[1]s LIKE ?`
		queryParams = append(queryParams, fmt.Sprintf("%%%s%%", search[0]))
	}

	stmt, err := s.db.PrepareContext(ctx, fmt.Sprintf(queryPattern, claimTypeToColumn[claimType]))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, queryParams...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	avail := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("failed to scan rows: %w", err)
		}
		avail = append(avail, s)
	}

	return avail, nil
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
	if err == sql.ErrNoRows {
		return ClaimDetail{}, ErrNoSuchClaim
	}
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

var ErrNoSuchClaim = errors.New("no such claim")

func (s *Store) DeleteClaim(ctx context.Context, ID int, userId string) error {
	stmt, err := s.db.PrepareContext(ctx, "DELETE FROM claims WHERE id = ? AND userid = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	res, err := stmt.ExecContext(ctx, ID, userId)
	if err != nil {
		return fmt.Errorf("failed to delete claim ID %d: %w", ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if rows == 0 {
		return ErrNoSuchClaim
	}
	return nil
}

func (s *Store) Flush(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM claims;")
	if err != nil {
		return fmt.Errorf("failed to execute delete query: %w", err)
	}
	return nil
}
