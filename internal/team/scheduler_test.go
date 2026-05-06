package team

import (
	"testing"
)

func TestScheduler_Topological(t *testing.T) {
	t.Parallel()
	s := NewScheduler()

	taskA := &Task{ID: "A", Status: TaskStatusPending}
	taskB := &Task{ID: "B", DependsOn: []string{"A"}}
	taskC := &Task{ID: "C", DependsOn: []string{"B"}}

	if err := s.AddTask(taskA); err != nil {
		t.Fatalf("AddTask A: %v", err)
	}
	if err := s.AddTask(taskB); err != nil {
		t.Fatalf("AddTask B: %v", err)
	}
	if err := s.AddTask(taskC); err != nil {
		t.Fatalf("AddTask C: %v", err)
	}

	// Initial statuses.
	if taskA.Status != TaskStatusPending {
		t.Errorf("A: want pending, got %s", taskA.Status)
	}
	if taskB.Status != TaskStatusBlocked {
		t.Errorf("B: want blocked, got %s", taskB.Status)
	}
	if taskC.Status != TaskStatusBlocked {
		t.Errorf("C: want blocked, got %s", taskC.Status)
	}

	// Complete A → B becomes running, C stays blocked.
	unblocked := s.MarkComplete("A")
	if len(unblocked) != 1 || unblocked[0] != "B" {
		t.Errorf("MarkComplete(A): want [B], got %v", unblocked)
	}
	if taskB.Status != TaskStatusRunning {
		t.Errorf("B after A complete: want running, got %s", taskB.Status)
	}
	if taskC.Status != TaskStatusBlocked {
		t.Errorf("C after A complete: want blocked, got %s", taskC.Status)
	}

	// Complete B → C becomes running.
	unblocked = s.MarkComplete("B")
	if len(unblocked) != 1 || unblocked[0] != "C" {
		t.Errorf("MarkComplete(B): want [C], got %v", unblocked)
	}
	if taskC.Status != TaskStatusRunning {
		t.Errorf("C after B complete: want running, got %s", taskC.Status)
	}
}

func TestScheduler_Cycle(t *testing.T) {
	t.Parallel()
	s := NewScheduler()

	taskA := &Task{ID: "A", DependsOn: []string{"B"}}
	taskB := &Task{ID: "B", DependsOn: []string{"A"}}

	// Adding A first (B not yet registered — should succeed as forward ref isn't
	// a cycle by itself with one node).
	if err := s.AddTask(taskA); err != nil {
		t.Fatalf("AddTask A (first): %v", err)
	}

	// Adding B creates a cycle A→B→A.
	err := s.AddTask(taskB)
	if err == nil {
		t.Fatal("AddTask B: expected cycle error, got nil")
	}
}

func TestScheduler_MultipleDeps(t *testing.T) {
	t.Parallel()
	s := NewScheduler()

	taskA := &Task{ID: "A"}
	taskB := &Task{ID: "B"}
	taskD := &Task{ID: "D", DependsOn: []string{"A", "B"}}

	for _, task := range []*Task{taskA, taskB, taskD} {
		if err := s.AddTask(task); err != nil {
			t.Fatalf("AddTask %s: %v", task.ID, err)
		}
	}

	// D depends on both A and B — must stay blocked until both complete.
	if taskD.Status != TaskStatusBlocked {
		t.Errorf("D initial: want blocked, got %s", taskD.Status)
	}

	// Complete A — D still blocked (B not done).
	unblocked := s.MarkComplete("A")
	for _, id := range unblocked {
		if id == "D" {
			t.Error("D should not unblock when only A is complete")
		}
	}
	if taskD.Status != TaskStatusBlocked {
		t.Errorf("D after A complete: want blocked, got %s", taskD.Status)
	}

	// Complete B — D now unblocked.
	unblocked = s.MarkComplete("B")
	found := false
	for _, id := range unblocked {
		if id == "D" {
			found = true
		}
	}
	if !found {
		t.Errorf("D should be in unblocked list after B complete, got %v", unblocked)
	}
	if taskD.Status != TaskStatusRunning {
		t.Errorf("D after B complete: want running, got %s", taskD.Status)
	}
}
