package team

import (
	"errors"
	"strings"
	"testing"
)

func TestForkBomb_MaxWorkers(t *testing.T) {
	t.Parallel()
	err := CheckTeamSize(25)
	if err == nil {
		t.Fatal("expected ErrTooManyWorkers, got nil")
	}
	if !errors.Is(err, ErrTooManyWorkers) {
		t.Errorf("expected ErrTooManyWorkers, got: %v", err)
	}
}

func TestForkBomb_MaxWorkersOK(t *testing.T) {
	t.Parallel()
	if err := CheckTeamSize(20); err != nil {
		t.Errorf("CheckTeamSize(20) = %v, want nil", err)
	}
	if err := CheckTeamSize(1); err != nil {
		t.Errorf("CheckTeamSize(1) = %v, want nil", err)
	}
}

func TestForkBomb_DepthCap(t *testing.T) {
	t.Setenv(TeamDepthEnvVar, "1")
	err := CheckDepth()
	if err == nil {
		t.Fatal("expected ErrTeamDepthExceeded, got nil")
	}
	if !errors.Is(err, ErrTeamDepthExceeded) {
		t.Errorf("expected ErrTeamDepthExceeded, got: %v", err)
	}
}

func TestForkBomb_DepthOK(t *testing.T) {
	t.Setenv(TeamDepthEnvVar, "")
	if err := CheckDepth(); err != nil {
		t.Errorf("CheckDepth() with empty env = %v, want nil", err)
	}
}

func TestForkBomb_IncrementEnv(t *testing.T) {
	t.Run("from_empty", func(t *testing.T) {
		t.Setenv(TeamDepthEnvVar, "")
		result := IncrementedDepthEnv()
		want := TeamDepthEnvVar + "=1"
		if !containsEntry(result, want) {
			t.Errorf("IncrementedDepthEnv() = %v, want entry %q", result, want)
		}
	})

	t.Run("from_two", func(t *testing.T) {
		t.Setenv(TeamDepthEnvVar, "2")
		result := IncrementedDepthEnv()
		want := TeamDepthEnvVar + "=3"
		if !containsEntry(result, want) {
			t.Errorf("IncrementedDepthEnv() = %v, want entry %q", result, want)
		}
	})
}

func containsEntry(env []string, entry string) bool {
	for _, e := range env {
		if strings.EqualFold(e, entry) || e == entry {
			return true
		}
	}
	return false
}
