package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mochow13/keen-code/internal/filesystem"
)

type safetyReviewRequester struct {
	auto          bool
	decision      OperationReviewDecision
	reviewErr     error
	manualAllowed bool
	onReview      func()
	reviews       int
	manualPrompts int
	operations    []Operation
}

func (r *safetyReviewRequester) AutoModeEnabled() bool { return r.auto }

func (r *safetyReviewRequester) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	r.manualPrompts++
	return r.manualAllowed, nil
}

func (r *safetyReviewRequester) RequestManualPermission(ctx context.Context, tool, path, resolved string, dangerous bool) (bool, error) {
	return r.RequestPermission(ctx, tool, path, resolved, dangerous)
}

func (r *safetyReviewRequester) ReviewOperation(_ context.Context, operation Operation) (OperationReviewDecision, error) {
	r.reviews++
	r.operations = append(r.operations, operation)
	if r.onReview != nil {
		r.onReview()
	}
	return r.decision, r.reviewErr
}

type safetyDiffCapture struct {
	lines []EditDiffLine
}

func (d *safetyDiffCapture) EmitDiff(lines []EditDiffLine) {
	d.lines = append([]EditDiffLine(nil), lines...)
}

type safetyBuildRequester struct {
	prompts int
}

func (r *safetyBuildRequester) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	r.prompts++
	return true, nil
}

func TestAutoSafetyReviewerFallbackPreventsEffects(t *testing.T) {
	root := t.TempDir()
	guard := filesystem.NewGuard(root, nil)
	for _, test := range []struct {
		name      string
		reviewErr error
		run       func(*safetyReviewRequester) error
		assert    func(*testing.T)
	}{
		{
			name: "denied write",
			run: func(requester *safetyReviewRequester) error {
				_, err := NewWriteFileTool(guard, nil, requester).Execute(context.Background(), map[string]any{
					"path": "denied.txt", "content": "must not exist",
				})
				return err
			},
			assert: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(root, "denied.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("denied write changed file: %v", err)
				}
			},
		},
		{
			name: "denied bash",
			run: func(requester *safetyReviewRequester) error {
				_, err := NewBashTool(guard, requester).Execute(context.Background(), map[string]any{
					"command": "printf marker > bash-marker.txt",
				})
				return err
			},
			assert: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(root, "bash-marker.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("denied bash changed file: %v", err)
				}
			},
		},
		{
			name:      "provider error",
			reviewErr: errors.New("provider unavailable"),
			run: func(requester *safetyReviewRequester) error {
				_, err := NewWriteFileTool(guard, nil, requester).Execute(context.Background(), map[string]any{
					"path": "provider-error.txt", "content": "must not exist",
				})
				return err
			},
			assert: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(root, "provider-error.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("provider error changed file: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requester := &safetyReviewRequester{
				auto:          true,
				decision:      OperationReviewAskUser,
				reviewErr:     test.reviewErr,
				manualAllowed: false,
			}
			if err := test.run(requester); err == nil {
				t.Fatal("operation succeeded after reviewer fallback")
			}
			if requester.reviews != 1 || requester.manualPrompts != 1 {
				t.Fatalf("reviews=%d manual prompts=%d", requester.reviews, requester.manualPrompts)
			}
			test.assert(t)
		})
	}
}

