package main

import "testing"

func TestVersionFromBuildConfigReadsEmbeddedConfig(t *testing.T) {
	// 不写死版本号：build/config.yml 的 info.version 每次发版都会变，
	// 这里只要求它存在且能被解析。
	got := versionFromBuildConfig(buildConfigYAML)
	t.Logf("embedded version = %q", got)
	if got == "" {
		t.Fatal("embedded build/config.yml has no info.version")
	}
	if _, ok := parseSemver(got); !ok {
		t.Fatalf("embedded version %q is not a parseable version", got)
	}
}

func TestVersionFromBuildConfigIgnoresOtherKeys(t *testing.T) {
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
	if got := versionFromBuildConfig(raw); got != "2.3.4" {
		t.Fatalf("versionFromBuildConfig = %q, want %q", got, "2.3.4")
	}
}

func TestVersionFromBuildConfigWithoutInfoBlock(t *testing.T) {
	if got := versionFromBuildConfig([]byte("version: '3'\n")); got != "" {
		t.Fatalf("versionFromBuildConfig = %q, want empty", got)
	}
}

func TestVersionFromBuildConfigSkipsMissingVersion(t *testing.T) {
	raw := []byte("info:\n  companyName: \"Veda\"\nother:\n  version: \"1.0.0\"\n")
	if got := versionFromBuildConfig(raw); got != "" {
		t.Fatalf("versionFromBuildConfig = %q, want empty", got)
	}
}

func TestNormalizeVersion(t *testing.T) {
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
		if got := normalizeVersion(input); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
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
		if got := compareVersions(tc.left, tc.right); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestCompareVersionsFallsBackToInequality(t *testing.T) {
	if got := compareVersions("nightly", "0.1.0"); got == 0 {
		t.Fatalf("compareVersions with unparsable input = %d, want non-zero", got)
	}
}

func TestCurrentVersionIsNotFallback(t *testing.T) {
	if got := currentVersion(); got == "" || got == fallbackVersion {
		t.Fatalf("currentVersion() = %q, want a real version", got)
	}
}
