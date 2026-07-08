package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironMap(t *testing.T) {
	env := EnvironMap([]string{"A=one", "B=two=three", "invalid"})
	if got := env["A"]; got != "one" {
		t.Fatalf("A = %q, want one", got)
	}
	if got := env["B"]; got != "two=three" {
		t.Fatalf("B = %q, want two=three", got)
	}
	if _, ok := env["invalid"]; ok {
		t.Fatal("invalid entry should be ignored")
	}
}

func TestValue(t *testing.T) {
	env := map[string]string{"SET": "value", "EMPTY": ""}
	if got := Value(env, "SET", "fallback"); got != "value" {
		t.Fatalf("SET = %q, want value", got)
	}
	if got := Value(env, "EMPTY", "fallback"); got != "fallback" {
		t.Fatalf("EMPTY = %q, want fallback", got)
	}
	if got := Value(env, "MISSING", "fallback"); got != "fallback" {
		t.Fatalf("MISSING = %q, want fallback", got)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".release-tools.env")
	if err := os.WriteFile(path, []byte("# comment\nA=file\nB='quoted'\nC=\"double\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"A": "env"}
	allowed := map[string]bool{"A": true, "B": true, "C": true}
	if err := LoadFile(path, env, allowed); err != nil {
		t.Fatal(err)
	}
	if got := env["A"]; got != "env" {
		t.Fatalf("A = %q, want env", got)
	}
	if got := env["B"]; got != "quoted" {
		t.Fatalf("B = %q, want quoted", got)
	}
	if got := env["C"]; got != "double" {
		t.Fatalf("C = %q, want double", got)
	}
}

func TestLoadFileRejectsUnsupportedKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".release-tools.env")
	if err := os.WriteFile(path, []byte("A=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path, map[string]string{}, map[string]bool{}); err == nil {
		t.Fatal("expected unsupported key error")
	}
}

func TestLoadFileRejectsInvalidLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".release-tools.env")
	if err := os.WriteFile(path, []byte("invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path, map[string]string{}, map[string]bool{}); err == nil {
		t.Fatal("expected invalid line error")
	}
}
