// Package jsonutil 提供各个持久化模块共用的 JSON 文件写入。
package jsonutil

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteFile 以缩进格式原子地写入一个 JSON 文件，权限为 0600。
func WriteFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
