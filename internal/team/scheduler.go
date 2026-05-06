package team

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCyclicDependency is returned when a task's dependency graph contains a cycle.
var ErrCyclicDependency = errors.New("team: cyclic dependency detected")

// Scheduler maintains the DAG of tasks and evaluates which tasks become
// runnable when dependencies are satisfied. It is safe for concurrent use.
type Scheduler struct {
	mu    sync.Mutex
	tasks map[string]*Task
	deps  map[string][]string // taskID → list of taskIDs it depends on
}

// NewScheduler creates a new, empty Scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		tasks: make(map[string]*Task),
		deps:  make(map[string][]string),
	}
}

// AddTask registers a task with the scheduler. It computes the initial status:
//   - TaskStatusPending  if the task has no dependencies
//   - TaskStatusBlocked  if one or more dependencies are not yet completed
//
// AddTask validates the full dependency graph for cycles using Kahn's algorithm
// and returns ErrCyclicDependency if a cycle would be introduced.
func (s *Scheduler) AddTask(t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store the task and its deps.
	s.tasks[t.ID] = t
	s.deps[t.ID] = append([]string(nil), t.DependsOn...)

	// Validate acyclicity over the full graph (Kahn's algorithm).
	if err := s.detectCycle(); err != nil {
		// Roll back the insertion so the scheduler stays consistent.
		delete(s.tasks, t.ID)
		delete(s.deps, t.ID)
		return err
	}

	// Compute initial status.
	if s.allDepsComplete(t.ID) {
		t.Status = TaskStatusPending
	} else {
		t.Status = TaskStatusBlocked
	}
	return nil
}

// MarkComplete marks taskID as completed and returns the IDs of tasks that
// became unblocked (all their dependencies are now complete). The returned
// tasks have their status updated to TaskStatusRunning.
func (s *Scheduler) MarkComplete(taskID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, ok := s.tasks[taskID]; ok {
		t.Status = TaskStatusCompleted
	}

	var unblocked []string
	for id, task := range s.tasks {
		if task.Status != TaskStatusBlocked {
			continue
		}
		if s.allDepsComplete(id) {
			task.Status = TaskStatusRunning
			unblocked = append(unblocked, id)
		}
	}
	return unblocked
}

// MarkFailed marks taskID as failed. Per D2, failure does NOT cascade to
// dependent tasks — only the task itself is marked failed.
func (s *Scheduler) MarkFailed(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, ok := s.tasks[taskID]; ok {
		t.Status = TaskStatusFailed
	}
}

// allDepsComplete returns true when every dependency of taskID has status
// TaskStatusCompleted. Must be called with s.mu held.
func (s *Scheduler) allDepsComplete(taskID string) bool {
	for _, depID := range s.deps[taskID] {
		dep, ok := s.tasks[depID]
		if !ok || dep.Status != TaskStatusCompleted {
			return false
		}
	}
	return true
}

// detectCycle runs Kahn's topological sort on the current s.tasks/s.deps state
// and returns ErrCyclicDependency if a cycle exists. Must be called with s.mu held.
func (s *Scheduler) detectCycle() error {
	// Build in-degree map.
	inDegree := make(map[string]int, len(s.tasks))
	for id := range s.tasks {
		inDegree[id] = 0
	}
	for id := range s.tasks {
		for _, dep := range s.deps[id] {
			// dep must already be registered — if not it won't be in inDegree
			// but we still need to account for it to detect forward references.
			if _, ok := inDegree[dep]; !ok {
				inDegree[dep] = 0
			}
			inDegree[id]++ // id depends on dep → id has one more in-edge
		}
	}

	// Collect all zero-in-degree nodes.
	queue := make([]string, 0, len(inDegree))
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		visited++

		// For each task that depends on cur, reduce its in-degree.
		for id := range s.tasks {
			for _, dep := range s.deps[id] {
				if dep == cur {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	if visited < len(inDegree) {
		return fmt.Errorf("%w", ErrCyclicDependency)
	}
	return nil
}

// taskStatusFromString converts a string to TaskStatus (used during journal replay).
func taskStatusFromString(s string) TaskStatus {
	switch s {
	case string(TaskStatusPending):
		return TaskStatusPending
	case string(TaskStatusBlocked):
		return TaskStatusBlocked
	case string(TaskStatusRunning):
		return TaskStatusRunning
	case string(TaskStatusCompleted):
		return TaskStatusCompleted
	case string(TaskStatusFailed):
		return TaskStatusFailed
	default:
		return TaskStatusPending
	}
}

// nowFunc is replaceable in tests.
var nowFunc = func() time.Time { return time.Now().UTC() }