func TestAutoSafetyCancellationPreventsApprovedWrite(t *testing.T) {
	root := t.TempDir()
	guard := filesystem.NewGuard(root, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requester := &safetyReviewRequester{auto: true, decision: OperationReviewApproved}
	requester.onReview = cancel

	_, err := NewWriteFileTool(guard, nil, requester).Execute(ctx, map[string]any{
		"path": "cancelled.txt", "content": "must not exist",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("write error = %v, want context cancellation", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cancelled.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled write changed file: %v", err)
	}
}

func TestAutoSafetyBashReviewErrorCannotGrantApproval(t *testing.T) {
	root := t.TempDir()
	requester := &safetyReviewRequester{
		auto: true, decision: OperationReviewApproved,
		reviewErr: errors.New("review failed after a partial result"),
	}
	_, err := NewBashTool(filesystem.NewGuard(root, nil), requester).Execute(context.Background(), map[string]any{
		"command": "printf marker > must-not-exist.txt",
	})
	if err == nil || requester.manualPrompts != 1 {
		t.Fatal("review error did not require manual approval")
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-exist.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed review allowed a shell effect: %v", err)
	}
}

func TestAutoSafetySymlinkSwapPreventsExternalWrite(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	insidePath := filepath.Join(root, "target.txt")
	outsidePath := filepath.Join(outside, "target.txt")
	if err := os.WriteFile(insidePath, []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	requester := &safetyReviewRequester{auto: true, decision: OperationReviewApproved}
	requester.onReview = func() {
		if err := os.Remove(insidePath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsidePath, insidePath); err != nil {
			t.Fatal(err)
		}
	}

	_, err := NewWriteFileTool(filesystem.NewGuard(root, nil), nil, requester).Execute(context.Background(), map[string]any{
		"path": "target.txt", "content": "reviewed content",
	})
	if err == nil {
		t.Fatal("symlink swap write succeeded")
	}
	content, err := os.ReadFile(outsidePath)
	if err != nil || string(content) != "outside" {
		t.Fatalf("outside target changed: %q, %v", content, err)
	}
}

func TestAutoSafetyEditReviewsExactFinalDiff(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	oldContent := "one\ntwo\nthree\n"
	if err := os.WriteFile(path, []byte(oldContent), 0600); err != nil {
		t.Fatal(err)
	}
	requester := &safetyReviewRequester{auto: true, decision: OperationReviewApproved}
	diffCapture := &safetyDiffCapture{}
	ops := []any{map[string]any{
		"start": anchorForLine(t, oldContent, 2),
		"text":  "TWO",
	}}

	_, err := NewEditFileTool(filesystem.NewGuard(root, nil), diffCapture, requester).Execute(context.Background(), map[string]any{
		"path": "main.txt", "ops": ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	finalContent := "one\nTWO\nthree\n"
	content, err := os.ReadFile(path)
	if err != nil || string(content) != finalContent {
		t.Fatalf("edited content = %q, %v", content, err)
	}
	if requester.reviews != 1 || len(requester.operations) != 1 {
		t.Fatalf("review count = %d, operations = %#v", requester.reviews, requester.operations)
	}
	expectedDiff := computeEditDiff(oldContent, finalContent)
	if requester.operations[0].Content != formatEditDiff(expectedDiff) {
		t.Fatalf("reviewed diff = %q, want %q", requester.operations[0].Content, formatEditDiff(expectedDiff))
	}
	if requester.operations[0].Bytes != len(finalContent) {
		t.Fatalf("reviewed byte count = %d, want %d", requester.operations[0].Bytes, len(finalContent))
	}
	if len(diffCapture.lines) == 0 || formatEditDiff(diffCapture.lines) != formatEditDiff(expectedDiff) {
		t.Fatal("emitted diff does not match reviewed final change")
	}
}

func TestAutoSafetyHardGuardDenialSkipsReviewer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ignored.txt")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	git := filesystem.NewGitAwareness()
	if err := git.LoadGitignore(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	requester := &safetyReviewRequester{auto: true, decision: OperationReviewApproved}
	_, err := NewWriteFileTool(filesystem.NewGuard(root, git), nil, requester).Execute(context.Background(), map[string]any{
		"path": "ignored.txt", "content": "changed",
	})
	if err == nil {
		t.Fatal("hard-denied write succeeded")
	}
	if requester.reviews != 0 || requester.manualPrompts != 0 {
		t.Fatalf("hard denial reached permission flow: %+v", requester)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "original" {
		t.Fatalf("hard-denied write changed file: %q, %v", content, readErr)
	}
}

func TestAutoSafetyBuildCompatibility(t *testing.T) {
	root := t.TempDir()
	requester := &safetyBuildRequester{}
	guard := filesystem.NewGuard(root, nil)
	if _, err := NewWriteFileTool(guard, nil, requester).Execute(context.Background(), map[string]any{
		"path": "build.txt", "content": "build mode",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBashTool(guard, requester).Execute(context.Background(), map[string]any{
		"command": "printf marker > build-marker.txt",
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build.txt", "build-marker.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("build compatibility did not create %s: %v", name, err)
		}
	}
	if requester.prompts != 1 {
		t.Fatalf("build prompts = %d, want 1 for the write", requester.prompts)
	}
}
