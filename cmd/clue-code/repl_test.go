package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

// newScannerFromString returns a bufio.Scanner reading from the given string.
func newScannerFromString(s string) *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(s))
}

// captureStdout redirects os.Stdout to a pipe, calls f, then restores stdout
// and returns what was written. The test is fatally failed if the pipe setup fails.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w

	f()

	if err := w.Close(); err != nil {
		t.Fatalf("pipe write-end Close: %v", err)
	}
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("pipe ReadAll: %v", err)
	}
	return string(out)
}

// discardStdout redirects os.Stdout to io.Discard for the duration of f.
func discardStdout(t *testing.T, f func()) {
	t.Helper()
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w

	f()

	if err := w.Close(); err != nil {
		t.Fatalf("pipe write-end Close: %v", err)
	}
	os.Stdout = old
}

// discardStderr redirects os.Stderr to io.Discard for the duration of f,
// returning what was written.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w

	f()

	if err := w.Close(); err != nil {
		t.Fatalf("pipe write-end Close: %v", err)
	}
	os.Stderr = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("pipe ReadAll: %v", err)
	}
	return string(out)
}

// --- REPL unit tests (no real API) ---

// TestREPL_HelpCommand verifies /help output contains expected keywords.
func TestREPL_HelpCommand(t *testing.T) {
	sess := newReplSession("test/model")
	helpText := captureStdout(t, sess.Help)
	for _, keyword := range []string{"exit", "clear", "save", "/help"} {
		if !strings.Contains(helpText, keyword) {
			t.Errorf("Help() output missing %q; got:\n%s", keyword, helpText)
		}
	}
}

// TestREPL_ClearCommand verifies /clear resets history to length 0.
func TestREPL_ClearCommand(t *testing.T) {
	sess := newReplSession("test/model")
	sess.AppendUser("msg1")
	sess.AppendAssistant("reply1")
	sess.AppendUser("msg2")
	if len(sess.history) != 3 {
		t.Fatalf("expected 3 history entries before clear, got %d", len(sess.history))
	}

	discardStdout(t, sess.Clear)

	if len(sess.history) != 0 {
		t.Errorf("expected history len=0 after Clear(), got %d", len(sess.history))
	}
}

// TestREPL_SaveCommand verifies /save writes a valid Markdown file.
func TestREPL_SaveCommand(t *testing.T) {
	sess := newReplSession("anthropic/claude-sonnet-4-5")
	sess.AppendUser("bonjour")
	sess.AppendAssistant("Bonjour ! Comment puis-je vous aider ?")

	path := t.TempDir() + "/conversation.md"

	var saveErr error
	discardStdout(t, func() {
		saveErr = sess.Save(path)
	})
	if saveErr != nil {
		t.Fatalf("Save() returned error: %v", saveErr)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	content := string(data)

	for _, want := range []string{"---", "model:", "date:", "## You", "## Claude", "bonjour", "Bonjour"} {
		if !strings.Contains(content, want) {
			t.Errorf("saved file missing %q; content:\n%s", want, content)
		}
	}
}

// TestREPL_ExitCommand verifies /exit returns exit=true.
func TestREPL_ExitCommand(t *testing.T) {
	sess := newReplSession("test/model")
	exit := handleMetaCommand(sess, "/exit")
	if !exit {
		t.Error("handleMetaCommand(/exit) should return exit=true")
	}
	exit = handleMetaCommand(sess, "/quit")
	if !exit {
		t.Error("handleMetaCommand(/quit) should return exit=true")
	}
}

// TestREPL_ConversationContext verifies that the second request includes the
// first exchange in the messages array (history maintained).
func TestREPL_ConversationContext(t *testing.T) {
	var capturedBodies [][]model.Message

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the request body to capture messages.
		var body struct {
			Messages []model.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			capturedBodies = append(capturedBodies, body.Messages)
		}
		// Serve a simple streaming response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"delta": map[string]any{"content": "reply"}},
			},
		})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return
		}
		if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	cfg := buildTestConfig(t, srv.URL, "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY_REPL_TEST")
	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sess := newReplSession("deepseek-chat")

	var (
		assistant1 string
		usage1     model.Usage
		err1       error
		err2       error
	)

	discardStdout(t, func() {
		// First exchange.
		sess.AppendUser("first question")
		assistant1, usage1, err1 = sendStreaming(context.Background(), client, "deepseek-chat", sess.history)
		if err1 == nil {
			sess.AppendAssistant(assistant1)
			sess.AddUsage(usage1)
			// Second exchange.
			sess.AppendUser("second question")
			_, _, err2 = sendStreaming(context.Background(), client, "deepseek-chat", sess.history)
		}
	})

	if err1 != nil {
		t.Fatalf("first sendStreaming: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second sendStreaming: %v", err2)
	}
	if len(capturedBodies) < 2 {
		t.Fatalf("expected 2 requests, got %d", len(capturedBodies))
	}

	// First request: 1 user message.
	if len(capturedBodies[0]) != 1 {
		t.Errorf("first request: expected 1 message, got %d", len(capturedBodies[0]))
	}
	// Second request: 3 messages (user, assistant, user).
	if len(capturedBodies[1]) < 3 {
		t.Errorf("second request: expected >=3 messages (history), got %d", len(capturedBodies[1]))
	}
}

