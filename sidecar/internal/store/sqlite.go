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
	if filter.PathPrefix != "" {
		q += " AND file_path GLOB ?"
		args = append(args, filter.PathPrefix+"/*")
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
	if filter.PathPrefix != "" {
		q += " AND file_path GLOB ?"
		args = append(args, filter.PathPrefix+"/*")
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

// ClearByPath deletes all events whose file_path falls under pathPrefix.
func (s *SQLiteStore) ClearByPath(ctx context.Context, pathPrefix string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE file_path GLOB ?", pathPrefix+"/*")
	if err != nil {
		return fmt.Errorf("clear events by path: %w", err)
	}
	return nil
}

// QueryByDay returns token totals grouped by calendar day.
func (s *SQLiteStore) QueryByDay(ctx context.Context, filter QueryFilter) ([]DaySummary, error) {
	q := `SELECT DATE(timestamp) AS day,
	             SUM(tokens_input)  AS tokens_in,
	             SUM(tokens_output) AS tokens_out
	      FROM events WHERE 1=1`
	var args []any

	if filter.ProcessName != "" {
		q += " AND process_name = ?"
		args = append(args, filter.ProcessName)
	}
	if filter.PathPrefix != "" {
		q += " AND file_path GLOB ?"
		args = append(args, filter.PathPrefix+"/*")
	}
	if filter.Since != nil {
		q += " AND timestamp >= ?"
		args = append(args, filter.Since.UTC().Format("2006-01-02T15:04:05.000"))
	}

	q += " GROUP BY DATE(timestamp) ORDER BY day"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query by day: %w", err)
	}
	defer rows.Close()

	var results []DaySummary

	for rows.Next() {
		var r DaySummary
		if err := rows.Scan(&r.Day, &r.TokensIn, &r.TokensOut); err != nil {
			return nil, fmt.Errorf("scan day summary: %w", err)
		}
		results = append(results, r)
	}

	if results == nil {
		results = []DaySummary{}
	}

	return results, rows.Err()
}

// BatchInsertEvents records multiple events in a single transaction.
func (s *SQLiteStore) BatchInsertEvents(ctx context.Context, events []*Event) error {
	const q = `INSERT INTO events (timestamp, pid, process_name, file_path, tokens_input, tokens_output)
		VALUES (?, ?, ?, ?, ?, ?)`

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch tx: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare batch insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		ts := e.Timestamp.UTC().Format("2006-01-02T15:04:05.000")
		if _, err := stmt.ExecContext(ctx, ts, e.PID, e.ProcessName, e.FilePath, e.TokensInput, e.TokensOutput); err != nil {
			tx.Rollback()
			return fmt.Errorf("batch insert event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch events: %w", err)
	}

	return nil
}

// BatchInsertToolEvents records multiple tool events in a single transaction, ignoring duplicates.
func (s *SQLiteStore) BatchInsertToolEvents(ctx context.Context, events []*ToolEvent) error {
	const q = `INSERT OR IGNORE INTO skill_events
		(timestamp, agent, session_id, tool_name, tool_call_id, cwd)
		VALUES (?, ?, ?, ?, ?, ?)`

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch tool tx: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare batch tool insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		ts := e.Timestamp.UTC().Format("2006-01-02T15:04:05.000")
		if _, err := stmt.ExecContext(ctx, ts, e.Agent, e.SessionID, e.ToolName, e.ToolCallID, e.CWD); err != nil {
			tx.Rollback()
			return fmt.Errorf("batch insert tool event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch tool events: %w", err)
	}

	return nil
}

// InsertToolEvent records a skill/tool invocation, ignoring duplicates.
func (s *SQLiteStore) InsertToolEvent(ctx context.Context, e *ToolEvent) error {
	const q = `INSERT OR IGNORE INTO skill_events
		(timestamp, agent, session_id, tool_name, tool_call_id, cwd)
		VALUES (?, ?, ?, ?, ?, ?)`

	ts := e.Timestamp.UTC().Format("2006-01-02T15:04:05.000")
	_, err := s.db.ExecContext(ctx, q, ts, e.Agent, e.SessionID, e.ToolName, e.ToolCallID, e.CWD)
	if err != nil {
		return fmt.Errorf("insert skill event: %w", err)
	}
	return nil
}

// QueryTools returns aggregated tool-use counts grouped by tool_name and agent.
func (s *SQLiteStore) QueryTools(ctx context.Context, filter ToolFilter) ([]ToolSummary, error) {
	q := `SELECT tool_name, agent, COUNT(*) AS cnt FROM skill_events WHERE 1=1`
	var args []any

	if filter.Agent != "" {
		q += " AND agent = ?"
		args = append(args, filter.Agent)
	}
	if filter.CWDPrefix != "" {
		q += " AND (cwd = ? OR cwd GLOB ?)"
		args = append(args, filter.CWDPrefix, filter.CWDPrefix+"/*")
	}
	if filter.Since != nil {
		q += " AND timestamp >= ?"
		args = append(args, filter.Since.UTC().Format("2006-01-02T15:04:05.000"))
	}

	q += " GROUP BY tool_name, agent ORDER BY cnt DESC"

	if filter.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query skills: %w", err)
	}
	defer rows.Close()

	var results []ToolSummary

	for rows.Next() {
		var r ToolSummary
		if err := rows.Scan(&r.ToolName, &r.Agent, &r.Count); err != nil {
			return nil, fmt.Errorf("scan skill summary: %w", err)
		}
		results = append(results, r)
	}

	if results == nil {
		results = []ToolSummary{}
	}

	return results, rows.Err()
}

// GetLogOffset returns the last-processed byte offset for a log file.
func (s *SQLiteStore) GetLogOffset(ctx context.Context, logPath string) (int64, error) {
	var offset int64
	err := s.db.QueryRowContext(ctx,
		"SELECT byte_offset FROM log_offsets WHERE log_path = ?", logPath,
	).Scan(&offset)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get log offset: %w", err)
	}
	return offset, nil
}

// SetLogOffset saves the byte offset for a log file.
func (s *SQLiteStore) SetLogOffset(ctx context.Context, logPath string, offset int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO log_offsets (log_path, byte_offset) VALUES (?, ?)
		 ON CONFLICT(log_path) DO UPDATE SET byte_offset = excluded.byte_offset`,
		logPath, offset,
	)
	if err != nil {
		return fmt.Errorf("set log offset: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
