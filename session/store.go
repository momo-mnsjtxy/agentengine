/*
会话存储：把已提交的 Agent 状态原子保存到用户私有目录。
存储不包含 API key、环境变量、审批许可或运行中的进程状态。
*/
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/momo-mnsjtxy/agentengine/agent"
	"github.com/momo-mnsjtxy/agentengine/model"
)

const version = 1

var ErrNotFound = errors.New("no saved session")

// State 是恢复 Agent 会话所需的最小持久化状态。
type State struct {
	Version      int              `json:"version"`
	ID           string           `json:"id"`
	Workspace    string           `json:"workspace"`
	Model        string           `json:"model"`
	Protocol     string           `json:"protocol"`
	Reasoning    string           `json:"reasoning"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	Agent        agent.Snapshot   `json:"agent"`
	Plan         []agent.PlanItem `json:"plan,omitempty"`
	Usage        model.Usage      `json:"usage,omitempty"`
	Timeline     []TimelineEntry  `json:"timeline,omitempty"`
	InputHistory []string         `json:"input_history,omitempty"`
}

// TimelineEntry 保存用户恢复后仍需要看到的已提交界面记录。
type TimelineEntry struct {
	Kind   string `json:"kind"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	Failed bool   `json:"failed,omitempty"`
}

// Store owns session files for the current user.
type Store struct {
	dir string
	now func() time.Time
}

// New creates the default XDG-compatible session store.
func New() (*Store, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	return NewAt(filepath.Join(dir, "agentengine", "sessions"))
}

// NewAt creates a store at an explicit directory, primarily for tests.
func NewAt(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, now: time.Now}, nil
}

// Start creates unsaved metadata for a new conversation.
func (store *Store) Start(workspace, modelName, protocol, reasoning string) (State, error) {
	id, err := newID(store.now())
	if err != nil {
		return State{}, err
	}
	now := store.now().UTC()
	return State{
		Version:   version,
		ID:        id,
		Workspace: workspace,
		Model:     modelName,
		Protocol:  protocol,
		Reasoning: reasoning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Save atomically replaces one session checkpoint.
func (store *Store) Save(state State) (State, error) {
	if !validID(state.ID) {
		return State{}, fmt.Errorf("invalid session id %q", state.ID)
	}
	if state.Workspace == "" {
		return State{}, errors.New("session workspace is empty")
	}
	state.Version = version
	state.UpdatedAt = store.now().UTC()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	state.Plan = append([]agent.PlanItem(nil), state.Plan...)
	state.Timeline = append([]TimelineEntry(nil), state.Timeline...)
	state.InputHistory = append([]string(nil), state.InputHistory...)

	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return State{}, err
	}
	temporary, err := os.CreateTemp(store.dir, ".session-*.tmp")
	if err != nil {
		return State{}, err
	}
	temporaryName := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return State{}, err
	}
	if _, err := temporary.Write(encoded); err != nil {
		return State{}, err
	}
	if err := temporary.Sync(); err != nil {
		return State{}, err
	}
	if err := temporary.Close(); err != nil {
		return State{}, err
	}
	target := filepath.Join(store.dir, state.ID+".json")
	if err := os.Rename(temporaryName, target); err != nil {
		return State{}, err
	}
	keep = true
	if err := syncDirectory(store.dir); err != nil {
		return State{}, err
	}
	return state, nil
}

// Latest returns the newest session for a workspace, optionally excluding the active one.
func (store *Store) Latest(workspace, excludeID string) (State, error) {
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		return State{}, err
	}
	var latest State
	found := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(store.dir, entry.Name()))
		if err != nil {
			return State{}, err
		}
		var candidate State
		if err := json.Unmarshal(content, &candidate); err != nil {
			continue
		}
		if candidate.Version != version || candidate.Workspace != workspace || candidate.ID == excludeID {
			continue
		}
		if !found || candidate.UpdatedAt.After(latest.UpdatedAt) {
			latest = candidate
			found = true
		}
	}
	if !found {
		return State{}, ErrNotFound
	}
	return cloneState(latest), nil
}

// List returns saved sessions for a workspace, newest first.
func (store *Store) List(workspace string) ([]State, error) {
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		return nil, err
	}
	var sessions []State
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(store.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var candidate State
		if err := json.Unmarshal(content, &candidate); err != nil {
			continue
		}
		if candidate.Version == version && candidate.Workspace == workspace {
			sessions = append(sessions, cloneState(candidate))
		}
	}
	sort.SliceStable(sessions, func(left, right int) bool {
		return sessions[left].UpdatedAt.After(sessions[right].UpdatedAt)
	})
	return sessions, nil
}

// Load returns one saved session for a workspace by stable ID.
func (store *Store) Load(workspace, id string) (State, error) {
	sessions, err := store.List(workspace)
	if err != nil {
		return State{}, err
	}
	for _, candidate := range sessions {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return State{}, ErrNotFound
}

func cloneState(state State) State {
	state.Plan = append([]agent.PlanItem(nil), state.Plan...)
	state.Timeline = append([]TimelineEntry(nil), state.Timeline...)
	state.InputHistory = append([]string(nil), state.InputHistory...)
	return state
}

func stateDir() (string, error) {
	if configured := os.Getenv("XDG_STATE_HOME"); configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

func newID(now time.Time) (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return now.UTC().Format("20060102T150405.000000000") + "-" + hex.EncodeToString(random), nil
}

func validID(id string) bool {
	if id == "" || strings.Contains(id, "..") {
		return false
	}
	for _, character := range id {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '.' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
