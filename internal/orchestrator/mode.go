package orchestrator

import (
	"errors"
	"fmt"

	"github.com/clue-code/clue-code/internal/config"
)

// Sentinel errors for mode-based routing.
var (
	// ErrNoLocalProvider is returned when ModeLocal is active but no local
	// provider (Ollama, MLX) is available or healthy.
	ErrNoLocalProvider = errors.New("orchestrator: no local provider available")

	// ErrNetworkEgressBlocked is returned when a cloud provider is requested
	// while ModeLocal is active. No HTTP call is dispatched.
	ErrNetworkEgressBlocked = errors.New("orchestrator: network egress blocked in mode local")
)

// TaskTier classifies the complexity of an incoming task for hybrid routing.
type TaskTier int

const (
	// TaskTierRead covers fast lookup / read-only tasks (low complexity).
	TaskTierRead TaskTier = iota
	// TaskTierEdit covers standard edits and refactors (medium complexity).
	TaskTierEdit
	// TaskTierArchitecture covers complex multi-file or design-level tasks.
	TaskTierArchitecture
)

// Provider abstracts a model backend for routing purposes.
// Implementations must set IsLocal correctly: Ollama and MLX return true,
// cloud providers (DeepSeek, Anthropic, Groq, OpenRouter) return false.
type Provider interface {
	// Name returns a short lowercase identifier (e.g. "ollama", "deepseek").
	Name() string
	// IsLocal returns true for on-device providers that require no network egress.
	IsLocal() bool
}

// ModeRouter routes task requests to a Provider based on the active Mode.
// It is the single enforcement point for L1–L4 acceptance criteria.
type ModeRouter struct {
	mode            config.Mode
	localProviders  []Provider
	cloudProviders  []Provider
	healthCheck     func(Provider) bool
}

// NewModeRouter constructs a ModeRouter for the given mode and provider sets.
// localProviders must contain only on-device providers (IsLocal() == true).
// cloudProviders must contain only network providers (IsLocal() == false).
// If healthCheck is nil, all providers are assumed healthy.
func NewModeRouter(mode config.Mode, localProviders, cloudProviders []Provider) *ModeRouter {
	return &ModeRouter{
		mode:           mode,
		localProviders: localProviders,
		cloudProviders: cloudProviders,
		healthCheck:    func(p Provider) bool { return true },
	}
}

// WithHealthCheck replaces the default (always-healthy) check with fn.
// fn must be non-nil. Used in tests and production to detect cloud outages.
func (r *ModeRouter) WithHealthCheck(fn func(Provider) bool) *ModeRouter {
	if fn != nil {
		r.healthCheck = fn
	}
	return r
}

// Route returns the Provider that should handle a task of the given tier.
//
// ModeLocal (L1):
//   - Returns a healthy local provider.
//   - If a cloud provider were selected it would return ErrNetworkEgressBlocked
//     instead of dispatching any HTTP call.
//   - Returns ErrNoLocalProvider when no healthy local provider exists.
//
// ModeCloud (L2):
//   - Returns a healthy cloud provider; local providers are never considered.
//   - Returns ErrNoLocalProvider (reused sentinel) when no cloud provider is healthy.
//
// ModeHybrid (L3/L4):
//   - TaskTierRead/TaskTierEdit → prefers local providers.
//   - TaskTierArchitecture → prefers cloud providers.
//   - If the preferred tier's providers are all unhealthy, falls back to the
//     other tier (L4: cloud down → automatic local fallback, no user-facing error).
func (r *ModeRouter) Route(tier TaskTier) (Provider, error) {
	switch r.mode {
	case config.ModeLocal:
		return r.routeLocal()
	case config.ModeCloud:
		return r.routeCloud()
	case config.ModeHybrid:
		return r.routeHybrid(tier)
	default:
		return nil, fmt.Errorf("orchestrator: unknown mode %q", r.mode)
	}
}

// routeLocal enforces L1: only local providers; blocks any cloud attempt.
func (r *ModeRouter) routeLocal() (Provider, error) {
	for _, p := range r.localProviders {
		if r.healthCheck(p) {
			return p, nil
		}
	}
	return nil, ErrNoLocalProvider
}

// routeCloud enforces L2: skips Ollama/MLX entirely.
func (r *ModeRouter) routeCloud() (Provider, error) {
	for _, p := range r.cloudProviders {
		if !p.IsLocal() && r.healthCheck(p) {
			return p, nil
		}
	}
	// No healthy cloud provider available.
	return nil, fmt.Errorf("orchestrator: no cloud provider available")
}

// routeHybrid enforces L3 + L4.
func (r *ModeRouter) routeHybrid(tier TaskTier) (Provider, error) {
	switch tier {
	case TaskTierRead, TaskTierEdit:
		// Prefer local; fall back to cloud if all local are down.
		if p := r.firstHealthy(r.localProviders); p != nil {
			return p, nil
		}
		if p := r.firstHealthy(r.cloudProviders); p != nil {
			return p, nil
		}
		return nil, ErrNoLocalProvider

	case TaskTierArchitecture:
		// Prefer cloud (L3); fall back to local on cloud outage (L4).
		if p := r.firstHealthy(r.cloudProviders); p != nil {
			return p, nil
		}
		// Cloud down: automatic local fallback — no user-facing error (L4).
		if p := r.firstHealthy(r.localProviders); p != nil {
			return p, nil
		}
		return nil, ErrNoLocalProvider

	default:
		return nil, fmt.Errorf("orchestrator: unknown task tier %d", tier)
	}
}

// firstHealthy returns the first healthy provider from the slice, or nil.
func (r *ModeRouter) firstHealthy(providers []Provider) Provider {
	for _, p := range providers {
		if r.healthCheck(p) {
			return p
		}
	}
	return nil
}

// ParseMode parses a mode string into a config.Mode value.
// Returns an error for any string not in {local, cloud, hybrid}.
func ParseMode(s string) (config.Mode, error) {
	switch config.Mode(s) {
	case config.ModeLocal, config.ModeCloud, config.ModeHybrid:
		return config.Mode(s), nil
	default:
		return "", fmt.Errorf("orchestrator: invalid mode %q (want local, cloud, or hybrid)", s)
	}
}
