# Agentengine

Agentengine 是编码 Agent 的引擎，一个 Go 库：**交付工作流、任务台账和产出机制**，构建在 [Agentcore](https://github.com/momo-mnsjtxy/agentcore) 的干净认知循环之上。Agentcore 提供认知循环、工具闭环、审批、澄清、会话检查点、上下文压缩和模型连接；Agentengine 在其上叠加 goal → inspect → plan → implement → verify → review → release 工作流、有界任务台账和 `update_plan` / `update_workflow` / `review` 产出元工具。工作区工具和 TUI 属于应用（见 [Astraxis](https://github.com/momo-mnsjtxy/astraxis)），由调用方以 Toolbox 的身份传入。

`model` / `openai` 是 Agentcore 的类型别名，import 路径不变，下游代码无需改动。

引擎保持 HOP 的两条边界：系统提示由调用方提供（引擎不读 `AGENTS.md`、不假设工作区），工具结果可以带图。

## 已有能力

- 交付工作流（goal → inspect → plan → implement → verify → review → release）：`update_workflow` 是阶段唯一的移动方式，工具调用本身不移动阶段
- 门禁规则集中在 `agent/workflow.go` 的规则表，系统提示里的说明由 `agent.WorkflowRules()` 从同一张表生成
- 有界任务台账：目标、下一步、计划、修改文件、阻塞、最近决策
- 产出元工具：`update_plan` 计划事件、`review` 审查记录、release 二次确认与回滚要求
- 引擎自动向模型注册 `update_plan`、`update_workflow`、`review`，应用工具箱只需描述工作区能力
- 复用 Agentcore 的认知循环：目标 → 推理 → 工具 → 观察 → 继续，直到完成、失败或取消
- 复用 Agentcore 的工具闭环、审批、澄清、会话检查点与上下文压缩
- OpenAI Responses API 流式协议（默认），Chat Completions 回退
- 连接错误与限流在首个输出事件前的有限重试
- 读操作自动执行，写入类动作由应用决定是否审批，支持记住本次决定
- 应用不提供审批通道时高风险动作默认拒绝；不需要事件流时可传 `nil`
- 工具观察可附带图片，供 computer use 一类应用把屏幕送给模型；会话只保留最近 3 张图

## 使用

```go
provider := openai.New(baseURL, apiKey, model, "responses", "medium")
brain := agent.New(provider, toolbox, systemPrompt)
brain.Run(ctx, goal, events, approvals)
```

`toolbox` 实现 `agent.Toolbox`（5 个必需方法），`systemPrompt` 由应用组装（可叠加 `agent.DefaultSystem()`）。会话文件默认写在 `$XDG_STATE_HOME/agentengine/sessions`。

Toolbox 另外可以实现三个可选接口，实现了就多一项能力，不实现引擎照常工作：

| 接口 | 多出来的能力 |
|---|---|
| `agent.WorkspaceDiffer` | `Agent.WorkspaceDiff` 可用（review 阶段看改动） |
| `agent.WorkspaceUndoer` | `Agent.UndoLastChange` 可用 |
| `agent.FileMutator` | 任务台账记录「修改文件」 |

## 项目结构

```text
agent/agent.go       复用 Agentcore 的认知循环，叠加审批、工具闭环
agent/workflow.go    交付流水线：阶段顺序、前进条件、工具门禁、通过标记、提示词规则
agent/meta.go        产出元工具（update_plan / update_workflow / review）
agent/task.go        有界任务台账
agent/compact.go     按用户回合压缩旧上下文
model/               Agentcore 类型别名
openai/              Agentcore 类型别名
session/store.go     原子会话检查点、列表与恢复
```

`agentcore` 提供认知循环、工具闭环、审批、澄清、会话快照与恢复、上下文压缩和模型连接。架构边界见 [DESIGN.md](DESIGN.md)。代码遵循 [HOP](https://github.com/kernel4632/HOP)。
