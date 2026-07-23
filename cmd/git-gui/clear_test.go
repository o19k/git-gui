package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestClearScreenLeavesAltScreenFirst(t *testing.T) {
	var buf bytes.Buffer
	clearScreen(&buf)

	got := buf.String()
	want := "\x1b[?1049l\x1b[H\x1b[2J\x1b[3J"
	if got != want {
		t.Fatalf("clearScreen wrote %q, want %q", got, want)
	}
}

func TestIsTerminalRejectsFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if isTerminal(f) {
		t.Fatal("isTerminal said a regular file is a tty")
	}
}
