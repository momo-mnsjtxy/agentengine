/*
元工具：由引擎自己消化、不交给 Toolbox 的模型工具。
update_workflow / review / update_plan / request_clarification 都是引擎的流程控制，
不是工作区动作。加一个元工具 = 在 metaTools 里加一行，主循环不需要改。
*/
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/momo-mnsjtxy/agentengine/model"
)

// metaTool 是一个由引擎直接处理的模型工具：definition 是模型看到的合约，run 是引擎侧的处理。
// run 返回 ok（这次调用是否成立，false 会作为阻塞记进台账）、回填给模型的文本，
// 以及 stop（为真表示应立刻结束整个运行，例如用户取消）。
type metaTool struct {
	definition model.Tool
	run        func(*Agent, context.Context, chan<- Event, <-chan ApprovalDecision, model.ToolCall) (bool, string, bool)
}

// metaTools 是引擎认识的全部元工具。一个元工具一条记录，合约与处理写在一起，
// 加一个就是往这个列表里加一项，主循环和 toolDefinitions 都不需要改。
var metaTools = []metaTool{
	{
		run: (*Agent).updateWorkflow,
		definition: model.Tool{
			Type: "function",
			Function: model.FunctionTool{
				Name:        "update_workflow",
				Description: "Advance or rewind the delivery workflow by one stage, with evidence for forward progress.",
				Parameters: map[string]any{
					"type":     "object",
					"required": []string{"stage", "evidence"},
					"properties": map[string]any{
						"stage":    map[string]any{"type": "string", "enum": []string{"goal", "inspect", "plan", "implement", "verify", "review", "release"}},
						"evidence": map[string]any{"type": "string"},
					},
				},
			},
		},
	},
	{
		run: (*Agent).recordReview,
		definition: model.Tool{
			Type: "function",
			Function: model.FunctionTool{
				Name:        "review",
				Description: "Record the implementation review summary and risk before release.",
				Parameters: map[string]any{
					"type":     "object",
					"required": []string{"summary", "risk"},
					"properties": map[string]any{
						"summary": map[string]any{"type": "string"},
						"risk":    map[string]any{"type": "string"},
					},
				},
			},
		},
	},
	{
		run: (*Agent).updatePlan,
		definition: model.Tool{
			Type: "function",
			Function: model.FunctionTool{
				Name:        "update_plan",
				Description: "Replace the short task plan; keep exactly one step in_progress.",
				Parameters: map[string]any{
					"type":     "object",
					"required": []string{"plan"},
					"properties": map[string]any{
						"explanation": map[string]any{"type": "string"},
						"plan": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"step", "status"},
								"properties": map[string]any{
									"step":   map[string]any{"type": "string"},
									"status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
								},
							},
						},
					},
				},
			},
		},
	},
	{
		run: (*Agent).requestClarification,
		definition: model.Tool{
			Type: "function",
			Function: model.FunctionTool{
				Name:        "request_clarification",
				Description: "Pause and ask the user for missing information before continuing.",
				Parameters: map[string]any{
					"type":     "object",
					"required": []string{"question"},
					"properties": map[string]any{
						"question": map[string]any{"type": "string"},
						"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
				},
			},
		},
	},
}

// handleMeta 分派元工具。返回 handled 表示命中了元工具，stop 表示应立刻结束运行。
func (agent *Agent) handleMeta(runContext context.Context, events chan<- Event, approvals <-chan ApprovalDecision, call model.ToolCall) (bool, bool) {
	for _, meta := range metaTools {
		if call.Function.Name != meta.definition.Function.Name {
			continue
		}
		ok, output, stop := meta.run(agent, runContext, events, approvals, call)
		agent.appendMessage(model.Message{Role: "tool", ToolCallID: call.ID, Content: output})
		agent.recordToolOutcome(call, "", output, !ok)
		return true, stop
	}
	return false, false
}

