package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"veda/internal/model"
)

const sessionFileName = "session.json"

// sessionDoc / sessionTargetDoc 是磁盘上的结构：路径字段里存的是文件或工作区的编号。
type sessionDoc struct {
	Last *sessionTargetDoc `json:"last,omitempty"`
}

type sessionTargetDoc struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	File string `json:"file,omitempty"`
}

func (doc *sessionDoc) toSession() *model.Session {
	if doc == nil || doc.Last == nil {
		return &model.Session{}
	}
	target := &model.OpenTarget{Kind: doc.Last.Kind}
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
	return &model.Session{Last: target}
}

// SessionStore 持久化最近打开的工作区。
type SessionStore struct {
	mu sync.Mutex
}

func sessionPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionFileName), nil
}

// Load 读取 session.json。文件不存在时返回空会话。
func (s *SessionStore) Load() (*model.Session, error) {
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
			return &model.Session{}, nil
		}
		return nil, err
	}
	if len(bytesTrimSpace(raw)) == 0 {
		return &model.Session{}, nil
	}
	var doc sessionDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.toSession(), nil
}

// Save 把当前工作区写入 session.json，路径会转换成 file-index.json 里的编号。
func (s *SessionStore) Save(sess *model.Session) error {
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
				id, idErr := EnsureFileID(target.File)
				if idErr != nil {
					return idErr
				}
				next.File = id
			}
		} else if target.Path != "" {
			id, idErr := EnsureFileID(target.Path)
			if idErr != nil {
				return idErr
			}
			next.Path = id
		}
		doc.Last = next
	}
	return writeJSONFile(path, doc)
}
