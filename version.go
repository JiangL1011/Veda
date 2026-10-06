package main

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// buildConfigYAML 提供版本号的兜底来源：build/config.yml 的 info.version。
// 发布流水线会先把这个字段改写成 tag 里的版本号，再执行 go build，所以嵌入的
// 就是本次构建的真实版本。
//
//go:embed build/config.yml
var buildConfigYAML []byte

// appVersion 由打包脚本通过 -ldflags "-X main.appVersion=x.y.z" 注入，
// 为空时回退到 build/config.yml。
var appVersion string

const fallbackVersion = "0.0.0"

var (
	versionOnce   sync.Once
	versionCached string
)

// currentVersion 返回当前运行的版本号，统一去掉 v 前缀。
func currentVersion() string {
	versionOnce.Do(func() {
		for _, candidate := range []string{appVersion, versionFromBuildConfig(buildConfigYAML)} {
			if normalized := normalizeVersion(candidate); normalized != "" {
				versionCached = normalized
				return
			}
		}
		versionCached = fallbackVersion
	})
	return versionCached
}

// versionFromBuildConfig 从 config.yml 的 info 段里取出 version。
// 这里只做行级解析，避免为了一个字段引入 YAML 依赖。
func versionFromBuildConfig(raw []byte) string {
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

// normalizeVersion 清理版本字符串：去掉 v/V 前缀与首尾空白，只保留合法的
// 数字与点号、连字符段。无法识别时返回空串。
func normalizeVersion(raw string) string {
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
	value := normalizeVersion(raw)
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

// compareVersions 比较两个版本：返回 -1、0 或 1。无法解析时退化为字符串不等判断，
// 保证“看起来不一样”就不会被当成同一个版本。
func compareVersions(a, b string) int {
	left, okLeft := parseSemver(a)
	right, okRight := parseSemver(b)
	if !okLeft || !okRight {
		if normalizeVersion(a) == normalizeVersion(b) {
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
