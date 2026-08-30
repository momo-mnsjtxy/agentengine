package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/momo-mnsjtxy/agentengine/model"
)

func TestAdvanceWorkflowEnforcesOneStepAndEvidence(t *testing.T) {
	task := TaskState{Stage: StageGoal, StageEvidence: map[string]string{}}

	if _, err := task.advanceWorkflow(StagePlan, "skipped inspect"); err == nil {
		t.Fatal("forward advance was allowed to skip a stage")
	}
	if _, err := task.advanceWorkflow(StageInspect, ""); err == nil {
		t.Fatal("forward advance was allowed without evidence")
	}
	stage, err := task.advanceWorkflow(StageInspect, "read agent.go")
	if err != nil || stage != StageInspect {
		t.Fatalf("inspect advance=%q err=%v", stage, err)
	}
	if task.StageEvidence[string(StageInspect)] != "read agent.go" {
		t.Fatalf("stage evidence was not recorded: %#v", task.StageEvidence)
	}
	stage, err = task.advanceWorkflow(StageGoal, "")
	if err != nil || stage != StageGoal {
		t.Fatalf("rewind advance=%q err=%v", stage, err)
	}
	if _, err := task.advanceWorkflow("launch", "deployed"); err == nil {
		t.Fatal("unknown stage was accepted")
	}
}

func TestRunPublishesWorkflowWithoutCallingWorkspaceTool(t *testing.T) {
	provider := &fakeProvider{replies: []model.Reply{
		{ToolCalls: []model.ToolCall{{
			ID: "workflow_1", Type: "function",
			Function: model.FunctionCall{Name: "update_workflow", Arguments: `{"stage":"inspect","evidence":"read agent.go"}`},
		}}},
		{Content: "Inspecting."},
	}}
	toolbox := &fakeTools{}
	brain := New(provider, toolbox, "test")
	events := make(chan Event, 32)

	brain.Run(context.Background(), "inspect the project", events, make(chan ApprovalDecision))
	close(events)

	found := false
	for event := range events {
		if event.Kind == WorkflowChanged && event.Stage == StageInspect && event.Text == "read agent.go" {
			found = true
		}
	}
	if !found || toolbox.executed != 0 {
		t.Fatalf("workflow event=%v executed=%d", found, toolbox.executed)
	}
	snapshot := brain.Snapshot()
	if snapshot.Task.Stage != StageInspect || snapshot.Task.StageEvidence[string(StageInspect)] != "read agent.go" {
		t.Fatalf("workflow stage was not persisted: %#v", snapshot.Task)
	}
}

// update_workflow 是阶段唯一的移动方式：执行一个只读工具不改变阶段。
func TestToolCallNeverMovesStage(t *testing.T) {
	read := model.ToolCall{ID: "read_1", Type: "function", Function: model.FunctionCall{Name: "read_file", Arguments: `{}`}}
	provider := &fakeProvider{replies: []model.Reply{
		{ToolCalls: []model.ToolCall{read}},
		{Content: "Read it."},
	}}
	toolbox := &fakeTools{}
	brain := New(provider, toolbox, "test")
	events := make(chan Event, 16)
	approvals := make(chan ApprovalDecision, 1)
	approvals <- ApprovalDecision{Approved: true} // fakeTools 对一切调用都要审批

	brain.Run(context.Background(), "read one file", events, approvals)
	close(events)

	if toolbox.executed != 1 {
		t.Fatalf("read tool executed %d times, want 1", toolbox.executed)
	}
	if stage := brain.Snapshot().Task.Stage; stage != StageGoal {
		t.Fatalf("a tool call moved the stage to %s, want goal", stage)
	}
	for event := range events {
		if event.Kind == WorkflowChanged {
			t.Fatalf("a tool call published a workflow change to %s", event.Stage)
		}
	}
}

