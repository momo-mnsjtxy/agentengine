package video

import "testing"

func TestToolDefinitionsAreStable(t *testing.T) {
	tools := ToolDefinitions()
	if len(tools) != 7 {
		t.Fatalf("got %d tools, want 7", len(tools))
	}
	if tools[1].Function.Name != GenerateVideo || tools[4].Function.Name != ComposeVideo {
		t.Fatalf("unexpected tool order")
	}
}
