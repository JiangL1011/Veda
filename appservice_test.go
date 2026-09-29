package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTextAlwaysReadsCurrentDiskContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := newAppService()
	first, err := service.ReadText(path)
	if err != nil || first.Content != "first" {
		t.Fatalf("first read: %+v, %v", first, err)
	}

	if err := os.WriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := service.ReadText(path)
	if err != nil || second.Content != "second" {
		t.Fatalf("second read should come from disk: %+v, %v", second, err)
	}
}
