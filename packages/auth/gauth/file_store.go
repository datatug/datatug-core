package gauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileStore persists service accounts to a JSON file.
// It is safe to use even if the file does not exist yet: Load returns an empty slice.

type FileStore struct {
	// Filepath is the full path to the JSON file.
	Filepath string
}

// Validate implements Validate for FileStore to comply with the project style.
func (s FileStore) Validate() error {
	if s.Filepath == "" {
		return errors.New("file store filepath is required")
	}
	return nil
}

// DefaultFilepath returns a default config path for this application.
// On Unix-like systems: $XDG_CONFIG_HOME/firestore-viewer/service_accounts.json
// On macOS: ~/Library/Application Support/firestore-viewer/service_accounts.json
// On Windows: %AppData%/firestore-viewer/service_accounts.json
func DefaultFilepath() (string, error) {
	confDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}
	dir := filepath.Join(confDir, defaultFilepathDir)
	return filepath.Join(dir, "service_accounts.json"), nil
}

// ensureDir ensures the directory of the configured filepath exists.
func (s FileStore) ensureDir() error {
	if err := s.Validate(); err != nil {
		return err
	}
	dir := filepath.Dir(s.Filepath)
	return os.MkdirAll(dir, 0o755)
}

// Load implements Store.Load.
func (s FileStore) Load() ([]ServiceAccountDbo, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.Filepath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []ServiceAccountDbo{}, nil
		}
		return nil, fmt.Errorf("read file: %w", err)
	}
	var list []ServiceAccountDbo
	if len(b) == 0 {
		return []ServiceAccountDbo{}, nil
	}
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("unmarshal service accounts: %w", err)
	}
	return list, nil
}

// Save implements Store.Save.
func (s FileStore) Save(list []ServiceAccountDbo) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal service accounts: %w", err)
	}
	return os.WriteFile(s.Filepath, b, 0o644)
}