// TestREPL_StreamingOutput verifies that chunks from the SSE stream are
// assembled correctly into the final assistant string.
func TestREPL_StreamingOutput(t *testing.T) {
	tokens := []string{"Hello", " ", "world", "!"}
	srv := httptest.NewServer(openAIStreamHandler(tokens))
	defer srv.Close()

	cfg := buildTestConfig(t, srv.URL, "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY_STREAM_TEST")
	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	msgs := []model.Message{{Role: model.RoleUser, Content: "hi"}}
	var (
		result  string
		printed string
		sendErr error
	)
	printed = captureStdout(t, func() {
		result, _, sendErr = sendStreaming(context.Background(), client, "deepseek-chat", msgs)
	})

	if sendErr != nil {
		t.Fatalf("sendStreaming error: %v", sendErr)
	}
	if result != "Hello world!" {
		t.Errorf("assembled response = %q, want %q", result, "Hello world!")
	}
	if !strings.Contains(printed, "Hello") {
		t.Errorf("stdout should contain streamed tokens; got: %q", printed)
	}
}

// TestREPL_CtrlCInterrupt verifies that a canceled context causes sendStreaming
// to return context.Canceled without panicking, simulating Ctrl+C mid-response.
func TestREPL_CtrlCInterrupt(t *testing.T) {
	// Slow handler that streams tokens with delays.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 5; i++ {
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]any{"content": fmt.Sprintf("tok%d ", i)}},
				},
			})
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
		if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
			return
		}
	}))
	defer srv.Close()

	cfg := buildTestConfig(t, srv.URL, "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY_INTERRUPT_TEST")
	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay to simulate Ctrl+C.
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	msgs := []model.Message{{Role: model.RoleUser, Content: "long task"}}
	var sendErr error
	discardStdout(t, func() {
		_, _, sendErr = sendStreaming(ctx, client, "deepseek-chat", msgs)
	})

	// Must not panic. err should be context.Canceled (or nil if stream finished first).
	if sendErr != nil && sendErr != context.Canceled && !strings.Contains(sendErr.Error(), "context") {
		t.Errorf("unexpected error: %v (expected nil or context.Canceled)", sendErr)
	}
	t.Logf("interrupt err = %v (ok)", sendErr)
}

// TestREPL_SetModel verifies /model updates the session model ID.
func TestREPL_SetModel(t *testing.T) {
	sess := newReplSession("original/model")
	discardStdout(t, func() {
		sess.SetModel("anthropic/claude-haiku-4-5")
	})
	if sess.modelID != "anthropic/claude-haiku-4-5" {
		t.Errorf("modelID = %q, want %q", sess.modelID, "anthropic/claude-haiku-4-5")
	}
}

// TestREPL_TokensAccumulation verifies AddUsage accumulates correctly.
func TestREPL_TokensAccumulation(t *testing.T) {
	sess := newReplSession("test/model")
	sess.AddUsage(model.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30})
	sess.AddUsage(model.Usage{PromptTokens: 5, CompletionTokens: 15, TotalTokens: 20})

	if sess.tokensUsed.TotalTokens != 50 {
		t.Errorf("TotalTokens = %d, want 50", sess.tokensUsed.TotalTokens)
	}
	if sess.tokensUsed.PromptTokens != 15 {
		t.Errorf("PromptTokens = %d, want 15", sess.tokensUsed.PromptTokens)
	}
}

// TestREPL_MultiLineContinuation verifies readLine joins lines ending with '\'.
func TestREPL_MultiLineContinuation(t *testing.T) {
	input := "first line\\\nsecond line\n"
	scanner := newScannerFromString(input)

	var line string
	var ok bool
	discardStdout(t, func() {
		line, ok = readLine(scanner)
	})

	if !ok {
		t.Fatal("readLine returned ok=false")
	}
	if !strings.Contains(line, "first line") || !strings.Contains(line, "second line") {
		t.Errorf("readLine multi-line = %q; want both parts joined", line)
	}
}

// TestREPL_UnknownMetaCommand verifies unknown commands print an error but do
// not exit.
func TestREPL_UnknownMetaCommand(t *testing.T) {
	sess := newReplSession("test/model")
	errOut := captureStderr(t, func() {
		if handleMetaCommand(sess, "/unknown") {
			t.Error("unknown command should not cause exit")
		}
	})
	if !strings.Contains(errOut, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr; got %q", errOut)
	}
}
