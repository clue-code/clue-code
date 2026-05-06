package team

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the lifecycle state of a Task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// mailboxCap is the bounded capacity of each worker mailbox (D5).
const mailboxCap = 256

var (
	// ErrMailboxFull is returned by SendMessage when the target mailbox is at
	// capacity and the send cannot proceed without blocking.
	ErrMailboxFull = errors.New("team: mailbox full")

	// ErrTaskNotFound is returned when a task ID is not found in the team.
	ErrTaskNotFound = errors.New("team: task not found")
)

// Task represents a unit of work assigned to a team worker.
type Task struct {
	ID        string     `json:"id"`
	Status    TaskStatus `json:"status"`
	DependsOn []string   `json:"depends_on,omitempty"`
	Owner     string     `json:"owner,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Spec is the configuration for creating a new Team.
type Spec struct {
	// ID is the team identifier. If empty, a UUID v4 is generated.
	ID string
	// Workers is the number of worker slots. Must be <= MaxTeamWorkers.
	Workers int
	// ProjectRoot is the root directory under which the team journal is stored.
	ProjectRoot string
}

// TaskSpec describes a task to be created.
type TaskSpec struct {
	ID        string   // auto-generated if empty
	DependsOn []string // IDs of tasks that must complete before this one runs
	Owner     string   // worker ID that owns this task
}

// Team is the central coordination primitive. It owns the journal, task
// registry, per-worker mailboxes, and the scheduling DAG.
type Team struct {
	ID          string
	Workers     int
	seq         atomic.Uint64
	journal     *Journal
	projectRoot string

	mu        sync.RWMutex
	tasks     map[string]*Task
	mailboxes map[string]chan Message // workerID → bounded chan

	inboxClosed atomic.Bool
	scheduler   *Scheduler
}

// teamSnapshot is the JSON structure written to team.json.
type teamSnapshot struct {
	ID          string    `json:"id"`
	Workers     int       `json:"workers"`
	ProjectRoot string    `json:"project_root"`
	CreatedAt   time.Time `json:"created_at"`
}

// tasksSnapshot is the JSON structure written to tasks.json.
type tasksSnapshot struct {
	Tasks []*Task `json:"tasks"`
}

// TeamCreate creates a new Team, validates forkbomb constraints, opens the
// journal, initialises mailboxes, and writes the initial "team-create" envelope.
func TeamCreate(spec Spec) (*Team, error) {
	if err := CheckTeamSize(spec.Workers); err != nil {
		return nil, err
	}
	if err := CheckDepth(); err != nil {
		return nil, err
	}

	id := spec.ID
	if id == "" {
		id = uuid.New().String()
	}

	journal, err := OpenJournal(id, spec.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("team: open journal: %w", err)
	}

	t := &Team{
		ID:          id,
		Workers:     spec.Workers,
		journal:     journal,
		projectRoot: spec.ProjectRoot,
		tasks:       make(map[string]*Task),
		mailboxes:   make(map[string]chan Message, spec.Workers),
		scheduler:   NewScheduler(),
	}

	// Write the team-create envelope (seq=0).
	snap := teamSnapshot{
		ID:          id,
		Workers:     spec.Workers,
		ProjectRoot: spec.ProjectRoot,
		CreatedAt:   nowFunc(),
	}
	snapBytes, err := json.Marshal(snap)
	if err != nil {
		_ = journal.Close()
		return nil, fmt.Errorf("team: marshal team snapshot: %w", err)
	}

	env := Envelope{
		V:       EnvelopeVersion,
		Seq:     t.seq.Add(1) - 1, // first seq = 0
		From:    id,
		To:      id,
		Kind:    "team-create",
		Payload: json.RawMessage(snapBytes),
		Ts:      nowFunc(),
	}
	if err := journal.Append(env); err != nil {
		_ = journal.Close()
		return nil, fmt.Errorf("team: append team-create: %w", err)
	}

	// Write initial snapshot caches.
	if err := t.writeTeamSnapshot(); err != nil {
		_ = journal.Close()
		return nil, err
	}
	if err := t.writeTasksSnapshot(); err != nil {
		_ = journal.Close()
		return nil, err
	}

	return t, nil
}

// Open re-attaches to an existing team by replaying its journal. It
// reconstructs the task registry, scheduler state, and sequence counter.
// If snapshot caches (team.json / tasks.json) are missing, they are rebuilt
// from the journal (D12).
func Open(teamID, projectRoot string) (*Team, error) {
	journal, err := OpenJournal(teamID, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("team: open journal for %q: %w", teamID, err)
	}

	envs, err := journal.Read()
	if err != nil {
		_ = journal.Close()
		return nil, fmt.Errorf("team: journal read: %w", err)
	}

	t := &Team{
		ID:          teamID,
		projectRoot: projectRoot,
		journal:     journal,
		tasks:       make(map[string]*Task),
		mailboxes:   make(map[string]chan Message),
		scheduler:   NewScheduler(),
	}

	// Replay envelopes in order to reconstruct state.
	var maxSeq uint64
	for _, env := range envs {
		if env.Seq > maxSeq {
			maxSeq = env.Seq
		}
		switch env.Kind {
		case "team-create":
			var snap teamSnapshot
			if err := json.Unmarshal(env.Payload, &snap); err == nil {
				t.Workers = snap.Workers
			}
		case "task-create":
			var payload struct {
				ID        string   `json:"id"`
				DependsOn []string `json:"depends_on,omitempty"`
				Owner     string   `json:"owner,omitempty"`
				CreatedAt string   `json:"created_at,omitempty"`
			}
			if err := json.Unmarshal(env.Payload, &payload); err == nil {
				createdAt := env.Ts
				if payload.CreatedAt != "" {
					if parsed, err := time.Parse(time.RFC3339Nano, payload.CreatedAt); err == nil {
						createdAt = parsed
					}
				}
				task := &Task{
					ID:        payload.ID,
					Status:    TaskStatusPending,
					DependsOn: payload.DependsOn,
					Owner:     payload.Owner,
					CreatedAt: createdAt,
				}
				t.tasks[task.ID] = task
				// Re-add to scheduler (ignore cycle errors on replay — data was valid when written).
				_ = t.scheduler.AddTask(task)
			}
		case "task-update":
			var payload struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(env.Payload, &payload); err == nil {
				if task, ok := t.tasks[payload.ID]; ok {
					newStatus := taskStatusFromString(payload.Status)
					task.Status = newStatus
					// Update scheduler state.
					switch newStatus {
					case TaskStatusCompleted:
						t.scheduler.MarkComplete(payload.ID)
					case TaskStatusFailed:
						t.scheduler.MarkFailed(payload.ID)
					}
				}
			}
		case "message":
			// Messages are not replayed into mailboxes on open — mailboxes start empty.
		}
	}

	// seq starts at max(seq in journal)+1 to maintain monotonicity.
	t.seq.Store(maxSeq + 1)

	// Ensure mailboxes exist for all workers referenced in tasks.
	t.mu.Lock()
	for _, task := range t.tasks {
		if task.Owner != "" {
			if _, ok := t.mailboxes[task.Owner]; !ok {
				t.mailboxes[task.Owner] = make(chan Message, mailboxCap)
			}
		}
	}
	t.mu.Unlock()

	// Rebuild snapshot caches if missing (D12).
	dir := journalDir(projectRoot, teamID)
	teamJSONPath := filepath.Join(dir, "team.json")
	tasksJSONPath := filepath.Join(dir, "tasks.json")

	rebuildNeeded := false
	if _, err := os.Stat(teamJSONPath); os.IsNotExist(err) {
		rebuildNeeded = true
	}
	if _, err := os.Stat(tasksJSONPath); os.IsNotExist(err) {
		rebuildNeeded = true
	}
	if rebuildNeeded {
		if err := t.writeTeamSnapshot(); err != nil {
			_ = journal.Close()
			return nil, fmt.Errorf("team: rebuild team.json: %w", err)
		}
		if err := t.writeTasksSnapshot(); err != nil {
			_ = journal.Close()
			return nil, fmt.Errorf("team: rebuild tasks.json: %w", err)
		}
	}

	return t, nil
}

// TaskCreate creates a new task, appends its creation envelope to the journal,
// registers it with the scheduler, and updates snapshot caches.
func (t *Team) TaskCreate(spec TaskSpec) (*Task, error) {
	id := spec.ID
	if id == "" {
		id = uuid.New().String()
	}

	task := &Task{
		ID:        id,
		DependsOn: spec.DependsOn,
		Owner:     spec.Owner,
		CreatedAt: nowFunc(),
	}

	// Register with scheduler to compute initial status.
	if err := t.scheduler.AddTask(task); err != nil {
		return nil, fmt.Errorf("team: scheduler add task: %w", err)
	}

	// Ensure mailbox exists for owner.
	if spec.Owner != "" {
		t.mu.Lock()
		if _, ok := t.mailboxes[spec.Owner]; !ok {
			t.mailboxes[spec.Owner] = make(chan Message, mailboxCap)
		}
		t.mu.Unlock()
	}

	// Build payload.
	payload := struct {
		ID        string   `json:"id"`
		DependsOn []string `json:"depends_on,omitempty"`
		Owner     string   `json:"owner,omitempty"`
		CreatedAt string   `json:"created_at"`
	}{
		ID:        id,
		DependsOn: spec.DependsOn,
		Owner:     spec.Owner,
		CreatedAt: task.CreatedAt.Format(time.RFC3339Nano),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("team: marshal task-create payload: %w", err)
	}

	env := Envelope{
		V:       EnvelopeVersion,
		Seq:     t.seq.Add(1) - 1,
		From:    t.ID,
		To:      t.ID,
		Kind:    "task-create",
		Payload: json.RawMessage(payloadBytes),
		Ts:      nowFunc(),
	}
	if err := t.journal.Append(env); err != nil {
		return nil, fmt.Errorf("team: append task-create: %w", err)
	}

	t.mu.Lock()
	t.tasks[id] = task
	t.mu.Unlock()

	// Update snapshot caches.
	if err := t.writeTasksSnapshot(); err != nil {
		return nil, err
	}
	if err := t.writeTeamSnapshot(); err != nil {
		return nil, err
	}

	return task, nil
}

// SendMessage appends a message envelope to the journal (durability-first),
// then attempts a non-blocking push into the target worker's mailbox.
// Returns ErrMailboxFull immediately if the mailbox is at capacity (D5).
func (t *Team) SendMessage(to, kind string, payload json.RawMessage) error {
	// Ensure mailbox exists and get a reference to it.
	t.mu.RLock()
	ch, ok := t.mailboxes[to]
	t.mu.RUnlock()

	if !ok {
		// Create mailbox on first send to this worker.
		t.mu.Lock()
		ch, ok = t.mailboxes[to]
		if !ok {
			ch = make(chan Message, mailboxCap)
			t.mailboxes[to] = ch
		}
		t.mu.Unlock()
	}

	// Check capacity BEFORE journal append so the overflow path is truly
	// non-blocking (D5: 257th send must return ErrMailboxFull in < 1ms).
	if len(ch) >= mailboxCap {
		return ErrMailboxFull
	}

	seq := t.seq.Add(1) - 1

	env := Envelope{
		V:       EnvelopeVersion,
		Seq:     seq,
		From:    t.ID,
		To:      to,
		Kind:    "message",
		Payload: payload,
		Ts:      nowFunc(),
	}

	// Append to journal BEFORE mailbox push (durability guarantee).
	if err := t.journal.Append(env); err != nil {
		return fmt.Errorf("team: append message: %w", err)
	}

	msg := MessageFromEnvelope(env)
	msg.Kind = kind // preserve application-level kind

	// Non-blocking push (D5 explicit).
	select {
	case ch <- msg:
	default:
		return ErrMailboxFull
	}
	return nil
}

// Inbox returns the receive-only channel for workerID's mailbox. The channel
// is created with mailboxCap capacity if it does not yet exist.
func (t *Team) Inbox(workerID string) <-chan Message {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch, ok := t.mailboxes[workerID]
	if !ok {
		ch = make(chan Message, mailboxCap)
		t.mailboxes[workerID] = ch
	}
	return ch
}

// TaskList returns a snapshot copy of all tasks registered with the team.
func (t *Team) TaskList() []*Task {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]*Task, 0, len(t.tasks))
	for _, task := range t.tasks {
		// Return a copy to avoid external mutation.
		cp := *task
		out = append(out, &cp)
	}
	return out
}

// TaskUpdate updates the status of a task, appends the change to the journal,
// and asks the scheduler to evaluate newly-runnable tasks.
func (t *Team) TaskUpdate(taskID string, status TaskStatus) error {
	t.mu.RLock()
	_, ok := t.tasks[taskID]
	t.mu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	payload := struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}{ID: taskID, Status: string(status)}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("team: marshal task-update payload: %w", err)
	}

	env := Envelope{
		V:       EnvelopeVersion,
		Seq:     t.seq.Add(1) - 1,
		From:    t.ID,
		To:      t.ID,
		Kind:    "task-update",
		Payload: json.RawMessage(payloadBytes),
		Ts:      nowFunc(),
	}
	if err := t.journal.Append(env); err != nil {
		return fmt.Errorf("team: append task-update: %w", err)
	}

	t.mu.Lock()
	if task, exists := t.tasks[taskID]; exists {
		task.Status = status
	}
	t.mu.Unlock()

	// Let the scheduler evaluate downstream tasks.
	switch status {
	case TaskStatusCompleted:
		t.scheduler.MarkComplete(taskID)
	case TaskStatusFailed:
		t.scheduler.MarkFailed(taskID)
	}

	// Update snapshot caches.
	return t.writeTasksSnapshot()
}

// Close flushes the journal and closes all mailboxes.
func (t *Team) Close() error {
	t.inboxClosed.Store(true)

	t.mu.Lock()
	for _, ch := range t.mailboxes {
		close(ch)
	}
	t.mailboxes = make(map[string]chan Message)
	t.mu.Unlock()

	return t.journal.Close()
}

// writeTeamSnapshot writes team.json to the journal directory.
// Must NOT be called with t.mu held.
func (t *Team) writeTeamSnapshot() error {
	snap := teamSnapshot{
		ID:          t.ID,
		Workers:     t.Workers,
		ProjectRoot: t.projectRoot,
		CreatedAt:   nowFunc(),
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("team: marshal team.json: %w", err)
	}
	path := filepath.Join(journalDir(t.projectRoot, t.ID), "team.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("team: write team.json: %w", err)
	}
	return nil
}

// writeTasksSnapshot writes tasks.json to the journal directory.
// Must NOT be called with t.mu held.
func (t *Team) writeTasksSnapshot() error {
	t.mu.RLock()
	tasks := make([]*Task, 0, len(t.tasks))
	for _, task := range t.tasks {
		cp := *task
		tasks = append(tasks, &cp)
	}
	t.mu.RUnlock()

	snap := tasksSnapshot{Tasks: tasks}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("team: marshal tasks.json: %w", err)
	}
	path := filepath.Join(journalDir(t.projectRoot, t.ID), "tasks.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("team: write tasks.json: %w", err)
	}
	return nil
}
