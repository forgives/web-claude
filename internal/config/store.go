package config

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type File struct {
	AllowRemoteAccess  bool      `json:"allow_remote_access"`
	ListenAddr         string    `json:"listen_addr,omitempty"`
	PasswordHash       string    `json:"password_hash,omitempty"`
	RestartOnReconnect bool      `json:"restart_on_reconnect"`
	SessionSecret      string    `json:"session_secret,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	cfg  File
}

func Load(path string) (*Store, error) {
	store := &Store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, &store.cfg); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) ListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.ListenAddr != "" {
		return s.cfg.ListenAddr
	}
	return "127.0.0.1:8081"
}

func (s *Store) AllowRemoteAccess() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.AllowRemoteAccess
}

func (s *Store) RestartOnReconnect() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.RestartOnReconnect
}

func (s *Store) PasswordHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.PasswordHash
}

func (s *Store) SessionSecret() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.SessionSecret
}

func (s *Store) AuthConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.PasswordHash != "" && s.cfg.SessionSecret != ""
}

func (s *Store) ValidateListenAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" {
		host = "0.0.0.0"
	}
	if s.AllowRemoteAccess() {
		return nil
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("remote access is disabled; set allow_remote_access to true in data/config.json")
}

func DefaultPath(projectDir string) string {
	override := os.Getenv("WEB_CLAUDE_DATA_DIR")
	if override != "" {
		return filepath.Join(override, "config.json")
	}
	return filepath.Join(projectDir, "data", "config.json")
}

func (s *Store) SetAuth(passwordHash, sessionSecret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if s.cfg.CreatedAt.IsZero() {
		s.cfg.CreatedAt = now
	}
	s.cfg.PasswordHash = passwordHash
	s.cfg.SessionSecret = sessionSecret
	s.cfg.UpdatedAt = now

	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(s.path), "config-*.json")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(0o600); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, s.path)
}
