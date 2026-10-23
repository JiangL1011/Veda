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

// sessionDoc / sessionTargetDoc 是磁盘上的结构：路径字段里存的是文件或工作区的编号。
type sessionDoc struct {
	Last *sessionTargetDoc `json:"last,omitempty"`
}

type sessionTargetDoc struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	File string `json:"file,omitempty"`
}

func (doc *sessionDoc) toSession() *Session {
	if doc == nil || doc.Last == nil {
		return &Session{}
	}
	target := &OpenTarget{Kind: doc.Last.Kind}
	if doc.Last.Kind == "directory" {
		if path, ok := workspacePathByID(doc.Last.Path); ok {
			target.Path = path
		} else {
			// 旧数据里直接存的是路径；编号找不到时也按旧格式理解。
			target.Path = storedFilePath(doc.Last.Path)
		}
		target.File = storedFilePath(doc.Last.File)
	} else {
		target.Path = storedFilePath(doc.Last.Path)
	}
	return &Session{Last: target}
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

	waitForDataMigration()
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
	if len(bytesTrimSpace(raw)) == 0 {
		return &Session{}, nil
	}
	var doc sessionDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.toSession(), nil
}

func (s *sessionStore) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	waitForDataMigration()
	path, err := sessionPath()
	if err != nil {
		return err
	}
	doc := &sessionDoc{}
	if sess != nil && sess.Last != nil {
		target := sess.Last
		next := &sessionTargetDoc{Kind: target.Kind}
		if target.Kind == "directory" {
			if target.Path != "" {
				abs, absErr := normalizeFilePath(target.Path)
				if absErr != nil {
					return absErr
				}
				id, idErr := ensureWorkspaceID(abs)
				if idErr != nil {
					return idErr
				}
				next.Path = id
			}
			if target.File != "" {
				id, idErr := ensureFileID(target.File)
				if idErr != nil {
					return idErr
				}
				next.File = id
			}
		} else if target.Path != "" {
			id, idErr := ensureFileID(target.Path)
			if idErr != nil {
				return idErr
			}
			next.Path = id
		}
		doc.Last = next
	}
	return writeJSONFile(path, doc)
}
