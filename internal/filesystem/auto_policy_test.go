package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuard_CheckAutoPath_OrdinaryLocalFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	g := NewGuard(root, nil)
	for _, test := range []struct {
		path      string
		operation string
	}{
		{"main.go", "read"},
		{"main.go", "write"},
		{"new/file.go", "write"},
	} {
		if got := g.CheckAutoPath(test.path, test.operation); got != PermissionGranted {
			t.Errorf("CheckAutoPath(%q, %q) = %v, want granted", test.path, test.operation, got)
		}
	}
}

func TestGuard_CheckAutoPath_RequiresManualForExternalAndSensitivePaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, ".env"),
		filepath.Join(root, ".env.local"),
		filepath.Join(root, "server.pem"),
		filepath.Join(root, "credentials.json"),
		filepath.Join(root, ".git", "config"),
		filepath.Join(root, ".git", "hooks", "pre-commit"),
		filepath.Join(root, ".aws", "config"),
		filepath.Join(root, ".ssh", "config"),
		filepath.Join(root, "AGENTS.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("value"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	g := NewGuard(root, nil)
	if got := g.CheckAutoPath("AGENTS.md", "read"); got != PermissionGranted {
		t.Errorf("CheckAutoPath(AGENTS.md, read) = %v, want granted", got)
	}
	for _, test := range []struct {
		path      string
		operation string
	}{
		{filepath.Join(outside, "file.txt"), "write"},
		{".env", "read"},
		{".env.local", "write"},
		{"server.pem", "read"},
		{"credentials.json", "read"},
		{".git/config", "read"},
		{".git/hooks/pre-commit", "write"},
		{".git", "read"},
		{".aws", "read"},
		{".aws/config", "read"},
		{".ssh/config", "write"},
		{"AGENTS.md", "write"},
		{".agents/config.json", "write"},
	} {
		if got := g.CheckAutoPath(test.path, test.operation); got != PermissionPending {
			t.Errorf("CheckAutoPath(%q, %q) = %v, want pending", test.path, test.operation, got)
		}
	}
}

func TestGuard_CheckAutoPath_RequiresManualForSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "target.txt"), []byte("value"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "target.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	g := NewGuard(root, nil)
	for _, path := range []string{"link.txt", "linked-dir/new.txt"} {
		if got := g.CheckAutoPath(path, "write"); got != PermissionPending {
			t.Errorf("CheckAutoPath(%q, write) = %v, want pending", path, got)
		}
	}
}

func TestGuard_CheckAutoPath_RequiresManualForSymlinkWorkspace(t *testing.T) {
	target := t.TempDir()
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	g := NewGuard(root, nil)
	if got := g.CheckAutoPath("main.go", "read"); got != PermissionPending {
		t.Errorf("CheckAutoPath(main.go, read) = %v, want pending", got)
	}
}

func TestGuard_CheckAutoPath_HardDenialWins(t *testing.T) {
	root := t.TempDir()
	ignored := filepath.Join(root, "ignored.txt")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignored, []byte("value"), 0600); err != nil {
		t.Fatal(err)
	}
	git := NewGitAwareness()
	if err := git.LoadGitignore(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	g := NewGuard(root, git)
	if got := g.CheckAutoPath("ignored.txt", "read"); got != PermissionDenied {
		t.Errorf("CheckAutoPath(ignored.txt, read) = %v, want denied", got)
	}
}
