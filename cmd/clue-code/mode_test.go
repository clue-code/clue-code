package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests that use t.Setenv must NOT call t.Parallel() — Go panics on that combo.

func TestCmd_Mode_Get_DefaultHybrid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("CLUE_CODE_CONFIG", cfgPath)

	ctx := context.Background()
	code := runModeGet(ctx)
	if code != 0 {
		t.Fatalf("runModeGet returned %d, want 0", code)
	}
}

func TestCmd_Mode_Set_Valid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("CLUE_CODE_CONFIG", cfgPath)

	ctx := context.Background()

	for _, mode := range []string{"local", "cloud", "hybrid"} {
		code := runModeSet(ctx, mode)
		if code != 0 {
			t.Errorf("runModeSet(%q) returned %d, want 0", mode, code)
			continue
		}

		data, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("ReadFile after set %q: %v", mode, err)
		}
		if !strings.Contains(string(data), mode) {
			t.Errorf("config file does not contain %q: %s", mode, data)
		}
	}
}

func TestCmd_Mode_Set_Invalid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLUE_CODE_CONFIG", filepath.Join(dir, "config.json"))

	ctx := context.Background()
	code := runModeSet(ctx, "invalid")
	if code != 2 {
		t.Errorf("runModeSet(invalid) returned %d, want 2", code)
	}
}

func TestCmd_Mode_RunMode_Get(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLUE_CODE_CONFIG", filepath.Join(dir, "config.json"))

	ctx := context.Background()
	code := runMode(ctx, []string{"get"})
	if code != 0 {
		t.Errorf("runMode([get]) = %d, want 0", code)
	}
}

func TestCmd_Mode_RunMode_Set_Valid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("CLUE_CODE_CONFIG", cfgPath)

	ctx := context.Background()
	code := runMode(ctx, []string{"set", "local"})
	if code != 0 {
		t.Errorf("runMode([set local]) = %d, want 0", code)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "local") {
		t.Errorf("config does not contain 'local': %s", data)
	}
}

func TestCmd_Mode_RunMode_Set_Invalid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLUE_CODE_CONFIG", filepath.Join(dir, "config.json"))

	ctx := context.Background()
	code := runMode(ctx, []string{"set", "nope"})
	if code != 2 {
		t.Errorf("runMode([set nope]) = %d, want 2", code)
	}
}

func TestCmd_Mode_RunMode_NoArgs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	code := runMode(ctx, []string{})
	if code != 2 {
		t.Errorf("runMode([]) = %d, want 2", code)
	}
}

func TestCmd_Mode_RunMode_Unknown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	code := runMode(ctx, []string{"bogus"})
	if code != 2 {
		t.Errorf("runMode([bogus]) = %d, want 2", code)
	}
}

func TestCmd_Mode_RunMode_Help(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	code := runMode(ctx, []string{"help"})
	if code != 0 {
		t.Errorf("runMode([help]) = %d, want 0", code)
	}
}

func TestCmd_Mode_Persistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("CLUE_CODE_CONFIG", cfgPath)

	ctx := context.Background()

	if code := runMode(ctx, []string{"set", "cloud"}); code != 0 {
		t.Fatalf("set cloud: code %d", code)
	}

	if code := runModeGet(ctx); code != 0 {
		t.Fatalf("get after set cloud: code %d", code)
	}

	if code := runMode(ctx, []string{"set", "hybrid"}); code != 0 {
		t.Fatalf("set hybrid: code %d", code)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "hybrid") {
		t.Errorf("config does not contain 'hybrid' after set: %s", data)
	}
}
