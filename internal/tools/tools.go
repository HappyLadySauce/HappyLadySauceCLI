package tools

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/weather"
)

// NewAgentTools returns the Eino tools available to the agent.
// NewAgentTools 返回 agent 可用的 Eino tools。
func NewAgentTools() adk.ToolsConfig {
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{
				weather.GetWeatherTool(),
			},
		},
	}
}

// NewCapabilityRegistry returns descriptors for the tools exposed by NewAgentTools.
// NewCapabilityRegistry 返回 NewAgentTools 暴露工具对应的 descriptor。
func NewCapabilityRegistry() (*capability.Registry, error) {
	return capability.NewRegistry(weather.CapabilityDescriptor())
}
