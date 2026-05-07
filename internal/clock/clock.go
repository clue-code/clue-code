// Package clock provides an injectable time source for testable code.
package clock

import (
	"sync"
	"time"
)

// Ticker is the interface returned by Clock.NewTicker. It mirrors the subset
// of time.Ticker used by callers: C() for the channel and Stop() to release
// resources.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// Clock abstracts time operations so callers can be tested deterministically.
type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
	Sleep(time.Duration)
	// NewTicker returns a Ticker that fires approximately every d.
	NewTicker(d time.Duration) Ticker
}

// Real returns a Clock backed by the system clock.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }
func (realClock) Sleep(d time.Duration)           { time.Sleep(d) }
func (realClock) NewTicker(d time.Duration) Ticker {
	t := time.NewTicker(d)
	return &realTicker{t: t}
}

type realTicker struct{ t *time.Ticker }

func (r *realTicker) C() <-chan time.Time { return r.t.C }
func (r *realTicker) Stop()               { r.t.Stop() }

// FakeClock is a manually-advanced Clock for use in tests.
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*FakeTicker
}

// Fake returns a *FakeClock initialised to now.
func Fake(now time.Time) *FakeClock { return &FakeClock{now: now} }

// Now returns the fake clock's current time.
func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Since returns the elapsed duration since t on the fake clock.
func (f *FakeClock) Since(t time.Time) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now.Sub(t)
}

// Sleep advances the fake clock by d without blocking.
func (f *FakeClock) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Advance moves the fake clock forward by d and fires any FakeTickers whose
// interval has elapsed.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	now := f.now
	tickers := make([]*FakeTicker, len(f.tickers))
	copy(tickers, f.tickers)
	f.mu.Unlock()

	for _, tk := range tickers {
		tk.tick(now)
	}
}

// NewTicker returns a FakeTicker that fires when Advance moves time past its
// interval boundary. The ticker is registered with this FakeClock.
func (f *FakeClock) NewTicker(d time.Duration) Ticker {
	f.mu.Lock()
	defer f.mu.Unlock()
	tk := &FakeTicker{
		d:       d,
		ch:      make(chan time.Time, 1),
		started: f.now,
	}
	f.tickers = append(f.tickers, tk)
	return tk
}

// FakeTicker is a test Ticker whose channel is driven by FakeClock.Advance.
type FakeTicker struct {
	mu      sync.Mutex
	d       time.Duration
	ch      chan time.Time
	started time.Time
	last    time.Time
	stopped bool
}

// tick fires if now is at least one interval past the last fire time.
func (ft *FakeTicker) tick(now time.Time) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	if ft.stopped {
		return
	}
	base := ft.last
	if base.IsZero() {
		base = ft.started
	}
	for !now.Before(base.Add(ft.d)) {
		base = base.Add(ft.d)
		// Non-blocking send — drop if buffer full (mirrors time.Ticker behaviour).
		select {
		case ft.ch <- base:
		default:
		}
		ft.last = base
	}
}

// C returns the ticker channel.
func (ft *FakeTicker) C() <-chan time.Time { return ft.ch }

// Stop marks the ticker as stopped; no further ticks will be delivered.
func (ft *FakeTicker) Stop() {
	ft.mu.Lock()
	ft.stopped = true
	ft.mu.Unlock()
}
