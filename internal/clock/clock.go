// Package clock provides an injectable time source for testable code.
package clock

import "time"

// Clock abstracts time operations so callers can be tested deterministically.
type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
	Sleep(time.Duration)
}

// Real returns a Clock backed by the system clock.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }
func (realClock) Sleep(d time.Duration)           { time.Sleep(d) }

// FakeClock is a manually-advanced Clock for use in tests.
type FakeClock struct {
	now time.Time
}

// Fake returns a *FakeClock initialised to now.
func Fake(now time.Time) *FakeClock { return &FakeClock{now: now} }

// Now returns the fake clock's current time.
func (f *FakeClock) Now() time.Time { return f.now }

// Since returns the elapsed duration since t on the fake clock.
func (f *FakeClock) Since(t time.Time) time.Duration { return f.now.Sub(t) }

// Sleep advances the fake clock by d without blocking.
func (f *FakeClock) Sleep(d time.Duration) { f.now = f.now.Add(d) }

// Advance moves the fake clock forward by d.
func (f *FakeClock) Advance(d time.Duration) { f.now = f.now.Add(d) }
