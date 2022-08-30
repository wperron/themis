package themis

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/init.sql
var initScript string

type Store struct {
	db *sql.DB
}

func NewStore(conn string) (*Store, error) {
	db, err := sql.Open("sqlite3", conn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Store{
		db: db,
	}, nil
}
