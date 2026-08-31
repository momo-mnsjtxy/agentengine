# Agentengine 设计

Agentengine 是编码 Agent 的引擎，构建在 [Agentcore](https://github.com/momo-mnsjtxy/agentcore) 之上：Agentcore 提供干净认知循环、工具闭环、审批、澄清、会话检查点、上下文压缩和模型连接；Agentengine 在此基础上叠加交付工作流、任务台账和产出元工具。它不拥有编码工具或界面。

## 与 Agentcore 的边界

- Agentcore 拥有：认知循环、工具闭环、审批、澄清、重复失败暂停、会话快照与恢复、上下文压缩、图像观察、Responses/Chat 协议。它不读 `AGENTS.md`，不假设工作区或产品身份。
- Agentengine 拥有：交付工作流（goal → inspect → plan → implement → verify → review → release 状态机与门禁）、有界任务台账、产出元工具（`update_plan` / `update_workflow` / `review`）和会话存储。
- `model` / `openai` 是 Agentcore 的类型别名，import 路径不变，下游代码无需改动。

## 主链

```text
应用给出目标
  -> Agent 组装当前会话
  -> Responses 或 Chat 流式推理
  -> 回答 / 推理 / 用量事件交给应用
  -> 模型请求工具、update_plan、update_workflow、review 或 request_clarification
  -> 必要时等待应用审批
  -> 执行工具，把文字和可选截图放回会话
  -> 继续推理，直到回答完成、失败或取消
```

完整循环在 `agent/agent.go`，可以从上到下直接阅读。

## 模块边界

- `agent` 拥有会话、推理轮次、交付工作流、任务台账、澄清、审批记忆和工具闭环。
- `agent/workflow.go` 拥有整条交付流水线：阶段顺序、前进条件、工具门禁、通过标记，以及喂给系统提示的规则文本。想知道「进入某阶段要满足什么」，只读这一个文件。`agent.go` 只问两件事：这次调用能不能做、这个结果是否翻转门禁。
- 阶段只能由 `update_workflow` 移动。工具调用不会顺带推进阶段，界面上看到的阶段就是模型明确声明过的阶段。
- `agent` 自己声明并执行 `update_plan`、`update_workflow`、`review`、`request_clarification`；调用方的 `Toolbox` 不应重复注册这些流程控制工具。一个元工具在 `agent/meta.go` 里是一条记录，模型看到的合约和引擎侧的处理写在一起。
- `openai` 是 Agentcore 的类型别名，只负责模型网络协议。Responses 是默认路径，Chat Completions 是兼容回退；连接错误与限流在首个输出事件前有限重试。
- `model` 是 Agentcore 的类型别名，保存模块之间最少的共享消息、图片和事件结构。
- `session` 原子保存已提交检查点，支持列表与恢复。不写 API key、环境变量或审批许可。
- 应用实现 `agent.Toolbox`（5 个必需方法），决定能做什么、什么要审批。另外可选实现 `WorkspaceDiffer`、`WorkspaceUndoer`、`FileMutator`，实现了就多一项能力。

## 应用怎么接

```go
brain := agent.New(provider, toolbox, systemPrompt)
brain.Run(ctx, goal, events, approvals)
```

系统提示和工具集属于应用。引擎不读取 `AGENTS.md`，也不假设工作区。`agent.DefaultSystem()` 提供通用交付工作流提示，应用在其上叠加产品身份与项目指令。其中的门禁段落由 `agent.WorkflowRules()` 从 `workflow.go` 的规则表生成——改规则表，模型看到的说明跟着变，不会出现「代码拦了但提示词没说」。

截图挂在工具观察的 `Images` 上，由 `openai` 编进 Responses / Chat 请求。图片不写入会话文件。会话里只保留最近 3 张图，避免旧画面占满上下文。
