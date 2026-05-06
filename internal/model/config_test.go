package model

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfig_LoadDefault(t *testing.T) {
	// Point HOME to a temp dir with no config file.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: unexpected error: %v", err)
	}
	if cfg.DefaultModel == "" {
		t.Fatal("LoadConfig: default config has empty DefaultModel")
	}
	if len(cfg.Models) == 0 {
		t.Fatal("LoadConfig: default config has no models")
	}
	found := false
	for _, m := range cfg.Models {
		if m.Provider == "deepseek" {
			found = true
			break
		}
	}
	if !found {
		t.Error("LoadConfig: default config should contain a deepseek model")
	}
}

func TestConfig_LoadFromFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".config", "clue-code")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	want := Config{
		DefaultModel: "my-model",
		Models: []ModelConfig{
			{ID: "my-model", Provider: "deepseek", APIKeyEnv: "MY_KEY"},
		},
	}
	data, _ := yaml.Marshal(want)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DefaultModel != want.DefaultModel {
		t.Errorf("DefaultModel: got %q, want %q", cfg.DefaultModel, want.DefaultModel)
	}
	if len(cfg.Models) != 1 || cfg.Models[0].ID != "my-model" {
		t.Errorf("Models: unexpected %+v", cfg.Models)
	}
}

func TestConfig_FindModel(t *testing.T) {
	cfg := &Config{
		Models: []ModelConfig{
			{ID: "a", Provider: "deepseek"},
			{ID: "b", Provider: "ollama"},
		},
	}

	mc, err := cfg.FindModel("a")
	if err != nil {
		t.Fatalf("FindModel(a): %v", err)
	}
	if mc.Provider != "deepseek" {
		t.Errorf("FindModel(a): provider = %q, want deepseek", mc.Provider)
	}

	_, err = cfg.FindModel("missing")
	if err == nil {
		t.Fatal("FindModel(missing): expected ErrModelNotFound, got nil")
	}
	if !isModelNotFound(err) {
		t.Errorf("FindModel(missing): got %v, want ErrModelNotFound", err)
	}
}

func isModelNotFound(err error) bool {
	return err != nil && containsErr(err, ErrModelNotFound)
}

func containsErr(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
