// Package threadstore persists each daemon project in its own SQLite file.
package threadstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validThreadID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type Store struct {
	directory string
}

func New(directory string) (*Store, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("threadstore: directory is required")
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("threadstore: resolve directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("threadstore: create directory: %w", err)
	}
	return &Store{directory: abs}, nil
}

func (s *Store) Directory() string { return s.directory }

func (s *Store) Create(ctx context.Context, thread Thread) (*ThreadDB, error) {
	if !validThreadID.MatchString(thread.ID) {
		return nil, fmt.Errorf("threadstore: invalid project id %q", thread.ID)
	}
	path := s.path(thread.ID)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("threadstore: project %q already exists", thread.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	database, err := openDatabase(path)
	if err != nil {
		return nil, err
	}
	db := &ThreadDB{path: path, database: database}
	if err := db.initialize(ctx); err != nil {
		database.Close()
		_ = os.Remove(path)
		return nil, err
	}
	now := time.Now().UTC()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	thread.UpdatedAt = now
	if thread.Status == "" {
		thread.Status = "idle"
	}
	if err := db.writeThread(ctx, thread); err != nil {
		database.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return db, nil
}

func (s *Store) Open(ctx context.Context, id string) (*ThreadDB, error) {
	if !validThreadID.MatchString(id) {
		return nil, fmt.Errorf("threadstore: invalid project id %q", id)
	}
	path := s.path(id)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	database, err := openDatabase(path)
	if err != nil {
		return nil, err
	}
	db := &ThreadDB{path: path, database: database}
	if err := db.initialize(ctx); err != nil {
		database.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) List(ctx context.Context) ([]Thread, error) {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return nil, err
	}
	var threads []Thread
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".db")
		if !validThreadID.MatchString(id) {
			continue
		}
		db, err := s.Open(ctx, id)
		if err != nil {
			return nil, err
		}
		thread, readErr := db.Thread(ctx)
		_ = db.Close()
		if readErr != nil {
			return nil, readErr
		}
		threads = append(threads, thread)
	}
	sort.Slice(threads, func(i, j int) bool { return threads[i].UpdatedAt.After(threads[j].UpdatedAt) })
	return threads, nil
}

func (s *Store) Delete(id string) error {
	if !validThreadID.MatchString(id) {
		return fmt.Errorf("threadstore: invalid project id %q", id)
	}
	err := os.Remove(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return sql.ErrNoRows
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(s.path(id) + suffix)
	}
	return err
}

func (s *Store) path(id string) string { return filepath.Join(s.directory, id+".db") }
