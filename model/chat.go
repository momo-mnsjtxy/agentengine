/*
模型对话协议：由 agentcore 定义，这里以类型别名重新导出。
agentengine 保持同一 import 路径，下游代码无需改动。
*/
package model

import (
	core "github.com/momo-mnsjtxy/agentcore/model"
)

type Message = core.Message
type Image = core.Image
type Observation = core.Observation
type ToolCall = core.ToolCall
type FunctionCall = core.FunctionCall
type Tool = core.Tool
type FunctionTool = core.FunctionTool
type Reply = core.Reply
type Usage = core.Usage
type StreamKind = core.StreamKind

const (
	OutputDelta    StreamKind = core.OutputDelta
	ReasoningDelta StreamKind = core.ReasoningDelta
	UsageChanged   StreamKind = core.UsageChanged
)

type StreamEvent = core.StreamEvent
type Stream = core.Stream
type Provider = core.Provider
