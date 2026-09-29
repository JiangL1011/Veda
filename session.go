package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	sessionFileName = "session.json"
	dataDirName     = ".veda"
	// legacyDataDirName 是改名前的数据目录，仅用于把旧数据迁移到 ~/.veda。
	legacyDataDirName = ".marknote"
)

// dataDirMu 串行化首次创建数据目录时的旧数据迁移。
var dataDirMu sync.Mutex

// Session 持久化在 ~/.veda/session.json 中。
type Session struct {
	Last *OpenTarget `json:"last,omitempty"`
}

// OpenTarget 是用户打开的文件或目录。
type OpenTarget struct {
	Kind string `json:"kind"` // "file" | "directory"
	Path string `json:"path"`
	File string `json:"file,omitempty"`
}

type sessionStore struct {
	mu sync.Mutex
}

func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDirMu.Lock()
	defer dataDirMu.Unlock()

	dir := filepath.Join(home, dataDirName)
	// 首次以 Veda 运行时，把旧版 Marknote 的 ~/.marknote 数据搬过来。
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := migrateLegacyDataDir(home, dir); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func sessionPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionFileName), nil
}

func (s *sessionStore) Load() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Session{}, nil
		}
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *sessionStore) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := sessionPath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, sess)
}
