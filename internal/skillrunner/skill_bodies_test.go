package skillrunner

// skill_bodies_test.go — template parsing + stub-model flow tests for skill bodies.
// worker-2 owns the top half (autopilot + ralph tests).
// worker-3 appended the bottom half: ccg + cancel tests (below).

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"text/template"

	"github.com/clue-code/clue-code/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers shared by both halves
// ---------------------------------------------------------------------------

// loadSkillFromDir loads a named skill from the real skills/ directory tree.
func loadSkillFromDir(t *testing.T, name string) *Skill {
	t.Helper()
	e := NewEngine(nil)
	if err := e.Load("../../skills"); err != nil {
		_ = err // non-fatal; other skills may have load issues
	}
	s, ok := e.skills[name]
	if !ok {
		t.Fatalf("loadSkillFromDir: skill %q not found in ../../skills", name)
	}
	return s
}

// ---------------------------------------------------------------------------
// worker-2: autopilot + ralph skill body tests
// ---------------------------------------------------------------------------

// TestAutopilotBody_ParsesAsTemplate verifies that autopilot/SKILL.md body is
// valid Go text/template syntax and contains required template variables.
func TestAutopilotBody_ParsesAsTemplate(t *testing.T) {
	skill := loadSkillFromDir(t, "autopilot")

	if skill.Body == "" {
		t.Fatal("autopilot body is empty")
	}
	if !strings.Contains(skill.Body, "{{") {
		t.Fatal("autopilot body has no template actions ({{ }})")
	}
	if _, err := template.New("autopilot").Parse(skill.Body); err != nil {
		t.Fatalf("autopilot body template parse error: %v", err)
	}

	for _, want := range []string{".SessionID", ".ProjectRoot", ".SkillArgs"} {
		if !strings.Contains(skill.Body, want) {
			t.Errorf("autopilot body: missing required template variable %q", want)
		}
	}
	for _, section := range []string{"Phase 0", "Phase 1", "Phase 2", "Phase 3", "Phase 4", "Phase 5"} {
		if !strings.Contains(skill.Body, section) {
			t.Errorf("autopilot body: missing section %q", section)
		}
	}
}

