/*
Task ledger: keep a bounded, persistent summary of the current goal and execution state.
It is deliberately small so restoring a session does not depend on replaying the full transcript.
*/
package agent

import (
	"strings"

	"github.com/momo-mnsjtxy/agentengine/model"
)

const (
	maxTaskDecisions    = 20
	maxTaskBlockers     = 8
	maxTaskFiles        = 120
	maxTaskObservation  = 1200
	maxTaskDecisionRune = 180
	maxTaskPlanContext  = 24
)

// TaskDecision is a compact record of one action and its observed outcome.
type TaskDecision struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

// TaskState is the next-step oriented task ledger persisted with an Agent snapshot.
type TaskState struct {
	Goal            string            `json:"goal,omitempty"`
	NextStep        string            `json:"next_step,omitempty"`
	Stage           Stage             `json:"stage,omitempty"`
	StageEvidence   map[string]string `json:"stage_evidence,omitempty"`
	VerifyPassed    bool              `json:"verify_passed,omitempty"`
	ReviewPassed    bool              `json:"review_passed,omitempty"`
	ReviewSummary   string            `json:"review_summary,omitempty"`
	ReviewRisk      string            `json:"review_risk,omitempty"`
	Plan            []PlanItem        `json:"plan,omitempty"`
	Decisions       []TaskDecision    `json:"decisions,omitempty"`
	ModifiedFiles   []string          `json:"modified_files,omitempty"`
	Blockers        []string          `json:"blockers,omitempty"`
	LastTool        string            `json:"last_tool,omitempty"`
	LastObservation string            `json:"last_observation,omitempty"`
}

func cloneTask(task TaskState) TaskState {
	task.Plan = append([]PlanItem(nil), task.Plan...)
	task.Decisions = append([]TaskDecision(nil), task.Decisions...)
	task.ModifiedFiles = append([]string(nil), task.ModifiedFiles...)
	task.Blockers = append([]string(nil), task.Blockers...)
	if task.StageEvidence != nil {
		task.StageEvidence = cloneStringMap(task.StageEvidence)
	}
	return task
}

func (task *TaskState) setGoal(goal string) {
	task.Goal = goal
	task.NextStep = ""
	task.Stage = StageGoal
	task.StageEvidence = map[string]string{}
	task.VerifyPassed = false
	task.ReviewPassed = false
	task.ReviewSummary = ""
	task.ReviewRisk = ""
}

func (task *TaskState) setPlan(plan []PlanItem) {
	task.Plan = append([]PlanItem(nil), plan...)
	task.refreshNextStep()
}

func (task *TaskState) recordTool(call model.ToolCall, modified []string, output string, failed bool) {
	task.LastTool = call.Function.Name
	task.LastObservation = taskObservation(output)
	task.Decisions = append(task.Decisions, TaskDecision{
		Tool:    call.Function.Name,
		Summary: taskDecisionSummary(call.Function.Name, output),
	})
	if len(task.Decisions) > maxTaskDecisions {
		task.Decisions = task.Decisions[len(task.Decisions)-maxTaskDecisions:]
	}
	for _, path := range modified {
		task.recordModifiedFile(path)
	}
	if failed {
		task.addBlocker(call.Function.Name + " failed: " + firstTaskLine(output))
	}
}

func (task *TaskState) refreshNextStep() {
	for _, item := range task.Plan {
		if item.Status == "in_progress" {
			task.NextStep = item.Step
			return
		}
	}
	for _, item := range task.Plan {
		if item.Status == "pending" {
			task.NextStep = item.Step
			return
		}
	}
	task.NextStep = ""
}

func (task *TaskState) recordModifiedFile(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, existing := range task.ModifiedFiles {
		if existing == path {
			return
		}
	}
	task.ModifiedFiles = append(task.ModifiedFiles, path)
	if len(task.ModifiedFiles) > maxTaskFiles {
		task.ModifiedFiles = task.ModifiedFiles[len(task.ModifiedFiles)-maxTaskFiles:]
	}
}

func (task *TaskState) addBlocker(blocker string) {
	blocker = strings.TrimSpace(blocker)
	if blocker == "" {
		return
	}
	for _, existing := range task.Blockers {
		if existing == blocker {
			return
		}
	}
	task.Blockers = append(task.Blockers, blocker)
	if len(task.Blockers) > maxTaskBlockers {
		task.Blockers = task.Blockers[len(task.Blockers)-maxTaskBlockers:]
	}
}

func (task TaskState) context() string {
	if task.Goal == "" {
		return ""
	}
	var output strings.Builder
	output.WriteString("Current task state:\n")
	output.WriteString("Goal: " + task.Goal + "\n")
	output.WriteString("Workflow stage: " + string(task.stage()) + "\n")
	if evidence := task.StageEvidence[string(task.stage())]; evidence != "" {
		output.WriteString("Stage evidence: " + evidence + "\n")
	}
	if task.VerifyPassed {
		output.WriteString("Verification: passed\n")
	}
	if task.ReviewPassed {
		output.WriteString("Review: passed\n")
		if task.ReviewSummary != "" {
			output.WriteString("Review summary: " + task.ReviewSummary + "\n")
		}
		if task.ReviewRisk != "" {
			output.WriteString("Review risk: " + task.ReviewRisk + "\n")
		}
	}
	plan := task.Plan
	if len(plan) > maxTaskPlanContext {
		plan = plan[:maxTaskPlanContext]
	}
	if len(plan) > 0 {
		output.WriteString("Plan:\n")
		for _, item := range plan {
			output.WriteString("- [" + valueOr(item.Status, "pending") + "] " + item.Step + "\n")
		}
	}
	if task.NextStep != "" {
		output.WriteString("Next step: " + task.NextStep + "\n")
	}
	if len(task.ModifiedFiles) > 0 {
		output.WriteString("Modified files:\n")
		for _, path := range task.ModifiedFiles {
			output.WriteString("- " + path + "\n")
		}
	}
	if len(task.Blockers) > 0 {
		output.WriteString("Blockers:\n")
		for _, blocker := range task.Blockers {
			output.WriteString("- " + blocker + "\n")
		}
	}
	if len(task.Decisions) > 0 {
		output.WriteString("Recent decisions:\n")
		start := max(0, len(task.Decisions)-5)
		for _, decision := range task.Decisions[start:] {
			output.WriteString("- " + decision.Tool + ": " + decision.Summary + "\n")
		}
	}
	return strings.TrimSpace(output.String())
}

func taskObservation(output string) string {
	return truncateRunes(strings.Join(strings.Fields(output), " "), maxTaskObservation)
}

func taskDecisionSummary(name, output string) string {
	summary := strings.Join(strings.Fields(output), " ")
	if summary == "" {
		summary = name
	}
	return truncateRunes(summary, maxTaskDecisionRune)
}

func firstTaskLine(output string) string {
	if line, _, ok := strings.Cut(output, "\n"); ok {
		return truncateRunes(strings.TrimSpace(line), 120)
	}
	return truncateRunes(strings.TrimSpace(output), 120)
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
