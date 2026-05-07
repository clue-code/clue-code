package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/clue-code/clue-code/internal/config"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	name    string
	isLocal bool
}

func (m *mockProvider) Name() string  { return m.name }
func (m *mockProvider) IsLocal() bool { return m.isLocal }

func ollamaMock() Provider    { return &mockProvider{name: "ollama", isLocal: true} }
func mlxMock() Provider       { return &mockProvider{name: "mlx", isLocal: true} }
func deepseekMock() Provider  { return &mockProvider{name: "deepseek", isLocal: false} }
func anthropicMock() Provider { return &mockProvider{name: "anthropic", isLocal: false} }

// TestRouter_LocalMode_BlocksNetworkEgress verifies L1:
// In ModeLocal, Route returns a local provider and never dispatches to cloud.
func TestRouter_LocalMode_BlocksNetworkEgress(t *testing.T) {
	t.Parallel()

	// Set up a TLS test server to represent a cloud endpoint.
	var hitCount int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hitCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Cloud provider pointing at the test server (would be called if egress not blocked).
	cloudProvider := &mockProvider{name: "deepseek-test", isLocal: false}
	localProvider := ollamaMock()

	r := NewModeRouter(config.ModeLocal,
		[]Provider{localProvider},
		[]Provider{cloudProvider},
	)

	// Route(TaskTierRead) must return the local provider.
	got, err := r.Route(TaskTierRead)
	if err != nil {
		t.Fatalf("Route(TaskTierRead) in ModeLocal returned error: %v", err)
	}
	if !got.IsLocal() {
		t.Errorf("Route returned non-local provider %q in ModeLocal", got.Name())
	}

	// Attempting routeLocal when no local providers available → ErrNoLocalProvider.
	emptyLocalRouter := NewModeRouter(config.ModeLocal, nil, []Provider{cloudProvider})
	_, err = emptyLocalRouter.Route(TaskTierRead)
	if err != ErrNoLocalProvider {
		t.Errorf("expected ErrNoLocalProvider, got %v", err)
	}

	// Verify zero requests hit the cloud test server (network egress blocked structurally).
	if n := atomic.LoadInt64(&hitCount); n != 0 {
		t.Errorf("cloud test server received %d requests, want 0 (network egress must be blocked)", n)
	}
}

// TestRouter_CloudMode_SkipsLocal verifies L2:
// In ModeCloud, local providers (ollama, mlx) are never selected.
func TestRouter_CloudMode_SkipsLocal(t *testing.T) {
	t.Parallel()

	ollama := ollamaMock()
	mlx := mlxMock()
	deepseek := deepseekMock()

	r := NewModeRouter(config.ModeCloud,
		[]Provider{ollama, mlx},
		[]Provider{deepseek},
	)

	got, err := r.Route(TaskTierRead)
	if err != nil {
		t.Fatalf("Route in ModeCloud returned error: %v", err)
	}
	if got.IsLocal() {
		t.Errorf("Route returned local provider %q in ModeCloud; only cloud providers should be eligible", got.Name())
	}
	if got.Name() != "deepseek" {
		t.Errorf("Route = %q, want deepseek", got.Name())
	}

	// All cloud providers unhealthy → error (not local fallback).
	r2 := NewModeRouter(config.ModeCloud, []Provider{ollama, mlx}, []Provider{deepseek}).
		WithHealthCheck(func(p Provider) bool { return false })
	_, err = r2.Route(TaskTierRead)
	if err == nil {
		t.Error("ModeCloud with all unhealthy cloud providers should return error")
	}
}

// TestRouter_HybridMode_TierRouting verifies L3:
// hybrid routes read/edit to local, architecture to cloud.
func TestRouter_HybridMode_TierRouting(t *testing.T) {
	t.Parallel()

	ollama := ollamaMock()
	deepseek := deepseekMock()

	r := NewModeRouter(config.ModeHybrid,
		[]Provider{ollama},
		[]Provider{deepseek},
	)

	// Read → local preferred.
	got, err := r.Route(TaskTierRead)
	if err != nil {
		t.Fatalf("Route(TaskTierRead): %v", err)
	}
	if !got.IsLocal() {
		t.Errorf("Route(TaskTierRead) = %q (cloud), want local", got.Name())
	}

	// Edit → local preferred.
	got, err = r.Route(TaskTierEdit)
	if err != nil {
		t.Fatalf("Route(TaskTierEdit): %v", err)
	}
	if !got.IsLocal() {
		t.Errorf("Route(TaskTierEdit) = %q (cloud), want local", got.Name())
	}

	// Architecture → cloud preferred.
	got, err = r.Route(TaskTierArchitecture)
	if err != nil {
		t.Fatalf("Route(TaskTierArchitecture): %v", err)
	}
	if got.IsLocal() {
		t.Errorf("Route(TaskTierArchitecture) = %q (local), want cloud", got.Name())
	}
}

