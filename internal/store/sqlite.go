package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func OpenSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("enable foreign keys: %w; close sqlite: %w", err, closeErr)
		}
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	return db, nil
}
