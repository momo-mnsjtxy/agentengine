/*
OpenAI 模型连接：由 agentcore 实现，这里以别名重新导出。
agentengine 保持同一 import 路径，下游代码无需改动。
*/
package openai

import (
	core "github.com/momo-mnsjtxy/agentcore/openai"
)

type Client = core.Client

func New(baseURL, apiKey, modelName, protocol, reasoning string) *Client {
	return core.New(baseURL, apiKey, modelName, protocol, reasoning)
}
