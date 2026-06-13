// Package files exposes guarded filesystem tools to the agent.
// Package files 向 agent 暴露受保护的文件系统工具。
package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	execfiles "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/files"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/execguard"
)

const (
	toolFileRead   = "file_read"
	toolFileList   = "file_list"
	toolFileEdit   = "file_edit"
	toolFileCreate = "file_create"
	toolFileDelete = "file_delete"
)

// FileReadParams contains file_read arguments.
// FileReadParams 包含 file_read 参数。
type FileReadParams struct {
	Path      string `json:"path" jsonschema:"description=Absolute or workspace-relative text file path, required"`
	StartLine int    `json:"start_line" jsonschema:"description=1-based first line to read; defaults to 1, optional"`
	MaxLines  int    `json:"max_lines" jsonschema:"description=Maximum lines to read; defaults to 200 and cannot exceed 1000, optional"`
}

// FileListParams contains file_list arguments.
// FileListParams 包含 file_list 参数。
type FileListParams struct {
	Path       string `json:"path" jsonschema:"description=Absolute or workspace-relative directory path, required"`
	MaxEntries int    `json:"max_entries" jsonschema:"description=Maximum direct children to return; defaults to 200 and cannot exceed 1000, optional"`
}

// FileEditParams contains file_edit arguments.
// FileEditParams 包含 file_edit 参数。
type FileEditParams struct {
	Path    string `json:"path" jsonschema:"description=Absolute or workspace-relative existing text file path, required"`
	OldText string `json:"old_text" jsonschema:"description=Exact non-empty text that must appear once, required"`
	NewText string `json:"new_text" jsonschema:"description=Replacement text, required"`
}

// FileCreateParams contains file_create arguments.
// FileCreateParams 包含 file_create 参数。
type FileCreateParams struct {
	Path    string `json:"path" jsonschema:"description=Absolute or workspace-relative new text file path, required"`
	Content string `json:"content" jsonschema:"description=UTF-8 text content for the new file, required"`
}

// FileDeleteParams contains file_delete arguments.
// FileDeleteParams 包含 file_delete 参数。
type FileDeleteParams struct {
	Path string `json:"path" jsonschema:"description=Absolute or workspace-relative regular file path, required"`
}

type toolSet struct {
	guard   *securitycore.WorkspaceGuard
	service *execfiles.Service
}

// NewTools returns guarded filesystem tools.
// NewTools 返回受保护的文件系统工具。
func NewTools(guard *securitycore.WorkspaceGuard) ([]tool.BaseTool, error) {
	if guard == nil {
		return nil, errors.New("workspace guard is required")
	}
	set := &toolSet{
		guard:   guard,
		service: execfiles.NewService(),
	}
	readTool, err := utils.InferTool(toolFileRead, "Read a bounded UTF-8 line range from a workspace file", set.read)
	if err != nil {
		return nil, fmt.Errorf("infer %s tool: %w", toolFileRead, err)
	}
	listTool, err := utils.InferTool(toolFileList, "List direct children of a workspace directory without recursion", set.list)
	if err != nil {
		return nil, fmt.Errorf("infer %s tool: %w", toolFileList, err)
	}
	editTool, err := utils.InferTool(toolFileEdit, "Edit a workspace text file by one unique exact text replacement", set.edit)
	if err != nil {
		return nil, fmt.Errorf("infer %s tool: %w", toolFileEdit, err)
	}
	createTool, err := utils.InferTool(toolFileCreate, "Create a new UTF-8 workspace text file", set.create)
	if err != nil {
		return nil, fmt.Errorf("infer %s tool: %w", toolFileCreate, err)
	}
	deleteTool, err := utils.InferTool(toolFileDelete, "Delete one regular workspace file", set.delete)
	if err != nil {
		return nil, fmt.Errorf("infer %s tool: %w", toolFileDelete, err)
	}
	return []tool.BaseTool{readTool, listTool, editTool, createTool, deleteTool}, nil
}

// CapabilityDescriptors returns security descriptors for filesystem tools.
// CapabilityDescriptors 返回文件系统工具的安全 descriptor。
func CapabilityDescriptors() []capability.Descriptor {
	return []capability.Descriptor{
		{
			Name:          toolFileRead,
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
			Scopes:        []string{securitycore.ScopeFileRead},
		},
		{
			Name:          toolFileList,
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
			Scopes:        []string{securitycore.ScopeFileList},
		},
		{
			Name:          toolFileEdit,
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskMedium,
			DefaultPolicy: capability.DefaultPolicyReview,
			Scopes:        []string{securitycore.ScopeFileWrite},
		},
		{
			Name:          toolFileCreate,
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskMedium,
			DefaultPolicy: capability.DefaultPolicyReview,
			Scopes:        []string{securitycore.ScopeFileWrite},
		},
		{
			Name:          toolFileDelete,
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskHigh,
			DefaultPolicy: capability.DefaultPolicyReview,
			Scopes:        []string{securitycore.ScopeFileDelete},
		},
	}
}

// OperationBuilders returns security operation builders for filesystem tools.
// OperationBuilders 返回文件系统工具的安全 operation builder。
func OperationBuilders() map[string]securitycore.OperationBuilder {
	return map[string]securitycore.OperationBuilder{
		toolFileRead:   readOperationBuilder(),
		toolFileList:   listOperationBuilder(),
		toolFileEdit:   editOperationBuilder(),
		toolFileCreate: createOperationBuilder(),
		toolFileDelete: deleteOperationBuilder(),
	}
}

