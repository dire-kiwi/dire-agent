package threadstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type ThreadDB struct {
	path     string
	database *sql.DB
}

func openDatabase(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("threadstore: open SQLite: %w", err)
	}
	database.SetMaxOpenConns(1)
	return database, nil
}

func (d *ThreadDB) initialize(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS metadata (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    data BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    data BLOB,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    data BLOB,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS provider_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    provider TEXT NOT NULL,
    session_id TEXT NOT NULL,
    data BLOB NOT NULL,
    updated_at TEXT NOT NULL
);`
	if _, err := d.database.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("threadstore: initialize SQLite schema: %w", err)
	}
	return nil
}

func (d *ThreadDB) Path() string { return d.path }
func (d *ThreadDB) Close() error { return d.database.Close() }

func (d *ThreadDB) Thread(ctx context.Context) (Thread, error) {
	var data []byte
	if err := d.database.QueryRowContext(ctx, `SELECT data FROM metadata WHERE singleton = 1`).Scan(&data); err != nil {
		return Thread{}, err
	}
	var thread Thread
	if err := json.Unmarshal(data, &thread); err != nil {
		return Thread{}, fmt.Errorf("threadstore: decode project metadata: %w", err)
	}
	return thread, nil
}

func (d *ThreadDB) UpdateThread(ctx context.Context, update func(*Thread) error) (Thread, error) {
	thread, err := d.Thread(ctx)
	if err != nil {
		return Thread{}, err
	}
	if err := update(&thread); err != nil {
		return Thread{}, err
	}
	thread.UpdatedAt = time.Now().UTC()
	if err := d.writeThread(ctx, thread); err != nil {
		return Thread{}, err
	}
	return thread, nil
}

func (d *ThreadDB) writeThread(ctx context.Context, thread Thread) error {
	data, err := json.Marshal(thread)
	if err != nil {
		return err
	}
	_, err = d.database.ExecContext(ctx, `INSERT INTO metadata(singleton, data) VALUES(1, ?) ON CONFLICT(singleton) DO UPDATE SET data=excluded.data`, data)
	return err
}

func (d *ThreadDB) SaveState(ctx context.Context, state State) error {
	state.UpdatedAt = time.Now().UTC()
	_, err := d.database.ExecContext(ctx, `
INSERT INTO provider_state(singleton, provider, session_id, data, updated_at)
VALUES(1, ?, ?, ?, ?)
ON CONFLICT(singleton) DO UPDATE SET provider=excluded.provider, session_id=excluded.session_id, data=excluded.data, updated_at=excluded.updated_at`,
		state.Provider, state.SessionID, []byte(state.Data), state.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (d *ThreadDB) LoadState(ctx context.Context) (State, error) {
	var state State
	var data []byte
	var updated string
	err := d.database.QueryRowContext(ctx,
		`SELECT provider, session_id, data, updated_at FROM provider_state WHERE singleton = 1`).
		Scan(&state.Provider, &state.SessionID, &data, &updated)
	if err != nil {
		return State{}, err
	}
	state.Data = append(json.RawMessage(nil), data...)
	state.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return state, nil
}
