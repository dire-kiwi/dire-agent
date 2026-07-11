package configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Store) writeLocked(ctx context.Context, config Config) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("configuration: create directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("configuration: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("configuration: secure temporary file: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		cleanup()
		return fmt.Errorf("configuration: encode: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("configuration: sync: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("configuration: close: %w", err)
	}
	if err := contextErr(ctx); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("configuration: replace: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("configuration: secure file: %w", err)
	}
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("configuration: open directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("configuration: sync directory: %w", err)
	}
	return nil
}