func (s *toolSet) read(ctx context.Context, req *FileReadParams) (*execfiles.ReadResult, error) {
	if req == nil {
		return nil, errors.New("file_read request is nil")
	}
	path, err := execguard.RequireAuthorizedPath(ctx, s.guard, req.Path)
	if err != nil {
		return nil, err
	}
	result, err := s.service.ReadText(ctx, execfiles.ReadRequest{
		Path:      path,
		StartLine: req.StartLine,
		MaxLines:  req.MaxLines,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *toolSet) list(ctx context.Context, req *FileListParams) (*execfiles.ListResult, error) {
	if req == nil {
		return nil, errors.New("file_list request is nil")
	}
	path, err := execguard.RequireAuthorizedPath(ctx, s.guard, req.Path)
	if err != nil {
		return nil, err
	}
	result, err := s.service.ListDirectory(ctx, execfiles.ListRequest{
		Path:       path,
		MaxEntries: req.MaxEntries,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *toolSet) edit(ctx context.Context, req *FileEditParams) (*execfiles.EditResult, error) {
	if req == nil {
		return nil, errors.New("file_edit request is nil")
	}
	path, err := execguard.RequireAuthorizedPath(ctx, s.guard, req.Path)
	if err != nil {
		return nil, err
	}
	result, err := s.service.EditText(ctx, execfiles.EditRequest{
		Path:    path,
		OldText: req.OldText,
		NewText: req.NewText,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *toolSet) create(ctx context.Context, req *FileCreateParams) (*execfiles.CreateResult, error) {
	if req == nil {
		return nil, errors.New("file_create request is nil")
	}
	path, err := execguard.RequireAuthorizedPath(ctx, s.guard, req.Path)
	if err != nil {
		return nil, err
	}
	result, err := s.service.CreateText(ctx, execfiles.CreateRequest{
		Path:    path,
		Content: req.Content,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *toolSet) delete(ctx context.Context, req *FileDeleteParams) (*execfiles.DeleteResult, error) {
	if req == nil {
		return nil, errors.New("file_delete request is nil")
	}
	path, err := execguard.RequireAuthorizedPath(ctx, s.guard, req.Path)
	if err != nil {
		return nil, err
	}
	result, err := s.service.DeleteFile(ctx, execfiles.DeleteRequest{Path: path})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func readOperationBuilder() securitycore.OperationBuilder {
	return func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) securitycore.OperationRequest {
		var params FileReadParams
		_ = json.Unmarshal([]byte(input.RawJSON), &params)
		request.OperationKind = securitycore.OperationFileRead
		request.Resources = fileResources(securitycore.ResourceKindFile, params.Path)
		request.SanitizedArgsSummary = fmt.Sprintf("{path=%s,start_line=%d,max_lines=%d}", sanitizePath(params.Path), params.StartLine, params.MaxLines)
		return request
	}
}

func listOperationBuilder() securitycore.OperationBuilder {
	return func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) securitycore.OperationRequest {
		var params FileListParams
		_ = json.Unmarshal([]byte(input.RawJSON), &params)
		request.OperationKind = securitycore.OperationFileList
		request.Resources = fileResources(securitycore.ResourceKindPath, params.Path)
		request.SanitizedArgsSummary = fmt.Sprintf("{path=%s,max_entries=%d}", sanitizePath(params.Path), params.MaxEntries)
		return request
	}
}

func editOperationBuilder() securitycore.OperationBuilder {
	return func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) securitycore.OperationRequest {
		var params FileEditParams
		_ = json.Unmarshal([]byte(input.RawJSON), &params)
		request.OperationKind = securitycore.OperationFileWrite
		request.Resources = fileResources(securitycore.ResourceKindFile, params.Path)
		request.SanitizedArgsSummary = fmt.Sprintf(
			"{path=%s,old_bytes=%d,new_bytes=%d,old_sha256=%s,new_sha256=%s}",
			sanitizePath(params.Path),
			len(params.OldText),
			len(params.NewText),
			sha256Text(params.OldText),
			sha256Text(params.NewText),
		)
		return request
	}
}

func createOperationBuilder() securitycore.OperationBuilder {
	return func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) securitycore.OperationRequest {
		var params FileCreateParams
		_ = json.Unmarshal([]byte(input.RawJSON), &params)
		request.OperationKind = securitycore.OperationFileWrite
		request.Resources = fileResources(securitycore.ResourceKindFile, params.Path)
		request.SanitizedArgsSummary = fmt.Sprintf(
			"{path=%s,content_bytes=%d,content_sha256=%s}",
			sanitizePath(params.Path),
			len(params.Content),
			sha256Text(params.Content),
		)
		return request
	}
}

func deleteOperationBuilder() securitycore.OperationBuilder {
	return func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) securitycore.OperationRequest {
		var params FileDeleteParams
		_ = json.Unmarshal([]byte(input.RawJSON), &params)
		request.OperationKind = securitycore.OperationFileDelete
		request.Resources = fileResources(securitycore.ResourceKindFile, params.Path)
		request.SanitizedArgsSummary = fmt.Sprintf("{path=%s}", sanitizePath(params.Path))
		return request
	}
}

func fileResources(kind, path string) []securitycore.OperationResource {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return []securitycore.OperationResource{{Kind: kind, Value: path}}
}

func sanitizePath(path string) string {
	return securitycore.SanitizeText(strings.TrimSpace(path))
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
