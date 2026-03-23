CREATE TABLE IF NOT EXISTS events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    pid            INTEGER NOT NULL,
    process_name   TEXT    NOT NULL,
    file_path      TEXT    NOT NULL,
    tokens_input   INTEGER NOT NULL DEFAULT 0,
    tokens_output  INTEGER NOT NULL DEFAULT 0,
    mcp_skill_used TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_process   ON events(process_name);
CREATE INDEX IF NOT EXISTS idx_events_file_path ON events(file_path);
