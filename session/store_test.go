package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/momo-mnsjtxy/agentengine/agent"
	"github.com/momo-mnsjtxy/agentengine/model"
)

func TestSaveAndLoadLatestWorkspaceSession(t *testing.T) {
	store, err := NewAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return current }

	first, err := store.Start("/workspace/a", "model-a", "responses", "high")
	if err != nil {
		t.Fatal(err)
	}
	first.Agent = agent.Snapshot{
		Messages: []model.Message{{Role: "user", Content: "first"}},
		Turns:    1,
		Task: agent.TaskState{
			Goal:     "first",
			NextStep: "inspect",
			Plan:     []agent.PlanItem{{Step: "inspect", Status: "in_progress"}},
		},
	}
	first, err = store.Save(first)
	if err != nil {
		t.Fatal(err)
	}

	current = current.Add(time.Minute)
	second, err := store.Start("/workspace/a", "model-a", "responses", "high")
	if err != nil {
		t.Fatal(err)
	}
	second.Agent = agent.Snapshot{Messages: []model.Message{{Role: "user", Content: "second"}}, Turns: 2}
	second.Plan = []agent.PlanItem{{Step: "Resume", Status: "in_progress"}}
	second.Timeline = []TimelineEntry{{Kind: "user", Body: "full visible history"}}
	second.InputHistory = []string{"second"}
	second, err = store.Save(second)
	if err != nil {
		t.Fatal(err)
	}

	latest, err := store.Latest("/workspace/a", "")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != second.ID || latest.Agent.Turns != 2 || len(latest.Plan) != 1 || len(latest.Timeline) != 1 {
		t.Fatalf("unexpected latest session: %#v", latest)
	}

	previous, err := store.Latest("/workspace/a", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previous.ID != first.ID {
		t.Fatalf("expected first session, got %s", previous.ID)
	}
	if previous.Agent.Task.Goal != "first" || previous.Agent.Task.NextStep != "inspect" || len(previous.Agent.Task.Plan) != 1 {
		t.Fatalf("task ledger was not persisted: %#v", previous.Agent.Task)
	}
	if _, err := store.Latest("/workspace/b", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListAndLoadReturnWorkspaceSessionsNewestFirst(t *testing.T) {
	store, err := NewAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return current }

	first, err := store.Start("/workspace/a", "model-a", "responses", "high")
	if err != nil {
		t.Fatal(err)
	}
	first.Agent = agent.Snapshot{Messages: []model.Message{{Role: "user", Content: "first"}}, Turns: 1}
	first, err = store.Save(first)
	if err != nil {
		t.Fatal(err)
	}

	current = current.Add(time.Minute)
	second, err := store.Start("/workspace/a", "model-a", "responses", "high")
	if err != nil {
		t.Fatal(err)
	}
	second.Agent = agent.Snapshot{Messages: []model.Message{{Role: "user", Content: "second"}}, Turns: 2}
	second, err = store.Save(second)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Start("/workspace/b", "model-a", "responses", "high")
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := store.List("/workspace/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].ID != second.ID || sessions[1].ID != first.ID {
		t.Fatalf("unexpected session list: %#v", sessions)
	}

	loaded, err := store.Load("/workspace/a", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Agent.Messages[0].Content = "mutated"
	if first.Agent.Messages[0].Content != "first" {
		t.Fatal("Load returned shared message storage")
	}
	if _, err := store.Load("/workspace/b", first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSaveUsesPrivateFilePermissions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Start("/workspace", "model", "responses", "medium")
	if err != nil {
		t.Fatal(err)
	}
	state, err = store.Save(state)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, state.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode is %o", info.Mode().Perm())
	}
}

func TestSaveRejectsUnsafeSessionID(t *testing.T) {
	store, err := NewAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Save(State{ID: "../outside", Workspace: "/workspace"})
	if err == nil {
		t.Fatal("unsafe session id was accepted")
	}
}
