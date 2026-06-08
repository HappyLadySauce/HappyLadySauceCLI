package tools

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/adk"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/weather"
)

type AgentTools struct {
	Tools []adk.ToolsConfig
}

func NewAgentTools() adk.ToolsConfig {
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{
				weather.GetWeatherTool(),
			},
		},
	}
}