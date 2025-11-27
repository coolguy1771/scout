package database

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type DB struct {
	*sqlx.DB
	logger *zap.Logger
}

func New(dsn string) (*DB, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Initialize logger (can be nil if not needed)
	logger := zap.NewNop()

	return &DB{DB: db, logger: logger}, nil
}

func (d *DB) Close() error {
	return d.DB.Close()
}

// WithTx executes a function within a transaction
func (d *DB) WithTx(fn func(*sqlx.Tx) error) error {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	err = fn(tx)
	return err
}

// Health checks database connectivity
func (d *DB) Health() error {
	var result sql.NullString
	err := d.Get(&result, "SELECT 1")
	return err
}
