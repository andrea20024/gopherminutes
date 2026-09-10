// Package storage provides data access layer with PostgreSQL.
package storage

import (
	"context"
	"database/sql"

	"github.com/andrea20024/goferminutes2/internal/logger"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Repository provides shared SQL infrastructure for all repositories.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository with the given database connection.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// DB returns the underlying *sql.DB for direct access when needed.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// TxQueryRow executes a query within a transaction and scans the result.
func (r *Repository) TxQueryRow(tx *sql.Tx, query string, args ...interface{}) *sql.Row {
	return tx.QueryRow(query, args...)
}

// TxQuery executes a query within a transaction.
func (r *Repository) TxQuery(tx *sql.Tx, query string, args ...interface{}) (*sql.Rows, error) {
	return tx.Query(query, args...)
}

// TxExec executes a statement within a transaction.
func (r *Repository) TxExec(tx *sql.Tx, query string, args ...interface{}) (sql.Result, error) {
	return tx.Exec(query, args...)
}

// QueryRow executes a query and scans the result.
func (r *Repository) QueryRow(query string, args ...interface{}) *sql.Row {
	return r.db.QueryRow(query, args...)
}

// Query executes a query and returns rows.
func (r *Repository) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return r.db.Query(query, args...)
}

// Exec executes a statement.
func (r *Repository) Exec(query string, args ...interface{}) (sql.Result, error) {
	return r.db.Exec(query, args...)
}

// Ping checks the database connection.
func (r *Repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// Shutdown closes the database connection.
func (r *Repository) Shutdown() error {
	if logger.Sugar() != nil {
		logger.Sugar().Infow("closing database connection", "component", "storage")
	}
	return r.db.Close()
}

// DeleteMeeting deletes a meeting and its associated task by ID.
// This is a convenience method that delegates to MeetingRepo.DeleteMeeting.
func (r *Repository) DeleteMeeting(ctx context.Context, meetingID int) error {
	repo := NewMeetingRepo(r)
	return repo.DeleteMeeting(ctx, meetingID)
}
