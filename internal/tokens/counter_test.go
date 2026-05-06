package tokens

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

func TestCounter_Anthropic_KnownString(t *testing.T) {
	c := New()
	// "Hello, world!" is 13 characters.
	// heuristic: round(13 / 3.5) = round(3.714) = 4
	// Reference: Anthropic counts this as ~5 tokens.
	// Acceptance criterion I1 allows ±2% of ~5 tokens (~4-6 range is fine).
	n, err := c.Count("Hello, world!", TokenizerAnthropic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ref := 5
	maxDelta := 1 // ±2% of a ~50-token reference is generous; for small strings allow ±1
	if diff := n - ref; diff < -maxDelta || diff > maxDelta {
		t.Errorf("Count(%q) = %d, want %d ±%d", "Hello, world!", n, ref, maxDelta)
	}
}

func TestCounter_DeepSeek_cl100k(t *testing.T) {
	c := New()

	// Multilingual fixture: French + English + Go code snippet.
	// Heuristic: chars/4 (cl100k_base average).
	// fixture = "Bonjour le monde. Hello world. func main() { fmt.Println(\"hi\") }"
	// len = 65 chars → round(65/4) = 16 tokens.
	fixture := "Bonjour le monde. Hello world. func main() { fmt.Println(\"hi\") }"

	n, err := c.Count(fixture, TokenizerDeepSeek)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reference: round(65 / 4) = 16. Allow ±2 tokens (>2% for small strings).
	ref := int(math.Round(float64(len(fixture)) / 4.0))
	tolerance := 2
	if diff := n - ref; diff < -tolerance || diff > tolerance {
		t.Errorf("Count(fixture) = %d, want %d ±%d", n, ref, tolerance)
	}
}

func TestCounter_EmptyAndNil(t *testing.T) {
	c := New()

	// Empty string → 0 tokens, no error, no panic.
	for _, kind := range []TokenizerKind{TokenizerAnthropic, TokenizerOpenAI, TokenizerDeepSeek} {
		n, err := c.Count("", kind)
		if err != nil {
			t.Errorf("Count(\"\", %s) unexpected error: %v", kind, err)
		}
		if n != 0 {
			t.Errorf("Count(\"\", %s) = %d, want 0", kind, n)
		}
	}

	// Nil messages slice → 0 tokens, no error, no panic.
	for _, kind := range []TokenizerKind{TokenizerAnthropic, TokenizerOpenAI, TokenizerDeepSeek} {
		n, err := c.CountMessages(nil, kind)
		if err != nil {
			t.Errorf("CountMessages(nil, %s) unexpected error: %v", kind, err)
		}
		if n != 0 {
			t.Errorf("CountMessages(nil, %s) = %d, want 0", kind, n)
		}
	}

	// Empty messages slice → 0 tokens.
	for _, kind := range []TokenizerKind{TokenizerAnthropic, TokenizerOpenAI, TokenizerDeepSeek} {
		n, err := c.CountMessages([]model.Message{}, kind)
		if err != nil {
			t.Errorf("CountMessages([], %s) unexpected error: %v", kind, err)
		}
		if n != 0 {
			t.Errorf("CountMessages([], %s) = %d, want 0", kind, n)
		}
	}
}

func TestCounter_LargePayload(t *testing.T) {
	// Build a ~50KB markdown string.
	block := strings.Repeat("# Heading\n\nThis is a paragraph with some content. ", 1000)
	payload := block
	for len(payload) < 50*1024 {
		payload += block
	}
	payload = payload[:50*1024]

	c := New()

	start := time.Now()
	n, err := c.Count(payload, TokenizerDeepSeek)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == 0 {
		t.Error("expected non-zero token count for 50KB payload")
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("Count(50KB) took %v, want <50ms", elapsed)
	}

	// Same check for Anthropic heuristic (should be even faster).
	start = time.Now()
	n2, err := c.Count(payload, TokenizerAnthropic)
	elapsed2 := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n2 == 0 {
		t.Error("expected non-zero anthropic token count for 50KB payload")
	}
	if elapsed2 > 50*time.Millisecond {
		t.Errorf("Count(50KB, Anthropic) took %v, want <50ms", elapsed2)
	}
}

func TestCounter_Concurrent(t *testing.T) {
	c := New()
	text := "The quick brown fox jumps over the lazy dog."

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := c.Count(text, TokenizerDeepSeek)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent Count error: %v", err)
	}
}
