package main

import (
	"context"
	"fmt"
	"os"

	"github.com/momo-mnsjtxy/agentengine/agent"
	"github.com/momo-mnsjtxy/agentengine/model"
	"github.com/momo-mnsjtxy/agentengine/openai"
	"github.com/momo-mnsjtxy/agentengine/video"
)

// demoTools makes the Agentengine loop runnable without a media provider. It
// returns deterministic asset references; replace Execute with calls to a real
// video.Provider when wiring production generation services.
type demoTools struct{}

func (demoTools) Definitions() []model.Tool                  { return video.ToolDefinitions() }
func (demoTools) Capability(model.ToolCall) agent.Capability { return agent.CapabilityWrite }
func (demoTools) NeedsApproval(model.ToolCall) bool          { return false }
func (demoTools) Preview(call model.ToolCall) string         { return call.Function.Name }
func (demoTools) Execute(_ context.Context, call model.ToolCall) (model.Observation, error) {
	return model.Observation{Text: fmt.Sprintf("demo tool completed: %s; asset_uri=demo://%s/result", call.Function.Name, call.ID)}, nil
}

func main() {
	cfg, err := video.ModelConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	provider := openai.New(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Protocol, cfg.Reasoning)
	system := agent.DefaultSystem() + "\n\n" + video.DefaultSystem()
	brain := agent.New(provider, demoTools{}, system)
	events := make(chan agent.Event)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := range events {
			if event.Text != "" {
				fmt.Printf("[%s] %s\n", event.Kind, event.Text)
			} else {
				fmt.Printf("[%s]\n", event.Kind)
			}
		}
	}()
	brain.Run(context.Background(), prompt(), events, nil)
	close(events)
	<-done
}

func prompt() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	return "制作一个 10 秒、16:9、科技感的产品介绍短视频，先给出镜头计划，再执行演示工具。"
}