// update_plan 只写计划，不推进阶段，也不伪造阶段证据。
func TestUpdatePlanRecordsPlanWithoutMovingStage(t *testing.T) {
	plan := model.ToolCall{ID: "plan_1", Type: "function", Function: model.FunctionCall{
		Name:      "update_plan",
		Arguments: `{"explanation":"two steps","plan":[{"step":"read","status":"in_progress"},{"step":"edit","status":"pending"}]}`,
	}}
	provider := &fakeProvider{replies: []model.Reply{
		{ToolCalls: []model.ToolCall{plan}},
		{Content: "Planned."},
	}}
	brain := New(provider, &fakeTools{}, "test")
	events := make(chan Event, 16)

	brain.Run(context.Background(), "plan the work", events, make(chan ApprovalDecision))
	close(events)

	task := brain.Snapshot().Task
	if task.Stage != StageGoal {
		t.Fatalf("update_plan moved the stage to %s, want goal", task.Stage)
	}
	if len(task.StageEvidence) != 0 {
		t.Fatalf("update_plan invented stage evidence: %#v", task.StageEvidence)
	}
	if task.NextStep != "read" || len(task.Plan) != 2 {
		t.Fatalf("plan was not recorded: %#v", task)
	}
	for event := range events {
		if event.Kind == WorkflowChanged {
			t.Fatalf("update_plan published a workflow change to %s", event.Stage)
		}
	}
}

// 提示词必须由规则表生成：加一个阶段或改一条门禁，模型看到的说明要跟着变。
// 这个测试防止"代码拦了但提示词没说"重新出现。
func TestWorkflowRulesAreGeneratedFromTheRuleTables(t *testing.T) {
	rules := WorkflowRules()
	for _, stage := range WorkflowStages() {
		if !strings.Contains(rules, string(stage)) {
			t.Fatalf("stage %s is missing from the generated rules:\n%s", stage, rules)
		}
	}
	for _, rule := range capabilityRules {
		if !strings.Contains(rules, rule.label) {
			t.Fatalf("capability %s is missing from the generated rules:\n%s", rule.capability, rules)
		}
	}
	for _, gate := range stageGates {
		if !strings.Contains(rules, gate.requirement) {
			t.Fatalf("gate %q is missing from the generated rules:\n%s", gate.requirement, rules)
		}
	}
	if !strings.Contains(rules, rollbackRequirement) {
		t.Fatalf("the rollback requirement is missing from the generated rules:\n%s", rules)
	}
	if !strings.Contains(DefaultSystem(), rules) {
		t.Fatal("DefaultSystem no longer carries the generated workflow rules")
	}
}

func TestToolGateBlocksRunBeforeVerifyAndWritesOutsideImplement(t *testing.T) {
	for _, test := range []struct {
		stage      Stage
		capability Capability
		allowed    bool
	}{
		{StageGoal, CapabilityCommand, false},
		{StageInspect, CapabilityCommand, false},
		{StagePlan, CapabilityWrite, false},
		{StageVerify, CapabilityCommand, true},
		{StageRelease, CapabilityCommand, true},
		{StageImplement, CapabilityWrite, true},
		{StageInspect, CapabilityRead, true},
	} {
		if got := toolAllowed(test.stage, test.capability); got != test.allowed {
			t.Fatalf("toolAllowed(%s, %s)=%v want %v", test.stage, test.capability, got, test.allowed)
		}
	}

	provider := &fakeProvider{replies: []model.Reply{
		{ToolCalls: []model.ToolCall{{ID: "run_early", Type: "function", Function: model.FunctionCall{Name: "run", Arguments: `{"command":"bad"}`}}}},
		{Content: "Stopped."},
	}}
	toolbox := &fakeTools{}
	brain := New(provider, toolbox, "test")
	events := make(chan Event, 16)
	brain.Run(context.Background(), "run too early", events, make(chan ApprovalDecision))
	if toolbox.executed != 0 {
		t.Fatalf("blocked run executed %d times", toolbox.executed)
	}
	if stage := brain.Snapshot().Task.Stage; stage != StageGoal {
		t.Fatalf("blocked tool advanced stage to %s", stage)
	}
}

