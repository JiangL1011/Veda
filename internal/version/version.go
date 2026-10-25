// Package version 解析并比较应用版本号。版本号的来源由 main 包在启动时通过
// Init 传入：编译期注入的 appVersion 与 build/config.yml 里的 info.version。
package version

import (
	"strconv"
	"strings"
	"sync"
)

// Fallback 是所有来源都不可用时的兜底版本号。
const Fallback = "0.0.0"

var (
	mu          sync.Mutex
	override    string
	buildConfig []byte
	cached      string
)

// Init 记录版本号来源。override 是由 -ldflags "-X main.appVersion=..." 注入的
// 版本号，buildConfig 是 build/config.yml 的原始内容。它应在第一次调用 Current
// 之前执行；再次调用会丢弃已经缓存的版本号。
func Init(overrideVersion string, config []byte) {
	mu.Lock()
	defer mu.Unlock()
	override = overrideVersion
	buildConfig = append([]byte(nil), config...)
	cached = ""
}

// Current 返回当前运行的版本号，统一去掉 v 前缀。
func Current() string {
	mu.Lock()
	defer mu.Unlock()
	if cached != "" {
		return cached
	}
	for _, candidate := range []string{override, FromBuildConfig(buildConfig)} {
		if normalized := Normalize(candidate); normalized != "" {
			cached = normalized
			return cached
		}
	}
	cached = Fallback
	return cached
}

// FromBuildConfig 从 config.yml 的 info 段里取出 version。
// 这里只做行级解析，避免为了一个字段引入 YAML 依赖。
func FromBuildConfig(raw []byte) string {
	infoIndent := -1
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if infoIndent < 0 {
			if indent == 0 && trimmed == "info:" {
				infoIndent = indent
			}
			continue
		}
		if indent <= infoIndent {
			infoIndent = -1
			if indent == 0 && trimmed == "info:" {
				infoIndent = indent
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "version:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
		if idx := strings.Index(value, " #"); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
		value = strings.Trim(value, `"'`)
		if value != "" {
			return value
		}
	}
	return ""
}

// Normalize 清理版本字符串：去掉 v/V 前缀与首尾空白，只保留合法的
// 数字与点号、连字符段。无法识别时返回空串。
func Normalize(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '+':
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
		default:
			return ""
		}
	}
	if value[0] < '0' || value[0] > '9' {
		return ""
	}
	return value
}

// semver 是版本比较用的最小实现：数字段 + 可选预发布标记。
type semver struct {
	numbers    []int
	prerelease string
}

func parseSemver(raw string) (semver, bool) {
	value := Normalize(raw)
	if value == "" {
		return semver{}, false
	}
	core := value
	if idx := strings.IndexAny(value, "-+"); idx >= 0 {
		core = value[:idx]
		if value[idx] == '-' {
			rest := value[idx+1:]
			if plus := strings.Index(rest, "+"); plus >= 0 {
				rest = rest[:plus]
			}
			parsed := semver{prerelease: rest}
			if !parsed.fill(core) {
				return semver{}, false
			}
			return parsed, true
		}
	}
	parsed := semver{}
	if !parsed.fill(core) {
		return semver{}, false
	}
	return parsed, true
}

func (s *semver) fill(core string) bool {
	for _, part := range strings.Split(core, ".") {
		if part == "" {
			return false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return false
		}
		s.numbers = append(s.numbers, n)
	}
	return len(s.numbers) > 0
}

// Compare 比较两个版本：返回 -1、0 或 1。无法解析时退化为字符串不等判断，
// 保证“看起来不一样”就不会被当成同一个版本。
func Compare(a, b string) int {
	left, okLeft := parseSemver(a)
	right, okRight := parseSemver(b)
	if !okLeft || !okRight {
		if Normalize(a) == Normalize(b) {
			return 0
		}
		return 1
	}
	size := len(left.numbers)
	if len(right.numbers) > size {
		size = len(right.numbers)
	}
	for i := 0; i < size; i++ {
		lv, rv := 0, 0
		if i < len(left.numbers) {
			lv = left.numbers[i]
		}
		if i < len(right.numbers) {
			rv = right.numbers[i]
		}
		if lv != rv {
			if lv < rv {
				return -1
			}
			return 1
		}
	}
	switch {
	case left.prerelease == right.prerelease:
		return 0
	case left.prerelease == "":
		return 1
	case right.prerelease == "":
		return -1
	case left.prerelease < right.prerelease:
		return -1
	default:
		return 1
	}
}
