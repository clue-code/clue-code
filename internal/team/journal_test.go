package team

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func makeEnv(seq uint64) Envelope {
	return Envelope{
		V:    EnvelopeVersion,
		Seq:  seq,
		From: "sender",
		To:   "recv",
		Kind: "msg",
		Ts:   time.Now().UTC(),
	}
}

func TestJournal_AppendAndRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j, err := OpenJournal("team1", dir)
	if err != nil {
		t.Fatalf("OpenJournal: %v", err)
	}
	defer j.Close()

	for i := uint64(1); i <= 5; i++ {
		if err := j.Append(makeEnv(i)); err != nil {
			t.Fatalf("Append seq=%d: %v", i, err)
		}
	}

	envs, err := j.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(envs) != 5 {
		t.Fatalf("got %d envelopes, want 5", len(envs))
	}
	for i, e := range envs {
		if e.Seq != uint64(i+1) {
			t.Errorf("envs[%d].Seq = %d, want %d", i, e.Seq, i+1)
		}
	}
}

func TestJournal_TornTailRecovery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write 5 envelopes via a journal, then close it.
	j, err := OpenJournal("team-torn", dir)
	if err != nil {
		t.Fatalf("OpenJournal first: %v", err)
	}
	for i := uint64(1); i <= 5; i++ {
		if err := j.Append(makeEnv(i)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := j.Close(); err != nil {
		t.Fatalf("Close first: %v", err)
	}

	// Corrupt the file: truncate it mid-way through the last line.
	jPath := journalPath(dir, "team-torn")
	fi, err := os.Stat(jPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Remove the last 10 bytes to simulate a torn write.
	newSize := fi.Size() - 10
	if newSize < 0 {
		newSize = 0
	}
	if err := os.Truncate(jPath, newSize); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	// Re-open — RecoverTornTail should drop the partial last record.
	j2, err := OpenJournal("team-torn", dir)
	if err != nil {
		t.Fatalf("OpenJournal second: %v", err)
	}
	defer j2.Close()

	envs, err := j2.Read()
	if err != nil {
		t.Fatalf("Read after recovery: %v", err)
	}
	if len(envs) != 4 {
		t.Fatalf("got %d envelopes after torn-tail recovery, want 4", len(envs))
	}
}

func TestUnsupportedEnvelopeVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	teamDir := filepath.Join(dir, ".clue-code", "teams", "team-v99")
	if err := os.MkdirAll(teamDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	jPath := filepath.Join(teamDir, "journal.ndjson")

	// Write a valid v=1 envelope followed by a v=99 envelope.
	type rawEnv struct {
		V    uint8  `json:"v"`
		Seq  uint64 `json:"seq"`
		From string `json:"from"`
		To   string `json:"to"`
		Kind string `json:"kind"`
		Ts   string `json:"ts"`
	}
	good, _ := json.Marshal(rawEnv{V: 1, Seq: 1, From: "a", To: "b", Kind: "ok", Ts: "2026-05-06T00:00:00Z"})
	bad, _ := json.Marshal(rawEnv{V: 99, Seq: 2, From: "a", To: "b", Kind: "bad", Ts: "2026-05-06T00:00:00Z"})
	content := append(good, '\n')
	content = append(content, bad...)
	content = append(content, '\n')
	if err := os.WriteFile(jPath, content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	j, err := OpenJournal("team-v99", dir)
	if err != nil {
		t.Fatalf("OpenJournal: %v", err)
	}
	defer j.Close()

	_, err = j.Read()
	if err == nil {
		t.Fatal("expected ErrUnsupportedEnvelopeVersion, got nil")
	}
	if !isUnsupportedVersionErr(err) {
		t.Errorf("expected ErrUnsupportedEnvelopeVersion in error chain, got: %v", err)
	}
}

func TestJournal_ConcurrentAppend(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j, err := OpenJournal("team-conc", dir)
	if err != nil {
		t.Fatalf("OpenJournal: %v", err)
	}
	defer j.Close()

	const goroutines = 10
	const perGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				seq := uint64(g*perGoroutine + i)
				env := Envelope{
					V:    EnvelopeVersion,
					Seq:  seq,
					From: fmt.Sprintf("worker-%d", g),
					To:   "coordinator",
					Kind: "work",
					Ts:   time.Now().UTC(),
				}
				if err := j.Append(env); err != nil {
					t.Errorf("Append goroutine %d seq %d: %v", g, i, err)
				}
			}
		}()
	}
	wg.Wait()

	envs, err := j.Read()
	if err != nil {
		t.Fatalf("Read after concurrent appends: %v", err)
	}
	if len(envs) != goroutines*perGoroutine {
		t.Errorf("got %d envelopes, want %d", len(envs), goroutines*perGoroutine)
	}
}