func TestAdvanceWorkflowRequiresVerificationBeforeReview(t *testing.T) {
	task := TaskState{Stage: StageVerify, StageEvidence: map[string]string{}}
	if _, err := task.advanceWorkflow(StageReview, "looks ready"); err == nil {
		t.Fatal("review was allowed before verification passed")
	}
	task.VerifyPassed = true
	stage, err := task.advanceWorkflow(StageReview, "tests passed")
	if err != nil || stage != StageReview {
		t.Fatalf("review advance=%q err=%v", stage, err)
	}
	if _, err := task.advanceWorkflow(StagePlan, ""); err != nil || task.VerifyPassed {
		t.Fatalf("rewind before verify should clear VerifyPassed: err=%v passed=%v", err, task.VerifyPassed)
	}
}

func TestAdvanceWorkflowRequiresReviewBeforeRelease(t *testing.T) {
	task := TaskState{Stage: StageReview, StageEvidence: map[string]string{}, VerifyPassed: true}
	if _, err := task.advanceWorkflow(StageRelease, "ready to deploy"); err == nil {
		t.Fatal("release was allowed before review passed")
	}
	task.ReviewPassed = true
	stage, err := task.advanceWorkflow(StageRelease, "ready to deploy")
	if err != nil || stage != StageRelease {
		t.Fatalf("release advance=%q err=%v", stage, err)
	}
	if _, err := task.advanceWorkflow(StageVerify, ""); err != nil || task.ReviewPassed {
		t.Fatalf("rewind before review should clear ReviewPassed: err=%v passed=%v", err, task.ReviewPassed)
	}
}

func TestRecordReviewMarksReviewPassed(t *testing.T) {
	brain := New(&fakeProvider{}, &fakeTools{}, "test")
	brain.task.Stage = StageReview
	brain.task.VerifyPassed = true
	call := model.ToolCall{ID: "review_1", Type: "function", Function: model.FunctionCall{Name: "review", Arguments: `{"summary":"small focused diff","risk":"low"}`}}
	events := make(chan Event, 4)
	brain.handleMeta(context.Background(), events, make(chan ApprovalDecision), call)
	snapshot := brain.Snapshot()
	if !snapshot.Task.ReviewPassed || snapshot.Task.ReviewSummary != "small focused diff" || snapshot.Task.ReviewRisk != "low" {
		t.Fatalf("review was not recorded: %#v", snapshot.Task)
	}
}

func TestRecordReviewRequiresReviewStageVerificationAndRisk(t *testing.T) {
	call := func(arguments string) model.ToolCall {
		return model.ToolCall{ID: "review_1", Type: "function", Function: model.FunctionCall{Name: "review", Arguments: arguments}}
	}

	brain := New(&fakeProvider{}, &fakeTools{}, "test")
	brain.handleMeta(context.Background(), make(chan Event, 4), nil, call(`{"summary":"early","risk":"low"}`))
	if brain.Snapshot().Task.ReviewPassed {
		t.Fatal("review passed outside the review stage")
	}

	brain.task.Stage = StageReview
	brain.handleMeta(context.Background(), make(chan Event, 4), nil, call(`{"summary":"unverified","risk":"low"}`))
	if brain.Snapshot().Task.ReviewPassed {
		t.Fatal("review passed before verification")
	}

	brain.task.VerifyPassed = true
	brain.handleMeta(context.Background(), make(chan Event, 4), nil, call(`{"summary":"missing risk"}`))
	if brain.Snapshot().Task.ReviewPassed {
		t.Fatal("review passed without a risk assessment")
	}
}

