package subagents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/filesystem"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/tools"
)

type deniedChildRuntime struct {
	keenmcp.Runtime
	calls int
}

func (r *deniedChildRuntime) CallTool(context.Context, string, string, map[string]any) (*keenmcp.ToolResult, error) {
	r.calls++
	return &keenmcp.ToolResult{}, nil
}

func TestChildAutoPolicyRequiresManualApprovalForRiskyOperations(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	secretPath := filepath.Join(root, ".env")
	if err := os.WriteFile(secretPath, []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	parent := tools.NewRegistry()
	for _, name := range []string{tools.ReadFileToolName, tools.WriteFileToolName, tools.BashToolName} {
		if err := parent.Register(namedTool{name: name}); err != nil {
			t.Fatal(err)
		}
	}
	requester := &childAutoRequester{}
	runtime := &deniedChildRuntime{}
	factory := ToolFactory{Guard: filesystem.NewGuard(root, nil), ParentRequester: requester, MCPRuntime: runtime}
	child := factory.Registry(Profile{}, parent)
	tests := []struct {
		name  string
		tool  string
		input map[string]any
	}{
		{"sensitive read", tools.ReadFileToolName, map[string]any{"path": ".env"}},
		{"external write", tools.WriteFileToolName, map[string]any{"path": filepath.Join(external, "new.txt"), "content": "new"}},
		{"dangerous shell", tools.BashToolName, map[string]any{"command": "rm keep.txt"}},
		{"MCP effect", tools.CallMCPToolName, map[string]any{"server": "fixture", "tool": "delete", "arguments": map[string]any{"id": "private"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := child.Get(tt.tool)
			if !ok {
				t.Fatalf("missing child tool %s", tt.tool)
			}
			before := requester.prompts
			if result, err := tool.Execute(context.Background(), tt.input); err == nil || result != nil {
				t.Fatalf("denied operation returned %v, %v", result, err)
			}
			if requester.prompts != before+1 || requester.reviews != 0 {
				t.Fatal("child bypassed manual policy or sent restricted data for model review")
			}
		})
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("denied shell changed its target: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(external, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied child write created a file: %v", err)
	}
	if runtime.calls != 0 {
		t.Fatal("denied child MCP call reached the server")
	}
}

func TestChildAutoBashUsesReviewedWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	parent := tools.NewRegistry()
	if err := parent.Register(namedTool{name: tools.BashToolName}); err != nil {
		t.Fatal(err)
	}
	requester := &childAutoRequester{}
	factory := ToolFactory{Guard: filesystem.NewGuard(root, nil), ParentRequester: requester}
	tool, _ := factory.Registry(Profile{}, parent).Get(tools.BashToolName)
	result, err := tool.Execute(context.Background(), map[string]any{"command": "pwd"})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(result.(map[string]any)["stdout"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("child command ran in %q, want %q", got, want)
	}
	if requester.reviews != 1 || requester.prompts != 0 {
		t.Fatal("ordinary child command did not use auto review")
	}
}
