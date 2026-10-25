package store

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"veda/internal/jsonutil"
)

const (
	dataDirName = ".veda"
	// legacyDataDirName 是改名前的数据目录，仅用于把旧数据迁移到 ~/.veda。
	legacyDataDirName = ".marknote"
)

// dataDirMu 串行化首次创建数据目录时的旧数据迁移。
var dataDirMu sync.Mutex

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DataDir 返回全局数据目录（~/.veda），首次调用时会把旧版本的
// ~/.marknote 数据迁移过来。
func DataDir() (string, error) {
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

func dataDir() (string, error) {
	return DataDir()
}

// isUUID 判断一个字符串是不是 file-index.json 分配的编号。
func isUUID(value string) bool {
	return uuidPattern.MatchString(strings.TrimSpace(value))
}

// newUUID 生成一个随机 UUID（v4）。
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32], nil
}

// writeJSONFile 以缩进格式原子地写入一个 JSON 文件。
func writeJSONFile(path string, value any) error {
	return jsonutil.WriteFile(path, value)
}

// bytesTrimSpace 去掉字节切片首尾的空白，用来判断文件是否为空。
func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

// mergeLegacyDir 把 legacy 目录中尚未存在于 dest 的条目移动过去，然后删除 legacy。
func mergeLegacyDir(legacy, dest string) error {
	info, err := os.Stat(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(legacy, entry.Name())
		dst := filepath.Join(dest, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return os.RemoveAll(legacy)
}

// migrateLegacyDataDir 把旧版本 Marknote 的 ~/.marknote 迁移到 ~/.veda。
func migrateLegacyDataDir(home, dest string) error {
	return mergeLegacyDir(filepath.Join(home, legacyDataDirName), dest)
}

// migrateLegacyWorkspaceDir 把工作区里遗留的 .veda/.marknote 目录迁移到全局数据目录。
func migrateLegacyWorkspaceDir(abs, dest string) error {
	for _, name := range []string{dataDirName, legacyDataDirName} {
		if err := mergeLegacyDir(filepath.Join(abs, name), dest); err != nil {
			return err
		}
	}
	return nil
}
