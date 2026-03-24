CREATE TABLE IF NOT EXISTS events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    pid            INTEGER NOT NULL,
    process_name   TEXT    NOT NULL,
    file_path      TEXT    NOT NULL,
    tokens_input   INTEGER NOT NULL DEFAULT 0,
    tokens_output  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_process   ON events(process_name);
CREATE INDEX IF NOT EXISTS idx_events_file_path ON events(file_path);

CREATE TABLE IF NOT EXISTS skill_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp    TEXT    NOT NULL,
    agent        TEXT    NOT NULL,
    session_id   TEXT    NOT NULL,
    tool_name    TEXT    NOT NULL,
    tool_call_id TEXT    NOT NULL UNIQUE,
    cwd          TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_skill_ts    ON skill_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_skill_agent ON skill_events(agent);
CREATE INDEX IF NOT EXISTS idx_skill_tool  ON skill_events(tool_name);
CREATE INDEX IF NOT EXISTS idx_skill_cwd   ON skill_events(cwd);

CREATE TABLE IF NOT EXISTS log_offsets (
    log_path    TEXT    PRIMARY KEY,
    byte_offset INTEGER NOT NULL DEFAULT 0
);
