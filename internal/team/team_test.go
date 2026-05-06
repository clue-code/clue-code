package team

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestFanOut (D1): 4 workers, each creates 2 tasks + sends 3 messages.
// Journal must contain exactly 21 lines:
//   1  team-create
//   4 workers × (2 task-create + 3 message) = 4 × 5 = 20
//   Total = 21
func TestFanOut(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tm, err := TeamCreate(Spec{Workers: 4, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	defer tm.Close()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		workerID := workerLabel(i)
		wg.Add(1)
		go func(wid string) {
			defer wg.Done()
			// Each worker creates 2 tasks.
			for j := 0; j < 2; j++ {
				if _, err := tm.TaskCreate(TaskSpec{Owner: wid}); err != nil {
					t.Errorf("worker %s TaskCreate: %v", wid, err)
				}
			}
			// Each worker sends 3 messages.
			for j := 0; j < 3; j++ {
				if err := tm.SendMessage(wid, "ping", json.RawMessage(`{"n":1}`)); err != nil {
					t.Errorf("worker %s SendMessage: %v", wid, err)
				}
			}
		}(workerID)
	}
	wg.Wait()

	envs, err := tm.journal.Read()
	if err != nil {
		t.Fatalf("journal.Read: %v", err)
	}

	// Count by kind.
	counts := map[string]int{}
	for _, env := range envs {
		counts[env.Kind]++
	}

	total := len(envs)
	if total != 21 {
		t.Errorf("journal line count: want 21, got %d (breakdown: %v)", total, counts)
	}
	if counts["team-create"] != 1 {
		t.Errorf("team-create count: want 1, got %d", counts["team-create"])
	}
	if counts["task-create"] != 8 {
		t.Errorf("task-create count: want 8, got %d", counts["task-create"])
	}
	if counts["message"] != 12 {
		t.Errorf("message count: want 12, got %d", counts["message"])
	}
}

// TestCrashResume (D3): create team, add tasks + messages, close, re-open.
// Verify task state is identical and seq is monotonically higher.
func TestCrashResume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Phase 1: create and populate.
	tm1, err := TeamCreate(Spec{Workers: 2, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	teamID := tm1.ID

	for i := 0; i < 5; i++ {
		if _, err := tm1.TaskCreate(TaskSpec{Owner: "worker-0"}); err != nil {
			t.Fatalf("TaskCreate %d: %v", i, err)
		}
	}
	for i := 0; i < 10; i++ {
		if err := tm1.SendMessage("worker-0", "work", json.RawMessage(`{}`)); err != nil {
			t.Fatalf("SendMessage %d: %v", i, err)
		}
	}

	seqBeforeClose := tm1.seq.Load()
	originalTasks := tm1.TaskList()

	if err := tm1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Phase 2: re-open and verify.
	tm2, err := Open(teamID, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tm2.Close()

	// Seq must be >= previous max to maintain monotonicity.
	if tm2.seq.Load() < seqBeforeClose {
		t.Errorf("seq after Open: %d < pre-close %d", tm2.seq.Load(), seqBeforeClose)
	}

	// Task count must match.
	recoveredTasks := tm2.TaskList()
	if len(recoveredTasks) != len(originalTasks) {
		t.Errorf("task count: want %d, got %d", len(originalTasks), len(recoveredTasks))
	}

	// Build lookup for comparison.
	origMap := map[string]TaskStatus{}
	for _, task := range originalTasks {
		origMap[task.ID] = task.Status
	}
	for _, task := range recoveredTasks {
		origStatus, ok := origMap[task.ID]
		if !ok {
			t.Errorf("unexpected task %s in recovered state", task.ID)
			continue
		}
		if task.Status != origStatus {
			t.Errorf("task %s status: want %s, got %s", task.ID, origStatus, task.Status)
		}
	}

	// Verify new operations produce higher seqs.
	if _, err := tm2.TaskCreate(TaskSpec{Owner: "worker-0"}); err != nil {
		t.Fatalf("post-Open TaskCreate: %v", err)
	}
	newSeq := tm2.seq.Load()
	if newSeq <= seqBeforeClose {
		t.Errorf("new seq %d should be > pre-close seq %d", newSeq, seqBeforeClose)
	}
}

// TestBackpressure (D5): fill mailbox to cap (256), 257th send must return
// ErrMailboxFull immediately (< 1ms, non-blocking).
func TestBackpressure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tm, err := TeamCreate(Spec{Workers: 1, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	defer tm.Close()

	const workerID = "worker-0"
	// Ensure mailbox exists.
	_ = tm.Inbox(workerID)

	// Fill to capacity (256).
	for i := 0; i < mailboxCap; i++ {
		if err := tm.SendMessage(workerID, "fill", json.RawMessage(`{}`)); err != nil {
			t.Fatalf("fill send %d: %v", i, err)
		}
	}

	// 257th must be ErrMailboxFull and must return in < 1ms.
	start := time.Now()
	err = tm.SendMessage(workerID, "overflow", json.RawMessage(`{}`))
	elapsed := time.Since(start)

	if err != ErrMailboxFull {
		t.Errorf("257th send: want ErrMailboxFull, got %v", err)
	}
	if elapsed >= time.Millisecond {
		t.Errorf("257th send took %v, want < 1ms (non-blocking)", elapsed)
	}
}

// TestRebuildFromJournalAlone (D12): delete team.json + tasks.json after
// creation, re-open — caches must be rebuilt and TaskList must match.
func TestRebuildFromJournalAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tm1, err := TeamCreate(Spec{Workers: 2, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	teamID := tm1.ID

	// Add 10 tasks and complete them.
	for i := 0; i < 10; i++ {
		task, err := tm1.TaskCreate(TaskSpec{Owner: "worker-0"})
		if err != nil {
			t.Fatalf("TaskCreate %d: %v", i, err)
		}
		if err := tm1.TaskUpdate(task.ID, TaskStatusCompleted); err != nil {
			t.Fatalf("TaskUpdate %d: %v", i, err)
		}
	}

	// Add enough messages to push past 200 journal entries.
	for i := 0; i < 200; i++ {
		_ = tm1.SendMessage("worker-0", "bulk", json.RawMessage(`{}`))
	}

	preTasks := tm1.TaskList()
	if err := tm1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Delete snapshot caches.
	jDir := journalDir(dir, teamID)
	for _, fname := range []string{"team.json", "tasks.json"} {
		path := filepath.Join(jDir, fname)
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", fname, err)
		}
	}

	// Re-open — must rebuild from journal alone.
	tm2, err := Open(teamID, dir)
	if err != nil {
		t.Fatalf("Open after cache deletion: %v", err)
	}
	defer tm2.Close()

	postTasks := tm2.TaskList()

	if len(postTasks) != len(preTasks) {
		t.Errorf("task count after rebuild: want %d, got %d", len(preTasks), len(postTasks))
	}

	preMap := map[string]TaskStatus{}
	for _, task := range preTasks {
		preMap[task.ID] = task.Status
	}
	for _, task := range postTasks {
		wantStatus, ok := preMap[task.ID]
		if !ok {
			t.Errorf("unexpected task %s after rebuild", task.ID)
			continue
		}
		if task.Status != wantStatus {
			t.Errorf("task %s status after rebuild: want %s, got %s", task.ID, wantStatus, task.Status)
		}
	}

	// team.json and tasks.json must have been recreated.
	for _, fname := range []string{"team.json", "tasks.json"} {
		path := filepath.Join(jDir, fname)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("%s not recreated after rebuild", fname)
		}
	}
}

// workerLabel returns a deterministic worker ID for test index i.
func workerLabel(i int) string {
	return "worker-" + string(rune('0'+i))
}
