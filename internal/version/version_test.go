package version

import (
	"os"
	"testing"
)

// repoBuildConfig 读取仓库里的 build/config.yml，作为测试用的真实配置。
func repoBuildConfig(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../build/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFromBuildConfigReadsRepoConfig(t *testing.T) {
	// 不写死版本号：build/config.yml 的 info.version 每次发版都会变，
	// 这里只要求它存在且能被解析。
	got := FromBuildConfig(repoBuildConfig(t))
	t.Logf("embedded version = %q", got)
	if got == "" {
		t.Fatal("build/config.yml has no info.version")
	}
	if _, ok := parseSemver(got); !ok {
		t.Fatalf("build/config.yml version %q is not a parseable version", got)
	}
}

func TestFromBuildConfigIgnoresOtherKeys(t *testing.T) {
	raw := []byte(`version: '3'

info:
  companyName: "Veda"
  # version: "9.9.9"
  version: "2.3.4"
  comments: "Veda"

dev_mode:
  root_path: .
  version: "ignored"
`)
	if got := FromBuildConfig(raw); got != "2.3.4" {
		t.Fatalf("FromBuildConfig = %q, want %q", got, "2.3.4")
	}
}

func TestFromBuildConfigWithoutInfoBlock(t *testing.T) {
	if got := FromBuildConfig([]byte("version: '3'\n")); got != "" {
		t.Fatalf("FromBuildConfig = %q, want empty", got)
	}
}

func TestFromBuildConfigSkipsMissingVersion(t *testing.T) {
	raw := []byte("info:\n  companyName: \"Veda\"\nother:\n  version: \"1.0.0\"\n")
	if got := FromBuildConfig(raw); got != "" {
		t.Fatalf("FromBuildConfig = %q, want empty", got)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"v1.2.3":     "1.2.3",
		" V0.1.0 ":   "0.1.0",
		"2.0.0-rc1":  "2.0.0-rc1",
		"1.0":        "1.0",
		"":           "",
		"nightly":    "",
		"v":          "",
		"1.2.3-beta": "1.2.3-beta",
	}
	for input, want := range cases {
		if got := Normalize(input); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"0.1.0", "0.1.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"1.0.0", "0.9.9", 1},
		{"1.0.0", "1.0", 0},
		{"1.0.1", "1.0", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"2.0.0", "10.0.0", -1},
	}
	for _, tc := range cases {
		if got := Compare(tc.left, tc.right); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestCompareFallsBackToInequality(t *testing.T) {
	if got := Compare("nightly", "0.1.0"); got == 0 {
		t.Fatalf("Compare with unparsable input = %d, want non-zero", got)
	}
}

func TestCurrentIsNotFallback(t *testing.T) {
	Init("", repoBuildConfig(t))
	if got := Current(); got == "" || got == Fallback {
		t.Fatalf("Current() = %q, want a real version", got)
	}
}

func TestCurrentPrefersLdflagsOverride(t *testing.T) {
	Init("v9.8.7", nil)
	if got := Current(); got != "9.8.7" {
		t.Fatalf("Current() = %q, want the ldflags override", got)
	}
}
