package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const maxImportedAssetBytes = 32 << 20

var numberedAssetName = regexp.MustCompile(`^(.*)\((\d+)\)$`)

// ImportedAsset 描述已保存的图片，以及要插入到 Markdown 里的路径。
type ImportedAsset struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	MarkdownPath string `json:"markdownPath"`
}

func imageExtension(name, contentType string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".avif", ".tif", ".tiff", ".svg":
		return ext, nil
	}
	kind := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch kind {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	case "image/gif":
		return ".gif", nil
	case "image/webp":
		return ".webp", nil
	case "image/bmp":
		return ".bmp", nil
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico", nil
	case "image/avif":
		return ".avif", nil
	case "image/tiff":
		return ".tiff", nil
	case "image/svg+xml":
		return ".svg", nil
	}
	detected := http.DetectContentType(data)
	if detected != contentType {
		return imageExtension(name, detected, nil)
	}
	return "", errors.New("仅支持导入图片文件")
}

func safeAssetName(name, contentType string, data []byte) (string, error) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		name = "image"
	}
	name, err := sanitizeChildName(name)
	if err != nil {
		return "", err
	}
	ext, err := imageExtension(name, contentType, data)
	if err != nil {
		return "", err
	}
	currentExt := filepath.Ext(name)
	if currentExt == "" {
		name += ext
	} else if strings.ToLower(currentExt) != ext {
		name = strings.TrimSuffix(name, currentExt) + ext
	}
	return name, nil
}

func uniqueAssetFile(dir, name string) (*os.File, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	nextNumber := 1
	if match := numberedAssetName.FindStringSubmatch(base); match != nil {
		if parsed, err := strconv.Atoi(match[2]); err == nil {
			base = match[1]
			nextNumber = parsed + 1
		}
	}
	for i := 0; i < 10000; i++ {
		next := name
		if i > 0 {
			next = fmt.Sprintf("%s(%d)%s", base, nextNumber+i-1, ext)
		}
		file, err := os.OpenFile(filepath.Join(dir, next), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return file, next, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", errors.New("无法生成唯一资源名称")
}

func (s *AppService) assetDirectory(workspacePath string) (string, error) {
	bundle, err := s.settings.loadBundle(workspacePath)
	if err != nil {
		return "", err
	}
	base := workspacePath
	settings := bundle.Workspace
	if workspacePath == "" {
		base, err = dataDir()
		settings = bundle.Global
		if err != nil {
			return "", err
		}
	}
	dir := filepath.Join(base, normalizeResourceDirectory(settings.General.ResourceDirectory))
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, dirAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("资源目录必须位于配置根目录内")
	}
	return dirAbs, nil
}

func (s *AppService) saveAsset(workspacePath, name, contentType string, data []byte) (*ImportedAsset, error) {
	if len(data) == 0 {
		return nil, errors.New("图片内容为空")
	}
	if len(data) > maxImportedAssetBytes {
		return nil, errors.New("图片超过 32 MB")
	}
	name, err := safeAssetName(name, contentType, data)
	if err != nil {
		return nil, err
	}
	dir, err := s.assetDirectory(workspacePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	file, savedName, err := uniqueAssetFile(dir, name)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(dir, savedName)
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(full)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	ok = true
	s.allow(full)
	markdownPath := full
	if workspacePath != "" {
		if rel, relErr := filepath.Rel(workspacePath, full); relErr == nil {
			markdownPath = rel
		}
	}
	return &ImportedAsset{Name: savedName, Path: full, MarkdownPath: filepath.ToSlash(markdownPath)}, nil
}

// ImportImageData 保存以 base64 编码传入的本地图片或粘贴图片。
func (s *AppService) ImportImageData(workspacePath, name, contentType, encoded string) (*ImportedAsset, error) {
	if comma := strings.IndexByte(encoded, ','); comma >= 0 && strings.Contains(encoded[:comma], "base64") {
		encoded = encoded[comma+1:]
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("无法读取图片数据")
	}
	return s.saveAsset(workspacePath, name, contentType, data)
}
