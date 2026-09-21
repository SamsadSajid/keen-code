package subagents

import (
	"context"

	"github.com/mochow13/keen-code/internal/filesystem"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/tools"
)

type AutoApprover struct{ Parent tools.PermissionRequester }

func (a AutoApprover) RequestPermission(ctx context.Context, tool, path, resolved string, dangerous bool) (bool, error) {
	if state, ok := a.Parent.(interface{ AutoModeEnabled() bool }); ok && state.AutoModeEnabled() {
		return a.Parent.RequestPermission(ctx, tool, path, resolved, dangerous)
	}
	return true, nil
}

func (a AutoApprover) AutoModeEnabled() bool {
	state, ok := a.Parent.(interface{ AutoModeEnabled() bool })
	return ok && state.AutoModeEnabled()
}

func (a AutoApprover) RequestManualPermission(ctx context.Context, tool, path, resolved string, dangerous bool) (bool, error) {
	if manual, ok := a.Parent.(tools.ManualPermissionRequester); ok {
		return manual.RequestManualPermission(ctx, tool, path, resolved, dangerous)
	}
	return a.RequestPermission(ctx, tool, path, resolved, dangerous)
}

func (a AutoApprover) ReviewOperation(ctx context.Context, operation tools.Operation) (tools.OperationReviewDecision, error) {
	if reviewer, ok := a.Parent.(tools.OperationReviewer); ok {
		return reviewer.ReviewOperation(ctx, operation)
	}
	return tools.OperationReviewNotRequired, nil
}

type NoopDiffEmitter struct{}

func (NoopDiffEmitter) EmitDiff([]tools.EditDiffLine) {}

type ToolFactory struct {
	Guard           *filesystem.Guard
	MCPRuntime      keenmcp.Runtime
	ParentRequester tools.PermissionRequester
}

func (f ToolFactory) Registry(profile Profile, parent *tools.Registry) *tools.Registry {
	registry := tools.NewRegistry()
	if f.Guard == nil {
		return registry
	}
	approver := AutoApprover{Parent: f.ParentRequester}
	available := map[string]tools.Tool{
		tools.ReadFileToolName:  tools.NewReadFileTool(f.Guard, approver),
		tools.GlobToolName:      tools.NewGlobTool(f.Guard, approver),
		tools.GrepToolName:      tools.NewGrepTool(f.Guard, approver),
		tools.WriteFileToolName: tools.NewWriteFileTool(f.Guard, NoopDiffEmitter{}, approver),
		tools.EditFileToolName:  tools.NewEditFileTool(f.Guard, NoopDiffEmitter{}, approver),
		tools.BashToolName:      tools.NewBashTool(f.Guard, approver),
		tools.WebFetchToolName:  tools.NewWebFetchTool(),
	}
	for _, name := range permissionToolNames(profile, registryNames(parent)) {
		if tool, ok := available[name]; ok {
			_ = registry.Register(tool)
		}
	}
	if f.MCPRuntime != nil {
		_ = registry.Register(tools.NewCallMCPTool(f.MCPRuntime, approver))
	}
	return registry
}

func registryNames(registry *tools.Registry) []string {
	if registry == nil {
		return nil
	}
	all := registry.All()
	names := make([]string, 0, len(all))
	for _, tool := range all {
		names = append(names, tool.Name())
	}
	return names
}
