package config

import (
	"testing"
)

func TestDefaults(t *testing.T) {
	t.Parallel()

	c := Defaults()
	if c.Mode != ModeLocal {
		t.Errorf("default Mode = %q, want %q", c.Mode, ModeLocal)
	}
	if len(c.ModelByTier) == 0 {
		t.Error("default ModelByTier is empty")
	}
	for _, tier := range []Tier{TierL0, TierL1, TierL2, TierL3} {
		if c.ModelByTier[tier] == "" {
			t.Errorf("default ModelByTier[%q] is empty", tier)
		}
	}
	if c.Telemetry {
		t.Error("Telemetry should be off by default (privacy-first)")
	}
}

func TestLoad_DefaultMode(t *testing.T) {
	t.Setenv("CLUE_CODE_MODE", "")
	c := Load()
	if c.Mode != ModeLocal {
		t.Errorf("Mode = %q, want %q", c.Mode, ModeLocal)
	}
}

func TestLoad_OverrideMode(t *testing.T) {
	t.Setenv("CLUE_CODE_MODE", "cloud")
	c := Load()
	if c.Mode != ModeCloud {
		t.Errorf("Mode = %q, want %q", c.Mode, ModeCloud)
	}
}

func TestLoad_InvalidModeIgnored(t *testing.T) {
	t.Setenv("CLUE_CODE_MODE", "bogus")
	c := Load()
	if c.Mode != ModeLocal {
		t.Errorf("invalid mode override should be ignored, got %q", c.Mode)
	}
}

func TestValidate_AcceptsModes(t *testing.T) {
	t.Parallel()
	for _, m := range []Mode{ModeLocal, ModeCloud, ModeHybrid} {
		c := Defaults()
		c.Mode = m
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%q) returned error: %v", m, err)
		}
	}
}

func TestValidate_RejectsUnknownMode(t *testing.T) {
	t.Parallel()
	c := Defaults()
	c.Mode = Mode("zzz")
	if err := c.Validate(); err == nil {
		t.Error("Validate should reject unknown Mode")
	}
}

func TestValidate_RejectsEmptyModelByTier(t *testing.T) {
	t.Parallel()
	c := Defaults()
	c.ModelByTier = map[Tier]string{}
	if err := c.Validate(); err == nil {
		t.Error("Validate should reject empty ModelByTier")
	}
}

func TestConfigPath_RespectsEnv(t *testing.T) {
	t.Setenv("CLUE_CODE_CONFIG", "/tmp/custom-clue-code.yaml")
	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if got != "/tmp/custom-clue-code.yaml" {
		t.Errorf("ConfigPath = %q, want override", got)
	}
}

func TestConfigPath_FallbackToHome(t *testing.T) {
	t.Setenv("CLUE_CODE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if got == "" {
		t.Error("ConfigPath returned empty string")
	}
}
