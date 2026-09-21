package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mochow13/keen-code/internal/filesystem"
)

type autoReviewRequester struct {
	operation Operation
	reviews   int
	prompts   int
	decision  OperationReviewDecision
}

func (r *autoReviewRequester) AutoModeEnabled() bool { return true }
func (r *autoReviewRequester) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	r.prompts++
	return true, nil
}
func (r *autoReviewRequester) ReviewOperation(_ context.Context, operation Operation) (OperationReviewDecision, error) {
	r.operation = operation
	r.reviews++
	return r.decision, nil
}

func TestAutoReviewApprovesOrdinaryBashAndWrite(t *testing.T) {
	root := t.TempDir()
	guard := filesystem.NewGuard(root, nil)
	requester := &autoReviewRequester{decision: OperationReviewApproved}
	if _, err := NewBashTool(guard, requester).Execute(context.Background(), map[string]any{"command": "true"}); err != nil {
		t.Fatal(err)
	}
	if requester.reviews != 1 || requester.operation.Command != "true" || requester.prompts != 0 {
		t.Fatalf("bash review = %+v", requester)
	}
	if _, err := NewWriteFileTool(guard, nil, requester).Execute(context.Background(), map[string]any{"path": "main.go", "content": "package main\n"}); err != nil {
		t.Fatal(err)
	}
	if requester.reviews != 2 || requester.operation.Path != filepath.Join(root, "main.go") || requester.operation.Content == "" {
		t.Fatalf("write review = %+v", requester)
	}
}

func TestAutoReviewKeepsExternalWriteManual(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	requester := &autoReviewRequester{decision: OperationReviewApproved}
	tool := NewWriteFileTool(filesystem.NewGuard(root, nil), nil, requester)
	path := filepath.Join(outside, "outside.txt")
	if _, err := tool.Execute(context.Background(), map[string]any{"path": path, "content": "value"}); err != nil {
		t.Fatal(err)
	}
	if requester.reviews != 0 || requester.prompts != 1 {
		t.Fatalf("external write = %+v", requester)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestAutoSearchSkipsSensitiveFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=value"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("TOKEN=value"), 0600); err != nil {
		t.Fatal(err)
	}
	requester := &autoReviewRequester{decision: OperationReviewApproved}
	guard := filesystem.NewGuard(root, nil)
	globResult, err := NewGlobTool(guard, requester).Execute(context.Background(), map[string]any{"pattern": "*"})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range globResult.(map[string]any)["files"].([]string) {
		if filepath.Base(file) == ".env" {
			t.Fatal("glob exposed .env")
		}
	}
	grepResult, err := NewGrepTool(guard, requester).Execute(context.Background(), map[string]any{"pattern": "TOKEN", "output_mode": "file"})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range grepResult.(map[string]any)["files"].([]string) {
		if filepath.Base(file) == ".env" {
			t.Fatal("grep exposed .env")
		}
	}
}

func TestAutoSearchRunsAfterManualExternalApproval(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	file := filepath.Join(outside, "external.go")
	if err := os.WriteFile(file, []byte("package external"), 0600); err != nil {
		t.Fatal(err)
	}
	requester := &autoReviewRequester{decision: OperationReviewApproved}
	result, err := NewGlobTool(filesystem.NewGuard(root, nil), requester).Execute(context.Background(), map[string]any{"path": outside, "pattern": "*"})
	if err != nil {
		t.Fatal(err)
	}
	files := result.(map[string]any)["files"].([]string)
	if len(files) != 1 || files[0] != file || requester.prompts != 1 {
		t.Fatalf("external glob = %#v, prompts %d", files, requester.prompts)
	}
}
