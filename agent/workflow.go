/*
交付流水线：这个文件拥有完整的 rail —— 阶段顺序、前进条件、工具门禁、通过标记。
update_workflow 是唯一的前进方式；工具调用本身永远不移动阶段。
agent.go 只向这里问两个问题：这次调用能不能做（refuse）、这个结果是否翻转门禁（recordVerification）。
*/
package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/momo-mnsjtxy/agentengine/model"
)

// Stage is one position in the default delivery pipeline.
type Stage string

const (
	StageGoal      Stage = "goal"
	StageInspect   Stage = "inspect"
	StagePlan      Stage = "plan"
	StageImplement Stage = "implement"
	StageVerify    Stage = "verify"
	StageReview    Stage = "review"
	StageRelease   Stage = "release"
)

func WorkflowStages() []Stage {
	return []Stage{StageGoal, StageInspect, StagePlan, StageImplement, StageVerify, StageReview, StageRelease}
}

func stageIndex(stage Stage) int {
	if stage == "" {
		stage = StageGoal
	}
	for index, candidate := range WorkflowStages() {
		if candidate == stage {
			return index
		}
	}
	return -1
}

// stageGate 是进入某个阶段的前置条件。requirement 既是拒绝理由，也是提示词里告诉模型的那句话，
// 所以规则只有一份：改这张表，门禁和提示词一起变。
type stageGate struct {
	stage       Stage
	requirement string
	satisfied   func(TaskState) bool
}

var stageGates = []stageGate{
	{StageReview, "verification to pass", func(task TaskState) bool { return task.VerifyPassed }},
	{StageRelease, "verification to pass", func(task TaskState) bool { return task.VerifyPassed }},
	{StageRelease, "the review tool to record a summary and risk", func(task TaskState) bool { return task.ReviewPassed }},
}

// advanceWorkflow 是阶段唯一的移动入口。前进一次只许一步、必须带证据、必须满足目标阶段的门禁；
// 回退总是允许，但会清掉它跨过的通过标记，避免旧证据为新代码背书。
func (task *TaskState) advanceWorkflow(target Stage, evidence string) (Stage, error) {
	target = Stage(strings.ToLower(strings.TrimSpace(string(target))))
	targetIndex := stageIndex(target)
	if target == "" || targetIndex < 0 {
		return task.Stage, fmt.Errorf("unknown workflow stage %q", target)
	}
	evidence = strings.TrimSpace(evidence)
	currentIndex := stageIndex(task.Stage)
	if targetIndex > currentIndex {
		if targetIndex > currentIndex+1 {
			return task.Stage, fmt.Errorf("advance one workflow stage at a time")
		}
		if evidence == "" {
			return task.Stage, fmt.Errorf("evidence is required to advance to %s", target)
		}
		for _, gate := range stageGates {
			if gate.stage == target && !gate.satisfied(*task) {
				return task.Stage, fmt.Errorf("%s requires %s", target, gate.requirement)
			}
		}
	}
	task.Stage = target
	if targetIndex < stageIndex(StageVerify) {
		task.VerifyPassed = false
	}
	if targetIndex < stageIndex(StageReview) {
		task.ReviewPassed = false
	}
	if task.StageEvidence == nil {
		task.StageEvidence = map[string]string{}
	}
	task.StageEvidence[string(target)] = evidence
	return task.Stage, nil
}

// Capability 描述工具对工作区的改动程度，由应用的 Toolbox 声明，用于阶段门禁。
type Capability string

const (
	CapabilityRead    Capability = "read"    // 只读观察，始终放行
	CapabilityWrite   Capability = "write"   // 修改文件，只在 implement
	CapabilityCommand Capability = "command" // 运行命令，只在 verify / release
)

// capabilityRule 把一种能力限制在若干阶段内。label 是提示词里的说法，
// 和 stageGates 一样：这张表同时喂给门禁和提示词，不存在两份规则。
// 表里没有的能力（read）不受阶段限制。
type capabilityRule struct {
	capability Capability
	label      string
	stages     []Stage
}

var capabilityRules = []capabilityRule{
	{CapabilityWrite, "File modifications", []Stage{StageImplement}},
	{CapabilityCommand, "Commands", []Stage{StageVerify, StageRelease}},
}

