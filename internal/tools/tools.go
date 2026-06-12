package tools

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/weather"
)

// NewAgentTools returns the Eino tools available to the agent.
// NewAgentTools 返回 agent 可用的 Eino tools。
func NewAgentTools() (adk.ToolsConfig, error) {
	weatherTool, err := weather.GetWeatherTool()
	if err != nil {
		return adk.ToolsConfig{}, err
	}
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{
				weatherTool,
			},
		},
	}, nil
}

// NewCapabilityRegistry returns descriptors for the tools exposed by NewAgentTools.
// NewCapabilityRegistry 返回 NewAgentTools 暴露工具对应的 descriptor。
func NewCapabilityRegistry() (*capability.Registry, error) {
	return capability.NewRegistry(weather.CapabilityDescriptor())
}

// NewOperationBuilders returns operation builders for tools exposed by NewAgentTools.
// NewOperationBuilders 返回 NewAgentTools 暴露工具对应的 operation builder。
func NewOperationBuilders() map[string]securitycore.OperationBuilder {
	return map[string]securitycore.OperationBuilder{
		"get_weather": weather.OperationBuilder(),
	}
}