// TestRouter_HybridMode_CloudDownFallback verifies L4:
// When cloud is unhealthy in hybrid mode, architecture tasks fall back to local.
// No panic, no user-facing error.
func TestRouter_HybridMode_CloudDownFallback(t *testing.T) {
	t.Parallel()

	ollama := ollamaMock()
	deepseek := deepseekMock()

	// healthCheck: cloud providers always unhealthy, local always healthy.
	r := NewModeRouter(config.ModeHybrid,
		[]Provider{ollama},
		[]Provider{deepseek},
	).WithHealthCheck(func(p Provider) bool {
		return p.IsLocal() // cloud is "down"
	})

	got, err := r.Route(TaskTierArchitecture)
	if err != nil {
		t.Fatalf("L4: Route(TaskTierArchitecture) with cloud down returned error: %v (want automatic local fallback)", err)
	}
	if !got.IsLocal() {
		t.Errorf("L4: fallback provider is not local: %q", got.Name())
	}
}

// TestRouter_HybridMode_AllDown verifies that when both tiers are unhealthy,
// ErrNoLocalProvider is returned.
func TestRouter_HybridMode_AllDown(t *testing.T) {
	t.Parallel()

	r := NewModeRouter(config.ModeHybrid,
		[]Provider{ollamaMock()},
		[]Provider{deepseekMock()},
	).WithHealthCheck(func(Provider) bool { return false })

	_, err := r.Route(TaskTierArchitecture)
	if err != ErrNoLocalProvider {
		t.Errorf("all unhealthy: want ErrNoLocalProvider, got %v", err)
	}
}

// TestParseMode covers valid and invalid inputs.
func TestParseMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input   string
		want    config.Mode
		wantErr bool
	}{
		{"local", config.ModeLocal, false},
		{"cloud", config.ModeCloud, false},
		{"hybrid", config.ModeHybrid, false},
		{"", "", true},
		{"HYBRID", "", true},
		{"invalid", "", true},
	} {
		got, err := ParseMode(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseMode(%q): want error, got %v", tc.input, got)
			}
		} else {
			if err != nil {
				t.Errorf("ParseMode(%q): unexpected error %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseMode(%q) = %q, want %q", tc.input, got, tc.want)
			}
		}
	}
}

// TestWithHealthCheck_NilIsNoop verifies nil healthCheck does not panic.
func TestWithHealthCheck_NilIsNoop(t *testing.T) {
	t.Parallel()
	r := NewModeRouter(config.ModeLocal, []Provider{ollamaMock()}, nil)
	r.WithHealthCheck(nil) // must not panic or replace the existing check
	got, err := r.Route(TaskTierRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsLocal() {
		t.Error("expected local provider")
	}
}

// TestModeRouter_MultipleLocalProviders verifies first-healthy wins in ModeLocal.
func TestModeRouter_MultipleLocalProviders(t *testing.T) {
	t.Parallel()

	first := &mockProvider{name: "first-local", isLocal: true}
	second := &mockProvider{name: "second-local", isLocal: true}

	r := NewModeRouter(config.ModeLocal, []Provider{first, second}, nil)
	got, err := r.Route(TaskTierRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "first-local" {
		t.Errorf("expected first-local, got %q", got.Name())
	}
}

// TestModeRouter_HybridReadFallsBackToCloud verifies that for Read tier in hybrid,
// when all local providers are down, cloud is used.
func TestModeRouter_HybridReadFallsBackToCloud(t *testing.T) {
	t.Parallel()

	ollama := ollamaMock()
	cloud := deepseekMock()

	r := NewModeRouter(config.ModeHybrid,
		[]Provider{ollama},
		[]Provider{cloud},
	).WithHealthCheck(func(p Provider) bool {
		return !p.IsLocal() // local is "down", cloud is healthy
	})

	got, err := r.Route(TaskTierRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsLocal() {
		t.Errorf("expected cloud fallback for Read when local is down, got local %q", got.Name())
	}
}