// rollbackRequirement 是 release 命令必须自带回滚命令的规则，refuse 和提示词共用这一句。
const rollbackRequirement = "Release commands must include a rollback command."

// toolAllowed 查表判断某能力的工具在某阶段是否放行。
func toolAllowed(stage Stage, capability Capability) bool {
	for _, rule := range capabilityRules {
		if rule.capability != capability {
			continue
		}
		for _, allowed := range rule.stages {
			if allowed == stage {
				return true
			}
		}
		return false
	}
	return true
}

// refuse 返回拒绝这次调用的原因，空字符串表示放行。
// 原因文本会作为工具结果回填给模型，所以它同时是给模型的纠正说明。
func (task TaskState) refuse(call model.ToolCall, capability Capability) string {
	stage := task.stage()
	if !toolAllowed(stage, capability) {
		return fmt.Sprintf("Tool %s is not allowed in workflow stage %s.", call.Function.Name, stage)
	}
	if task.needsReleaseConfirmation(capability) && releaseRollback(call) == "" {
		return rollbackRequirement
	}
	return ""
}

// needsReleaseConfirmation 报告这次调用是否需要二次确认：只有 release 阶段的命令需要。
func (task TaskState) needsReleaseConfirmation(capability Capability) bool {
	return capability == CapabilityCommand && task.stage() == StageRelease
}

// recordVerification 在 verify 阶段的命令成功后立起验证门禁，这是 VerifyPassed 唯一的置真点。
func (task *TaskState) recordVerification(capability Capability, failed bool) {
	if !failed && capability == CapabilityCommand && task.stage() == StageVerify {
		task.VerifyPassed = true
	}
}

// passReview 立起审查门禁，这是 ReviewPassed 唯一的置真点。前置条件由 recordReview 检查。
func (task *TaskState) passReview(summary, risk string) {
	task.ReviewPassed = true
	task.ReviewSummary = summary
	task.ReviewRisk = risk
}

func (task TaskState) stage() Stage {
	if task.Stage == "" {
		return StageGoal
	}
	return task.Stage
}

// WorkflowRules 用大白话写出这条 rail 的全部规则，供系统提示使用。
// 它从 WorkflowStages / capabilityRules / stageGates 生成，所以改了表，
// 模型看到的说明会跟着变，不会出现"代码拦了但提示词没说"的情况。
func WorkflowRules() string {
	stages := make([]string, 0, len(WorkflowStages()))
	for _, stage := range WorkflowStages() {
		stages = append(stages, string(stage))
	}

	var rules strings.Builder
	rules.WriteString("Follow the delivery workflow: " + strings.Join(stages, " -> ") + ".")
	rules.WriteString(" Move one stage at a time with update_workflow and evidence; a tool call never moves the stage by itself.")
	for _, rule := range capabilityRules {
		rules.WriteString(fmt.Sprintf(" %s are allowed only in %s.", rule.label, joinStages(rule.stages)))
	}
	// 一个阶段可能有多个门禁，按阶段合成一句，避免同一个阶段说好几遍。
	for _, stage := range WorkflowStages() {
		var requirements []string
		for _, gate := range stageGates {
			if gate.stage == stage {
				requirements = append(requirements, gate.requirement)
			}
		}
		if len(requirements) > 0 {
			rules.WriteString(fmt.Sprintf(" Advancing to %s requires %s.", stage, joinWords(requirements)))
		}
	}
	rules.WriteString(" In review, inspect the workspace diff and record a summary and risk with the review tool. ")
	rules.WriteString(rollbackRequirement)
	return rules.String()
}

func joinStages(stages []Stage) string {
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, string(stage))
	}
	return joinList(names, "or")
}

func joinWords(words []string) string {
	return joinList(words, "and")
}

func joinList(items []string, conjunction string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + conjunction + " " + items[len(items)-1]
}

// releaseRollback 读出 release 命令自带的回滚命令。
func releaseRollback(call model.ToolCall) string {
	var input struct {
		Rollback string `json:"rollback"`
	}
	if json.Unmarshal([]byte(call.Function.Arguments), &input) != nil {
		return ""
	}
	return strings.TrimSpace(input.Rollback)
}