func TestReleaseRunRequiresSecondConfirmation(t *testing.T) {
	verifyRun := model.ToolCall{ID: "verify_run", Type: "function", Function: model.FunctionCall{Name: "run", Arguments: `{"command":"go test ./..."}`}}
	reviewDiff := model.ToolCall{ID: "review_diff", Type: "function", Function: model.FunctionCall{Name: "git_diff", Arguments: `{}`}}
	review := model.ToolCall{ID: "review_1", Type: "function", Function: model.FunctionCall{Name: "review", Arguments: `{"summary":"small focused diff","risk":"low"}`}}
	releaseRun := model.ToolCall{ID: "release_run", Type: "function", Function: model.FunctionCall{Name: "run", Arguments: `{"command":"echo deploy","rollback":"git revert HEAD"}`}}
	replies := append(repliesToStage(StageVerify),
		model.Reply{ToolCalls: []model.ToolCall{verifyRun}},
		workflowReply(StageReview, "tests passed"),
		model.Reply{ToolCalls: []model.ToolCall{reviewDiff}},
		model.Reply{ToolCalls: []model.ToolCall{review}},
		workflowReply(StageRelease, "ready to deploy"),
		model.Reply{ToolCalls: []model.ToolCall{releaseRun}},
		model.Reply{Content: "Deploy cancelled."},
	)
	toolbox := &releaseTools{}
	brain := New(&fakeProvider{replies: replies}, toolbox, "test")
	events := make(chan Event, 32)
	approvals := make(chan ApprovalDecision, 3)
	approvals <- ApprovalDecision{Approved: true}
	approvals <- ApprovalDecision{Approved: true}
	approvals <- ApprovalDecision{Approved: false}

	brain.Run(context.Background(), "deploy the release", events, approvals)
	if toolbox.runs != 1 || toolbox.diffs != 1 {
		t.Fatalf("unexpected release tool execution: runs=%d diffs=%d", toolbox.runs, toolbox.diffs)
	}
	foundDenial := false
	for _, message := range brain.history {
		if message.Role == "tool" && message.ToolCallID == "release_run" && message.Content == "User denied release confirmation." {
			foundDenial = true
		}
	}
	if !foundDenial {
		t.Fatal("release denial was not returned to the model")
	}
}

func TestReleaseRunRequiresRollback(t *testing.T) {
	verifyRun := model.ToolCall{ID: "verify_run", Type: "function", Function: model.FunctionCall{Name: "run", Arguments: `{"command":"go test ./..."}`}}
	reviewDiff := model.ToolCall{ID: "review_diff", Type: "function", Function: model.FunctionCall{Name: "git_diff", Arguments: `{}`}}
	review := model.ToolCall{ID: "review_1", Type: "function", Function: model.FunctionCall{Name: "review", Arguments: `{"summary":"small focused diff","risk":"low"}`}}
	releaseRun := model.ToolCall{ID: "release_run", Type: "function", Function: model.FunctionCall{Name: "run", Arguments: `{"command":"echo deploy"}`}}
	replies := append(repliesToStage(StageVerify),
		model.Reply{ToolCalls: []model.ToolCall{verifyRun}},
		workflowReply(StageReview, "tests passed"),
		model.Reply{ToolCalls: []model.ToolCall{reviewDiff}},
		model.Reply{ToolCalls: []model.ToolCall{review}},
		workflowReply(StageRelease, "ready to deploy"),
		model.Reply{ToolCalls: []model.ToolCall{releaseRun}},
		model.Reply{Content: "Release blocked."},
	)
	toolbox := &releaseTools{}
	brain := New(&fakeProvider{replies: replies}, toolbox, "test")
	events := make(chan Event, 32)
	approvals := make(chan ApprovalDecision, 1)
	approvals <- ApprovalDecision{Approved: true}

	brain.Run(context.Background(), "deploy without rollback", events, approvals)
	if toolbox.runs != 1 || toolbox.diffs != 1 {
		t.Fatalf("unexpected tool execution: runs=%d diffs=%d", toolbox.runs, toolbox.diffs)
	}
	found := false
	for _, message := range brain.history {
		if message.Role == "tool" && message.ToolCallID == "release_run" && message.Content == rollbackRequirement {
			found = true
		}
	}
	if !found {
		t.Fatal("missing rollback was not reported to the model")
	}
}

type releaseTools struct {
	runs  int
	diffs int
}

func (*releaseTools) Definitions() []model.Tool { return nil }
func (tools *releaseTools) Capability(call model.ToolCall) Capability {
	if call.Function.Name == "git_diff" {
		return CapabilityRead
	}
	return CapabilityCommand
}
func (*releaseTools) NeedsApproval(call model.ToolCall) bool {
	return call.Function.Name == "run"
}
func (*releaseTools) Preview(model.ToolCall) string { return "preview" }
func (tools *releaseTools) Execute(_ context.Context, call model.ToolCall) (model.Observation, error) {
	if call.Function.Name == "run" {
		tools.runs++
	}
	if call.Function.Name == "git_diff" {
		tools.diffs++
	}
	return model.Observation{Text: "ok"}, nil
}
