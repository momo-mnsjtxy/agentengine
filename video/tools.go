package video

import "github.com/momo-mnsjtxy/agentengine/model"

const (
	GenerateImage = "generate_image"
	GenerateVideo = "generate_video"
	GenerateAudio = "generate_audio"
	MakeSubtitle  = "make_subtitle"
	ComposeVideo  = "compose_video"
	ProbeMedia    = "probe_media"
	StoreAsset    = "store_asset"
)

// ToolDefinitions returns the stable contracts for the media tools expected by
// the default video system prompt. Applications still implement execution,
// approval and provider selection in their Toolbox.
func ToolDefinitions() []model.Tool {
	return []model.Tool{
		mediaTool(GenerateImage, "Generate a still image asset for a shot.", "prompt"),
		mediaTool(GenerateVideo, "Generate a video clip for a shot.", "prompt"),
		mediaTool(GenerateAudio, "Generate narration, sound effects, or music.", "prompt"),
		mediaTool(MakeSubtitle, "Create subtitles from an audio or video asset.", "asset_uri"),
		mediaTool(ComposeVideo, "Compose clips, audio, and subtitles into a video.", "asset_uris"),
		mediaTool(ProbeMedia, "Inspect media metadata and quality signals.", "asset_uri"),
		mediaTool(StoreAsset, "Store a generated media asset and return its URI.", "asset_uri"),
	}
}

func mediaTool(name, description, required string) model.Tool {
	property := map[string]any{"type": "string"}
	if required == "asset_uris" {
		property = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	}
	return model.Tool{Type: "function", Function: model.FunctionTool{
		Name: name, Description: description,
		Parameters: map[string]any{
			"type": "object", "required": []string{required},
			"properties": map[string]any{required: property},
		},
	}}
}