// TestAutopilot_StubModelFlow verifies end-to-end flow for autopilot:
// load real skill body, render template with SkillContext, call stub model,
// receive output, check transcript. No live API calls are made.
func TestAutopilot_StubModelFlow(t *testing.T) {
	skill := loadSkillFromDir(t, "autopilot")

	sessionID := "skill-autopilot-0"
	st, projectDir := openTestStore(t, sessionID)

	chunks := []model.Chunk{
		{Delta: "spec drafted", Done: true, Usage: &model.Usage{TotalTokens: 5}},
	}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	err := runner.Run(context.Background(), skill, []string{"build a hello world in Go"})
	if err != nil {
		t.Fatalf("autopilot stub flow: Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "spec drafted") {
		t.Errorf("autopilot stub flow: want 'spec drafted' in output, got %q", out.String())
	}

	entries := readTranscript(t, projectDir)
	var sysEntry *TranscriptEntry
	for i := range entries {
		if entries[i].Role == "system" {
			sysEntry = &entries[i]
			break
		}
	}
	if sysEntry == nil {
		t.Fatal("autopilot stub flow: no system transcript entry found")
	}
	if strings.Contains(sysEntry.Content, "{{.SessionID}}") {
		t.Error("autopilot stub flow: system prompt contains unrendered {{.SessionID}}")
	}
	if !strings.Contains(sysEntry.Content, sessionID) {
		t.Errorf("autopilot stub flow: system prompt does not contain rendered sessionID %q", sessionID)
	}
}

// TestRalphBody_ParsesAsTemplate verifies that ralph/SKILL.md body is valid
// Go text/template syntax and contains required template variables.
func TestRalphBody_ParsesAsTemplate(t *testing.T) {
	skill := loadSkillFromDir(t, "ralph")

	if skill.Body == "" {
		t.Fatal("ralph body is empty")
	}
	if !strings.Contains(skill.Body, "{{") {
		t.Fatal("ralph body has no template actions ({{ }})")
	}
	if _, err := template.New("ralph").Parse(skill.Body); err != nil {
		t.Fatalf("ralph body template parse error: %v", err)
	}

	for _, want := range []string{".SessionID", ".ProjectRoot", ".SkillArgs"} {
		if !strings.Contains(skill.Body, want) {
			t.Errorf("ralph body: missing required template variable %q", want)
		}
	}
	for _, section := range []string{"Step 1", "Step 2", "Step 3", "Step 7", "prd.json"} {
		if !strings.Contains(skill.Body, section) {
			t.Errorf("ralph body: missing section/keyword %q", section)
		}
	}
}

// TestRalph_StubModelFlow verifies end-to-end flow for ralph:
// load real skill body, render template, call stub model, receive output,
// check transcript. No live API calls are made.
func TestRalph_StubModelFlow(t *testing.T) {
	skill := loadSkillFromDir(t, "ralph")

	sessionID := "skill-ralph-0"
	st, projectDir := openTestStore(t, sessionID)

	chunks := []model.Chunk{
		{Delta: "prd.json refined", Done: true, Usage: &model.Usage{TotalTokens: 5}},
	}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	err := runner.Run(context.Background(), skill, []string{"fix all lint issues"})
	if err != nil {
		t.Fatalf("ralph stub flow: Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "prd.json refined") {
		t.Errorf("ralph stub flow: want 'prd.json refined' in output, got %q", out.String())
	}

	entries := readTranscript(t, projectDir)
	var sysEntry *TranscriptEntry
	for i := range entries {
		if entries[i].Role == "system" {
			sysEntry = &entries[i]
			break
		}
	}
	if sysEntry == nil {
		t.Fatal("ralph stub flow: no system transcript entry found")
	}
	if strings.Contains(sysEntry.Content, "{{.SessionID}}") {
		t.Error("ralph stub flow: system prompt contains unrendered {{.SessionID}}")
	}
	if !strings.Contains(sysEntry.Content, sessionID) {
		t.Errorf("ralph stub flow: system prompt does not contain rendered sessionID %q", sessionID)
	}

	var userEntry *TranscriptEntry
	for i := range entries {
		if entries[i].Role == "user" {
			userEntry = &entries[i]
			break
		}
	}
	if userEntry == nil {
		t.Fatal("ralph stub flow: no user transcript entry found")
	}
	if !strings.Contains(userEntry.Content, "lint") {
		t.Errorf("ralph stub flow: user transcript entry does not contain task args, got %q", userEntry.Content)
	}
}

// ---------------------------------------------------------------------------
// worker-3: ccg + cancel skill body tests
// ---------------------------------------------------------------------------

// TestCCGBody_ParsesAsTemplate verifies the ccg SKILL.md body loads and
// renders without template errors. The body uses {{range .SkillArgs}} so
// we exercise both the zero-arg and one-arg paths.
func TestCCGBody_ParsesAsTemplate(t *testing.T) {
	skill := loadSkillFromDir(t, "ccg")

	// Zero args — template must render without error.
	out, err := RenderSkillPrompt(skill.Body, SkillContext{
		SkillName: "ccg",
		SkillArgs: []string{},
	})
	if err != nil {
		t.Fatalf("RenderSkillPrompt (no args): %v", err)
	}
	if !strings.Contains(out, "Reviewer A") {
		t.Errorf("rendered body missing 'Reviewer A': %q", out[:min(len(out), 200)])
	}

	// One arg — template renders task arg into body.
	out2, err := RenderSkillPrompt(skill.Body, SkillContext{
		SkillName: "ccg",
		SkillArgs: []string{"review this PR"},
	})
	if err != nil {
		t.Fatalf("RenderSkillPrompt (with arg): %v", err)
	}
	if !strings.Contains(out2, "review this PR") {
		t.Errorf("rendered body missing injected arg: %q", out2[:min(len(out2), 200)])
	}
}

// TestCancelBody_ParsesAsTemplate verifies the cancel SKILL.md body loads and
// renders without template errors. Cancel body has no template variables so
// it should be returned as-is.
func TestCancelBody_ParsesAsTemplate(t *testing.T) {
	skill := loadSkillFromDir(t, "cancel")

	out, err := RenderSkillPrompt(skill.Body, SkillContext{
		SkillName: "cancel",
		SkillArgs: []string{},
	})
	if err != nil {
		t.Fatalf("RenderSkillPrompt: %v", err)
	}
	if !strings.Contains(out, "state_clear") {
		t.Errorf("cancel body missing 'state_clear': %q", out[:min(len(out), 200)])
	}
	if !strings.Contains(out, "state_list_active") {
		t.Errorf("cancel body missing 'state_list_active': %q", out[:min(len(out), 200)])
	}
}

// TestCCG_StubModelFlow runs the ccg skill body through a RealRunner backed by
// a stub model client. Verifies: template renders, model is called, output
// contains stub delta, transcript is persisted.
func TestCCG_StubModelFlow(t *testing.T) {
	// Load skill before openTestStore changes cwd.
	skill := loadSkillFromDir(t, "ccg")

	sessionID := "skill-ccg-0"
	st, projectDir := openTestStore(t, sessionID)

	chunks := []model.Chunk{
		{Delta: "### Reviewer A\nLooks good.\n"},
		{Delta: "### Synthesis\nAll agree.\n"},
		{Delta: "", Done: true, Usage: &model.Usage{TotalTokens: 20}},
	}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	err := runner.Run(context.Background(), skill, []string{"review this API"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// stdout must contain stub deltas.
	got := out.String()
	if !strings.Contains(got, "Reviewer A") {
		t.Errorf("output missing 'Reviewer A': %q", got)
	}
	if !strings.Contains(got, "Synthesis") {
		t.Errorf("output missing 'Synthesis': %q", got)
	}

	// Transcript must be persisted (system + user + chunks).
	entries := readTranscript(t, projectDir)
	if len(entries) < 3 {
		t.Fatalf("transcript: want >=3 entries, got %d", len(entries))
	}
	last := entries[len(entries)-1]
	if !last.Done {
		t.Errorf("last transcript entry: want Done=true, got false")
	}
}

// TestCancel_StubModelFlow runs the cancel skill body through a RealRunner
// backed by a stub model client. Verifies: body renders, model is called with
// the rendered system prompt, output is streamed, transcript persisted.
func TestCancel_StubModelFlow(t *testing.T) {
	// Load skill before openTestStore changes cwd.
	skill := loadSkillFromDir(t, "cancel")

	sessionID := "skill-cancel-0"
	st, projectDir := openTestStore(t, sessionID)

	chunks := []model.Chunk{
		{Delta: "Cancelling ralph (session: skill-cancel-0)...\n"},
		{Delta: "Ralph cancelled.\n"},
		{Delta: "", Done: true, Usage: &model.Usage{TotalTokens: 15}},
	}
	client := &stubClient{chunks: chunks}

	var out bytes.Buffer
	runner := NewRealRunner(client, st, nil, &out)

	err := runner.Run(context.Background(), skill, []string{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// stdout must contain the structured ack from stub.
	got := out.String()
	if !strings.Contains(got, "Cancelling") {
		t.Errorf("output missing 'Cancelling': %q", got)
	}
	if !strings.Contains(got, "cancelled") {
		t.Errorf("output missing 'cancelled': %q", got)
	}

	// Transcript must have system entry with cancel body content.
	entries := readTranscript(t, projectDir)
	if len(entries) < 3 {
		t.Fatalf("transcript: want >=3 entries, got %d", len(entries))
	}
	var sysEntry *TranscriptEntry
	for i := range entries {
		if entries[i].Role == "system" {
			sysEntry = &entries[i]
			break
		}
	}
	if sysEntry == nil {
		t.Fatal("transcript: no system entry")
	}
	if !strings.Contains(sysEntry.Content, "state_list_active") {
		t.Errorf("system prompt missing 'state_list_active': %q", sysEntry.Content[:min(len(sysEntry.Content), 200)])
	}
}

// min is a local helper for Go <1.21 compat (avoids builtin min dependency).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
