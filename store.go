package themis

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/init.sql
var initScript string

const (
	CLAIM_TYPE_AREA = iota
	CLAIM_TYPE_REGION
	CLAIM_TYPE_TRADE
)

var claimTypeEnum = map[string]int{
	"area":   CLAIM_TYPE_AREA,
	"region": CLAIM_TYPE_REGION,
	"trade":  CLAIM_TYPE_TRADE,
}

var claimTypeEnumVals = map[int]string{
	CLAIM_TYPE_AREA:   "area",
	CLAIM_TYPE_REGION: "region",
	CLAIM_TYPE_TRADE:  "trade",
}

var claimTypeToColumn = map[string]string{
	"area":   "area",
	"region": "region",
	"trade":  "trade_node",
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

func (s *Store) Claim(ctx context.Context, player, province string, typ int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Commit()

	// Check conflicts
	stmt, err := s.db.PrepareContext(ctx, fmt.Sprintf(`SELECT COUNT(1) FROM provinces WHERE provinces.%s = ? and provinces.name in (
	SELECT provinces.name FROM claims LEFT JOIN provinces ON claims.val = provinces.trade_node WHERE claims.claim_type = 'trade'
	UNION SELECT provinces.name from claims LEFT JOIN provinces ON claims.val = provinces.region WHERE claims.claim_type = 'region'
	UNION SELECT provinces.name from claims LEFT JOIN provinces ON claims.val = provinces.area WHERE claims.claim_type = 'area'
)`, claimTypeToColumn[claimTypeEnumVals[typ]]))
	if err != nil {
		return fmt.Errorf("failed to prepare conflicts query: %w", err)
	}

	row := stmt.QueryRowContext(ctx, province)
	var count int
	err = row.Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to get count of conflicting provinces: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("found %d conflicting provinces", count)
	}

	stmt, err = s.db.PrepareContext(ctx, "INSERT INTO claims (player, claim_type, val) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare claim query: %w", err)
	}

	_, err = stmt.ExecContext(ctx, player, claimTypeEnumVals[typ], province)
	if err != nil {
		return fmt.Errorf("failed to insert claim: %w", err)
	}

	return nil
}
