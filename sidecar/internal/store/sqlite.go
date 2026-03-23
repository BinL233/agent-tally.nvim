package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SQLiteStore implements Store backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLite opens (or creates) a SQLite database at the given path.
func NewSQLite(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Init runs the embedded schema DDL to create tables and indexes.
func (s *SQLiteStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

// InsertEvent records a new event into the events table.
func (s *SQLiteStore) InsertEvent(ctx context.Context, e *Event) error {
	const q = `INSERT INTO events (timestamp, pid, process_name, file_path, tokens_input, tokens_output)
		VALUES (?, ?, ?, ?, ?, ?)`

	ts := e.Timestamp.UTC().Format("2006-01-02T15:04:05.000")
	_, err := s.db.ExecContext(ctx, q, ts, e.PID, e.ProcessName, e.FilePath, e.TokensInput, e.TokensOutput)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// Query returns events matching the given filter.
func (s *SQLiteStore) Query(ctx context.Context, filter QueryFilter) ([]Event, error) {
	q := `SELECT id, timestamp, pid, process_name, file_path, tokens_input, tokens_output
		FROM events WHERE 1=1`
	var args []any

	if filter.ProcessName != "" {
		q += " AND process_name = ?"
		args = append(args, filter.ProcessName)
	}
	if filter.Since != nil {
		q += " AND timestamp >= ?"
		args = append(args, filter.Since.UTC().Format("2006-01-02T15:04:05.000"))
	}

	q += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event

	for rows.Next() {
		var ev Event
		var ts string

		if err := rows.Scan(&ev.ID, &ts, &ev.PID, &ev.ProcessName, &ev.FilePath,
			&ev.TokensInput, &ev.TokensOutput); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		ev.Timestamp, _ = time.Parse("2006-01-02T15:04:05.000", ts)
		events = append(events, ev)
	}

	return events, rows.Err()
}

// QueryByFile returns token totals grouped by file path.
func (s *SQLiteStore) QueryByFile(ctx context.Context, filter QueryFilter) ([]FileTokenSummary, error) {
	q := `SELECT file_path, SUM(tokens_output) AS total_tokens, COUNT(*) AS event_count
		FROM events WHERE 1=1`
	var args []any

	if filter.ProcessName != "" {
		q += " AND process_name = ?"
		args = append(args, filter.ProcessName)
	}

	if filter.Since != nil {
		q += " AND timestamp >= ?"
		args = append(args, filter.Since.UTC().Format("2006-01-02T15:04:05.000"))
	}

	q += " GROUP BY file_path ORDER BY total_tokens DESC"

	if filter.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query by file: %w", err)
	}
	defer rows.Close()

	var results []FileTokenSummary

	for rows.Next() {
		var r FileTokenSummary

		if err := rows.Scan(&r.FilePath, &r.TokensOutput, &r.EventCount); err != nil {
			return nil, fmt.Errorf("scan file summary: %w", err)
		}

		results = append(results, r)
	}

	if results == nil {
		results = []FileTokenSummary{}
	}

	return results, rows.Err()
}

// ClearAll deletes all events from the events table.
func (s *SQLiteStore) ClearAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events")
	if err != nil {
		return fmt.Errorf("clear events: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
