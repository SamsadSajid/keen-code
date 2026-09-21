package filesystem

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CheckAutoPath returns whether auto mode may handle one file path.
// It never changes the policy used outside auto mode.
func (g *Guard) CheckAutoPath(path, operation string) Permission {
	if g.CheckPath(path, operation) == PermissionDenied {
		return PermissionDenied
	}

	resolved, err := g.ResolvePath(path)
	if err != nil {
		return PermissionPending
	}
	if !g.IsInWorkingDir(resolved) || g.IsMemoryPath(resolved) || g.IsInMemoryDir(resolved) {
		return PermissionPending
	}

	existing, exists, err := nearestExistingPath(resolved)
	if err != nil || hasAutoSymlinkComponent(g.workingDir, existing) {
		return PermissionPending
	}
	if !exists && operation == "read" {
		return PermissionPending
	}
	if isAutoSensitivePath(resolved, g.workingDir, operation) {
		return PermissionPending
	}

	return PermissionGranted
}

// nearestExistingPath finds the target or its nearest existing parent.
func nearestExistingPath(path string) (string, bool, error) {
	current := filepath.Clean(path)
	for {
		_, err := os.Lstat(current)
		if err == nil {
			return current, current == filepath.Clean(path), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false, err
		}
		next := filepath.Dir(current)
		if next == current {
			return "", false, err
		}
		current = next
	}
}

// hasAutoSymlinkComponent reports links from the workspace to path.
func hasAutoSymlinkComponent(workingDir, path string) bool {
	workingDir = filepath.Clean(workingDir)
	relative, err := filepath.Rel(workingDir, filepath.Clean(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	current := workingDir
	if info, err := os.Lstat(current); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part != "." {
			current = filepath.Join(current, part)
		}
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func isAutoSensitivePath(path, workingDir, operation string) bool {
	relative, err := filepath.Rel(filepath.Clean(workingDir), filepath.Clean(path))
	if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return true
	}
	if relative == "." {
		return false
	}

	parts := strings.Split(filepath.ToSlash(relative), "/")
	base := parts[len(parts)-1]
	if isAutoSensitiveBaseName(base) {
		return true
	}
	for _, part := range parts {
		if isAutoSensitiveDirectory(part) {
			return true
		}
	}
	if operation == "write" || operation == "edit" {
		if isAutoInstructionName(base) {
			return true
		}
		for _, part := range parts {
			if part == ".agents" || part == ".claude" || part == ".keen" {
				return true
			}
		}
	}
	return false
}

func isAutoSensitiveDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".aws", ".azure", ".docker", ".git", ".gnupg", ".kube", ".ssh":
		return true
	default:
		return false
	}
}

func isAutoInstructionName(name string) bool {
	switch strings.ToLower(name) {
	case "agents.md", "claude.md", "instructions.md":
		return true
	default:
		return false
	}
}

func isAutoSensitiveBaseName(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	if lower == ".netrc" || lower == ".npmrc" || lower == ".pypirc" || lower == ".git-credentials" || lower == "authorized_keys" {
		return true
	}
	if strings.Contains(lower, "credential") || strings.Contains(lower, "secret") {
		return true
	}
	for _, suffix := range []string{".key", ".pem", ".p12", ".pfx"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return strings.HasPrefix(lower, "id_")
}