// updatePlan 只替换计划，不动阶段。计划内容和当前阶段是两件独立的事：
// 模型可以在 inspect 阶段就写好计划，进入 plan 阶段要显式调 update_workflow。
func (agent *Agent) updatePlan(runContext context.Context, events chan<- Event, approvals <-chan ApprovalDecision, call model.ToolCall) (bool, string, bool) {
	var update struct {
		Explanation string     `json:"explanation"`
		Plan        []PlanItem `json:"plan"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &update); err != nil {
		return false, "Plan update failed: " + err.Error(), false
	}
	send(runContext, events, Event{Kind: PlanChanged, Text: update.Explanation, Plan: update.Plan})
	agent.setPlan(update.Plan)
	return true, "Plan updated.", false
}

// updateWorkflow applies an explicit stage transition from the model. It is the only way
// to cross a stage boundary, and evidence is required for every forward step.
func (agent *Agent) updateWorkflow(runContext context.Context, events chan<- Event, approvals <-chan ApprovalDecision, call model.ToolCall) (bool, string, bool) {
	var update struct {
		Stage    Stage  `json:"stage"`
		Evidence string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &update); err != nil {
		return false, "Workflow update failed: " + err.Error(), false
	}
	previous, stage, err := agent.advanceWorkflowStage(update.Stage, update.Evidence)
	if err != nil {
		return false, "Workflow update failed: " + err.Error(), false
	}
	if stage != previous {
		send(runContext, events, Event{Kind: WorkflowChanged, Stage: stage, Text: update.Evidence})
	}
	return true, fmt.Sprintf("Workflow stage is now %s", stage), false
}

// recordReview stores the model's diff review conclusion and marks the review gate passed.
func (agent *Agent) recordReview(runContext context.Context, events chan<- Event, approvals <-chan ApprovalDecision, call model.ToolCall) (bool, string, bool) {
	var review struct {
		Summary string `json:"summary"`
		Risk    string `json:"risk"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &review); err != nil {
		return false, "Review failed: " + err.Error(), false
	}
	review.Summary = strings.TrimSpace(review.Summary)
	review.Risk = strings.TrimSpace(review.Risk)
	switch stage := agent.currentStage(); {
	case stage != StageReview:
		return false, fmt.Sprintf("Review failed: workflow stage is %s, expected review.", stage), false
	case !agent.verificationPassed():
		return false, "Review failed: verification must pass first.", false
	case review.Summary == "":
		return false, "Review failed: summary is required.", false
	case review.Risk == "":
		return false, "Review failed: risk is required.", false
	}
	agent.passReview(review.Summary, review.Risk)
	send(runContext, events, Event{Kind: ReviewChanged, Review: review.Summary, Risk: review.Risk})
	return true, "Review recorded.", false
}

func (agent *Agent) requestClarification(runContext context.Context, events chan<- Event, approvals <-chan ApprovalDecision, call model.ToolCall) (bool, string, bool) {
	var input struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
		return false, "Clarification request failed: " + err.Error(), false
	}
	input.Question = strings.TrimSpace(input.Question)
	if input.Question == "" {
		input.Question = "Please provide the missing information."
	}
	send(runContext, events, Event{Kind: Clarification, Text: input.Question, Call: call})

	decision, received := receiveDecision(runContext, approvals)
	if !received {
		send(runContext, events, Event{Kind: Finished, Text: "Cancelled"})
		return false, "", true
	}
	// 用户主动跳过问题不是失败，是一个决定；两种回答都让模型继续往下走。
	if !decision.Approved {
		return true, "User dismissed the clarification.", false
	}
	if answer := strings.TrimSpace(decision.Answer); answer != "" {
		return true, "User answered: " + answer, false
	}
	return true, "User answered the clarification.", false
}

func (agent *Agent) requestRepeatedFailureClarification(runContext context.Context, events chan<- Event, approvals <-chan ApprovalDecision, failedCall model.ToolCall, output string) bool {
	question := fmt.Sprintf("The same action failed repeatedly: %s. How should I proceed?", failedCall.Function.Name)
	arguments, _ := json.Marshal(map[string]any{
		"question": question,
		"options":  []string{"Retry with a different approach", "Stop and explain the blocker", "Show the full output"},
	})
	call := model.ToolCall{
		ID:       "clarify_repeated_failure",
		Type:     "function",
		Function: model.FunctionCall{Name: "request_clarification", Arguments: string(arguments)},
	}
	agent.appendMessage(model.Message{Role: "assistant", ToolCalls: []model.ToolCall{call}})
	send(runContext, events, Event{Kind: Clarification, Text: question, Call: call})
	answer := "User dismissed the clarification."
	decision, received := receiveDecision(runContext, approvals)
	if !received {
		send(runContext, events, Event{Kind: Finished, Text: "Cancelled"})
		return true
	}
	if decision.Approved && strings.TrimSpace(decision.Answer) != "" {
		answer = "User answered: " + strings.TrimSpace(decision.Answer)
	}
	agent.appendMessage(model.Message{Role: "tool", ToolCallID: call.ID, Content: answer})
	agent.recordToolOutcome(call, "", answer, false)
	return false
}
