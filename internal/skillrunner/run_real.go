package skillrunner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/clue-code/clue-code/internal/hooks"
	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/state"
)

// Runner is the interface for executing a skill body.
type Runner interface {
	Run(ctx context.Context, skill *Skill, args []string) error
}

// RealRunner executes a skill by rendering its SKILL.md body as a system
// prompt, calling the model, streaming output, persisting the transcript,
// and firing mid-run lifecycle hooks (UserPromptSubmit, PreToolUse,
// PostToolUse). The engine wraps the call with SessionStart/Stop.
type RealRunner struct {
	modelClient model.Client
	store       state.Store
	hm          *hooks.Manager // nil-safe: hooks are no-op when nil
	out         io.Writer
}

// NewRealRunner constructs a RealRunner.
//   - hm may be nil (mid-run hooks become no-op).
//   - out defaults to os.Stdout when nil.
func NewRealRunner(c model.Client, s state.Store, hm *hooks.Manager, out io.Writer) *RealRunner {
	if out == nil {
		out = os.Stdout
	}
	return &RealRunner{
		modelClient: c,
		store:       s,
		hm:          hm,
		out:         out,
	}
}

// Run executes the skill: render prompt → fire UserPromptSubmit + PreToolUse
// → model stream → write stdout → persist transcript → fire PostToolUse.
//
// SessionStart and Stop are fired by Engine.Run around this method, so the
// resulting hook order on a successful run is:
//
//	SessionStart → UserPromptSubmit → PreToolUse → [stream] → PostToolUse → Stop
func (r *RealRunner) Run(ctx context.Context, skill *Skill, args []string) (retErr error) {
	// Match Engine.Run's sessionID format (engine.go:81) so transcript and
	// hooks-log share the same session key. Depth comes from the same
	// CLUE_CODE_SKILL_DEPTH env the engine consulted just before invoking us.
	depth := readSkillDepthEnv()
	sessionID := fmt.Sprintf("skill-%s-%d", skill.Name, depth)

	projectRoot, _ := os.Getwd()

	sctx := SkillContext{
		SkillName:   skill.Name,
		SkillArgs:   args,
		ProjectRoot: projectRoot,
		SessionID:   sessionID,
		Now:         time.Now(),
		UserShell:   os.Getenv("SHELL"),
	}

	systemPrompt, err := RenderSkillPrompt(skill.Body, sctx)
	if err != nil {
		return fmt.Errorf("skillrunner: render prompt: %w", err)
	}

	userContent := strings.Join(args, " ")

	// Fire UserPromptSubmit before any model interaction.
	if ferr := fireLifecycle(ctx, r.hm, hooks.EventUserPromptSubmit, sessionID, skill.Name, args); ferr != nil {
		return ferr
	}

	// Persist system and user transcript entries.
	if err := PersistEntry(ctx, r.store, sessionID, TranscriptEntry{
		Timestamp: time.Now(),
		Role:      "system",
		Content:   systemPrompt,
	}); err != nil {
		return fmt.Errorf("skillrunner: persist system: %w", err)
	}
	if err := PersistEntry(ctx, r.store, sessionID, TranscriptEntry{
		Timestamp: time.Now(),
		Role:      "user",
		Content:   userContent,
	}); err != nil {
		return fmt.Errorf("skillrunner: persist user: %w", err)
	}

	// Fire PostToolUse after the model call regardless of success/cancel/error.
	// Use a fresh background context so a cancelled ctx does not skip the hook.
	defer func() {
		postCtx := context.Background()
		if ferr := fireLifecycle(postCtx, r.hm, hooks.EventPostToolUse, sessionID, skill.Name, args); ferr != nil && retErr == nil {
			retErr = ferr
		}
	}()

	// Fire PreToolUse just before the model call.
	if ferr := fireLifecycle(ctx, r.hm, hooks.EventPreToolUse, sessionID, skill.Name, args); ferr != nil {
		return ferr
	}

	req := model.ChatRequest{
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: systemPrompt},
			{Role: model.RoleUser, Content: userContent},
		},
		Stream: true,
	}

	chunks, err := r.modelClient.ChatStream(ctx, req)
	if err != nil {
		return fmt.Errorf("skillrunner: chat stream: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-chunks:
			if !ok {
				return nil
			}
			if chunk.Delta != "" {
				if _, werr := io.WriteString(r.out, chunk.Delta); werr != nil {
					return fmt.Errorf("skillrunner: write output: %w", werr)
				}
			}
			entry := TranscriptEntry{
				Timestamp: time.Now(),
				Role:      "assistant",
				Delta:     chunk.Delta,
				Done:      chunk.Done,
				Usage:     chunk.Usage,
			}
			if perr := PersistEntry(ctx, r.store, sessionID, entry); perr != nil {
				return fmt.Errorf("skillrunner: persist chunk: %w", perr)
			}
			if chunk.Done {
				return nil
			}
		}
	}
}

// readSkillDepthEnv returns the parsed CLUE_CODE_SKILL_DEPTH (0 if unset or
// unparseable). Mirrors engine.go's depth-tracking convention.
func readSkillDepthEnv() int {
	val := os.Getenv(EnvSkillDepth)
	if val == "" {
		return 0
	}
	d, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return d
}
