package skillrunner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/state"
)

// stubClient is a model.Client that returns a fixed sequence of chunks.
type stubClient struct {
	chunks  []model.Chunk
	modelID string // set to test that RealRunner propagates Model to ChatRequest
	// block causes ChatStream to wait until ctx is cancelled before sending anything.
	block bool
	// lastReq captures the most recent ChatRequest for assertion.
	lastReq model.ChatRequest
}

func (s *stubClient) Provider() string { return "stub" }

// ModelID satisfies the optional interface checked by NewRealRunner so that
// RealRunner.modelID is populated and propagated to ChatRequest.Model.
func (s *stubClient) ModelID() string { return s.modelID }

func (s *stubClient) Chat(_ context.Context, _ model.ChatRequest) (model.Response, error) {
	return model.Response{}, nil
}

func (s *stubClient) ChatStream(ctx context.Context, req model.ChatRequest) (<-chan model.Chunk, error) {
	s.lastReq = req
	ch := make(chan model.Chunk, len(s.chunks)+1)
	go func() {
		defer close(ch)
		if s.block {
			<-ctx.Done()
			return
		}
		for _, c := range s.chunks {
			select {
			case <-ctx.Done():
				return
			case ch <- c:
			}
		}
	}()
	return ch, nil
}

// openTestStore creates a temp dir that looks like a project root and returns
// a Store and the temp dir path.
func openTestStore(t *testing.T, sessionID string) (state.Store, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".clue-code"), 0o700); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	st, err := state.Open(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return st, dir
}

// readTranscript reads NDJSON transcript entries from the session dir rooted at projectDir.
func readTranscript(t *testing.T, projectDir string) []TranscriptEntry {
	t.Helper()
	sessionsDir := filepath.Join(projectDir, ".clue-code", "state", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatalf("read sessions dir: %v", err)
	}
	var result []TranscriptEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(sessionsDir, e.Name(), "transcript")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read transcript: %v", err)
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			var entry TranscriptEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("unmarshal transcript line %q: %v", line, err)
			}
			result = append(result, entry)
		}
	}
	return result
}

func TestRealRunner_StreamsToStdout(t *testing.T) {
	sessionID := "skill-test-0"
	st, _ := openTestStore(t, sessionID)

	chunks := []model.Chunk{
		{Delta: "hello "},
		{Delta: "world"},
		{Delta: "!", Done: true, Usage: &model.Usage{TotalTokens: 10}},
	}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	skill := &Skill{Name: "test", Body: "system body"}
	err := runner.Run(context.Background(), skill, []string{"arg1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "hello ") {
		t.Errorf("output missing 'hello ': %q", got)
	}
	if !strings.Contains(got, "world") {
		t.Errorf("output missing 'world': %q", got)
	}
	if !strings.Contains(got, "!") {
		t.Errorf("output missing '!': %q", got)
	}
}

func TestRealRunner_PersistsTranscript(t *testing.T) {
	sessionID := "skill-test-0"
	st, projectDir := openTestStore(t, sessionID)

	chunks := []model.Chunk{
		{Delta: "a"},
		{Delta: "b"},
		{Delta: "c", Done: true, Usage: &model.Usage{TotalTokens: 5}},
	}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	skill := &Skill{Name: "test", Body: "body"}
	if err := runner.Run(context.Background(), skill, []string{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries := readTranscript(t, projectDir)
	// system + user + 3 chunk entries = 5 minimum
	if len(entries) < 5 {
		t.Fatalf("transcript: want >=5 entries (system+user+3 chunks), got %d", len(entries))
	}

	// Last entry must have Done=true
	last := entries[len(entries)-1]
	if !last.Done {
		t.Errorf("last transcript entry: want Done=true, got false")
	}
}

func TestRealRunner_HonorsCtxCancel(t *testing.T) {
	sessionID := "skill-cancel-0"
	st, _ := openTestStore(t, sessionID)

	client := &stubClient{block: true}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	skill := &Skill{Name: "cancel", Body: "body"}
	start := time.Now()
	err := runner.Run(ctx, skill, nil)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("Run took %v, want <=200ms", elapsed)
	}
	if err == nil {
		t.Error("Run: want error on cancel, got nil")
	}
}

// TestRealRunner_SetsModelField asserts that RealRunner populates ChatRequest.Model
// from the client's ModelID so the Anthropic API never receives an empty model field
// (which causes HTTP 400). This is the regression test for P0-5.
func TestRealRunner_SetsModelField(t *testing.T) {
	sessionID := "skill-model-0"
	st, _ := openTestStore(t, sessionID)

	wantModel := "anthropic/claude-sonnet-4-5"
	client := &stubClient{
		modelID: wantModel,
		chunks:  []model.Chunk{{Delta: "ok", Done: true}},
	}

	var out strings.Builder
	runner := NewRealRunner(client, st, nil, &out)

	skill := &Skill{Name: "modeltest", Body: "body"}
	if err := runner.Run(context.Background(), skill, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if client.lastReq.Model != wantModel {
		t.Errorf("ChatRequest.Model = %q, want %q", client.lastReq.Model, wantModel)
	}
}

func TestRealRunner_TemplateRendering(t *testing.T) {
	sessionID := "skill-tmpl-0"
	st, projectDir := openTestStore(t, sessionID)

	chunks := []model.Chunk{{Delta: "ok", Done: true}}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	skill := &Skill{Name: "tmpl", Body: "skill={{.SkillName}}"}
	if err := runner.Run(context.Background(), skill, []string{"x"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The system prompt sent to the model should have the template rendered.
	// We verify this by reading the transcript system entry.
	entries := readTranscript(t, projectDir)
	var sysEntry *TranscriptEntry
	for i := range entries {
		if entries[i].Role == "system" {
			sysEntry = &entries[i]
			break
		}
	}
	if sysEntry == nil {
		t.Fatal("transcript: no system entry found")
	}
	if sysEntry.Content != "skill=tmpl" {
		t.Errorf("system prompt: want 'skill=tmpl', got %q", sysEntry.Content)
	}
}
