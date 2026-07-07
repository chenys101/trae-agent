package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite_createsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := AtomicWrite(path, []byte("hello world"), 0o644)
	if err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("got %q, want %q", string(data), "hello world")
	}
}

func TestAtomicWrite_overwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// 先写入初始内容
	if err := AtomicWrite(path, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 覆盖写入
	if err := AtomicWrite(path, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Errorf("got %q, want %q", string(data), "new content")
	}
}

func TestAtomicWrite_noTempFilesLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := AtomicWrite(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "test.txt" {
			t.Errorf("unexpected temp file left: %s", e.Name())
		}
	}
}
