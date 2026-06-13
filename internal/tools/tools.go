package tools

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	execfiles "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/files"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	filetools "github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/files"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/weather"
)

// NewAgentTools returns the Eino tools available to the agent.
// NewAgentTools 返回 agent 可用的 Eino tools。
func NewAgentTools(workspaceGuard *securitycore.WorkspaceGuard, fileService *execfiles.Service) (adk.ToolsConfig, error) {
	weatherTool, err := weather.GetWeatherTool()
	if err != nil {
		return adk.ToolsConfig{}, err
	}
	fileTools, err := filetools.NewTools(workspaceGuard, fileService)
	if err != nil {
		return adk.ToolsConfig{}, err
	}
	allTools := []tool.BaseTool{weatherTool}
	allTools = append(allTools, fileTools...)
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: allTools,
		},
	}, nil
}

// NewCapabilityRegistry returns descriptors for the tools exposed by NewAgentTools.
// NewCapabilityRegistry 返回 NewAgentTools 暴露工具对应的 descriptor。
func NewCapabilityRegistry() (*capability.Registry, error) {
	descriptors := []capability.Descriptor{weather.CapabilityDescriptor()}
	descriptors = append(descriptors, filetools.CapabilityDescriptors()...)
	return capability.NewRegistry(descriptors...)
}

// NewOperationBuilders returns operation builders for tools exposed by NewAgentTools.
// NewOperationBuilders 返回 NewAgentTools 暴露工具对应的 operation builder。
func NewOperationBuilders() map[string]securitycore.OperationBuilder {
	builders := map[string]securitycore.OperationBuilder{
		"get_weather": weather.OperationBuilder(),
	}
	for name, builder := range filetools.OperationBuilders() {
		builders[name] = builder
	}
	return builders
}
