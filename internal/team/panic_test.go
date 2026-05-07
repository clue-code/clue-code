package team

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// TestPanicRecovery (D4): worker A panics on task1, worker B completes task2
// normally. task1 must be TaskFailed, journal must have agent-down, task2 must
// complete successfully.
func TestPanicRecovery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tm, err := TeamCreate(Spec{Workers: 2, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	defer tm.Close()

	task1, err := tm.TaskCreate(TaskSpec{Owner: "worker-a", ID: "task1"})
	if err != nil {
		t.Fatalf("TaskCreate task1: %v", err)
	}
	task2, err := tm.TaskCreate(TaskSpec{Owner: "worker-b", ID: "task2"})
	if err != nil {
		t.Fatalf("TaskCreate task2: %v", err)
	}
	_ = task2

	var wg sync.WaitGroup

	// Worker A: panics.
	wg.Add(1)
	var workerAErr error
	go func() {
		defer wg.Done()
		workerAErr = RunWorker(tm, "worker-a", task1.ID, func() error {
			panic("boom")
		})
	}()

	// Worker B: sends a message and marks task2 done normally.
	wg.Add(1)
	var workerBErr error
	go func() {
		defer wg.Done()
		workerBErr = RunWorker(tm, "worker-b", task2.ID, func() error {
			if err := tm.SendMessage("worker-b", "progress", json.RawMessage(`{"step":1}`)); err != nil {
				return err
			}
			return tm.TaskUpdate(task2.ID, TaskStatusCompleted)
		})
	}()

	wg.Wait()

	// Worker A must return ErrWorkerPanic.
	if !errors.Is(workerAErr, ErrWorkerPanic) {
		t.Errorf("worker A error: want ErrWorkerPanic, got %v", workerAErr)
	}

	// Worker B must complete without error.
	if workerBErr != nil {
		t.Errorf("worker B error: want nil, got %v", workerBErr)
	}

	// task1 must be TaskFailed.
	tasks := tm.TaskList()
	taskMap := map[string]TaskStatus{}
	for _, task := range tasks {
		taskMap[task.ID] = task.Status
	}
	if taskMap["task1"] != TaskStatusFailed {
		t.Errorf("task1 status: want failed, got %v", taskMap["task1"])
	}
	if taskMap["task2"] != TaskStatusCompleted {
		t.Errorf("task2 status: want completed, got %v", taskMap["task2"])
	}

	// Journal must contain agent-down envelope with "boom".
	envs, err := tm.journal.Read()
	if err != nil {
		t.Fatalf("journal.Read: %v", err)
	}
	var agentDownEnv *Envelope
	for i := range envs {
		if envs[i].Kind == "agent-down" {
			agentDownEnv = &envs[i]
			break
		}
	}
	if agentDownEnv == nil {
		t.Fatal("no agent-down envelope found in journal")
	}

	var p struct {
		WorkerID   string `json:"worker_id"`
		TaskID     string `json:"task_id"`
		PanicValue string `json:"panic_value"`
	}
	if err := json.Unmarshal(agentDownEnv.Payload, &p); err != nil {
		t.Fatalf("unmarshal agent-down payload: %v", err)
	}
	if p.WorkerID != "worker-a" {
		t.Errorf("agent-down worker_id: want worker-a, got %q", p.WorkerID)
	}
	if p.TaskID != "task1" {
		t.Errorf("agent-down task_id: want task1, got %q", p.TaskID)
	}
	if p.PanicValue != "boom" {
		t.Errorf("agent-down panic_value: want boom, got %q", p.PanicValue)
	}
}

// TestPanicRecovery_NoCascade: panic in worker A must not affect worker B's
// task status or ability to continue operating.
func TestPanicRecovery_NoCascade(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tm, err := TeamCreate(Spec{Workers: 2, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	defer tm.Close()

	taskA, err := tm.TaskCreate(TaskSpec{Owner: "worker-a", ID: "taskA"})
	if err != nil {
		t.Fatalf("TaskCreate taskA: %v", err)
	}
	taskB, err := tm.TaskCreate(TaskSpec{Owner: "worker-b", ID: "taskB"})
	if err != nil {
		t.Fatalf("TaskCreate taskB: %v", err)
	}

	var wg sync.WaitGroup

	// Worker A panics immediately.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = RunWorker(tm, "worker-a", taskA.ID, func() error {
			panic("worker-a-panic")
		})
	}()

	// Worker B continues independently.
	wg.Add(1)
	var bErr error
	go func() {
		defer wg.Done()
		bErr = RunWorker(tm, "worker-b", taskB.ID, func() error {
			// Do some work unrelated to A.
			return tm.TaskUpdate(taskB.ID, TaskStatusCompleted)
		})
	}()

	wg.Wait()

	if bErr != nil {
		t.Errorf("worker B should not be affected by A's panic: got %v", bErr)
	}

	tasks := tm.TaskList()
	taskMap := map[string]TaskStatus{}
	for _, task := range tasks {
		taskMap[task.ID] = task.Status
	}

	if taskMap["taskA"] != TaskStatusFailed {
		t.Errorf("taskA: want failed, got %v", taskMap["taskA"])
	}
	if taskMap["taskB"] != TaskStatusCompleted {
		t.Errorf("taskB: want completed, got %v (panic must not cascade)", taskMap["taskB"])
	}
}

// TestRunWorker_NormalReturn: RunWorker returns the fn error without wrapping
// when no panic occurs.
func TestRunWorker_NormalReturn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tm, err := TeamCreate(Spec{Workers: 1, ProjectRoot: dir})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	defer tm.Close()

	task, err := tm.TaskCreate(TaskSpec{Owner: "w", ID: "t1"})
	if err != nil {
		t.Fatalf("TaskCreate: %v", err)
	}

	// fn returns nil — RunWorker must return nil.
	got := RunWorker(tm, "w", task.ID, func() error { return nil })
	if got != nil {
		t.Errorf("RunWorker nil fn: want nil, got %v", got)
	}

	// fn returns a sentinel error — RunWorker must return it unwrapped.
	sentinel := errors.New("my-error")
	got = RunWorker(tm, "w", task.ID, func() error { return sentinel })
	if !errors.Is(got, sentinel) {
		t.Errorf("RunWorker error fn: want sentinel, got %v", got)
	}
}
