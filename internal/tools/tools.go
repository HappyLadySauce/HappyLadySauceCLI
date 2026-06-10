package tools

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/weather"
)

func NewAgentTools() adk.ToolsConfig {
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{
				weather.GetWeatherTool(),
			},
		},
	}
}
