# Permission System

The permission system in Keen Code uses a `Guard` to enforce filesystem access policies, with user approval for sensitive operations.

## Guard

The `Guard` type in `internal/filesystem/guard.go` is the central component:

```go
type Guard struct {
    workingDir   string
    blockedPaths []string
    gitignore    *GitAwareness
}

func NewGuard(workingDir string, gitignore *GitAwareness) *Guard
```

## Permission States

```go
type Permission int

const (
    PermissionDenied  Permission = iota  // Blocked by policy
    PermissionGranted                     // Allowed without prompting
    PermissionPending                     // Needs user approval
)
```

## Path Resolution

Paths are resolved relative to the working directory:

```go
func (g *Guard) ResolvePath(path string) (string, error) {
    if filepath.IsAbs(path) {
        return filepath.Clean(path), nil
    }
    return filepath.Join(g.workingDir, path), nil
}
```

## Policy Checks

`CheckPath` evaluates an operation against the policy:

```go
func (g *Guard) CheckPath(path string, operation string) Permission
```

### Operation: "read"
- **Granted**: Path is in the working directory, in a skills directory, or under `~/.keen/bash/`
- **Pending**: Path is outside working directory but not blocked
- **Denied**: Path is blocked by policy or does not resolve

### Operation: "write" or "edit"
- **Pending**: Always requires user approval (no auto-grant for writes)
- **Denied**: Path is blocked by policy

### Other Operations
- **Denied**: Any unrecognized operation is blocked

## Blocked Paths

System directories are blocked by default:

```go
func defaultBlockedPaths() []string {
    return []string{
        "/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64",
        "/proc", "/sys", "/dev", "/root",
    }
}
```

## GitAwareness

The `GitAwareness` type (`internal/filesystem/gitawareness.go`) loads `.gitignore` files to excludeIgnored paths from tool access:

```go
type GitAwareness struct {
    patternSets []PatternSet
}

func (g *GitAwareness) LoadGitignoreRecursive(root string) error
func (g *GitAwareness) IsIgnored(filePath string) bool
func (g *GitAwareness) FilterPaths(paths []string) []string
```

It uses the `go-git` library's gitignore parser to handle `.gitignore` patterns correctly.

## Blocked Path Checks

A path is blocked if:
1. It cannot be resolved
2. It matches a `.gitignore` pattern
3. It is in a hidden directory under home (`~/.something`)
4. It has a prefix in `blockedPaths` (system directories)
5. It is in a skill directory (exception - always allowed for reads)
6. It is under `~/.keen/bash/` (exception - always allowed for reads)

```go
func (g *Guard) IsBlocked(path string) bool {
    resolved, err := g.ResolvePath(path)
    if err != nil {
        return true
    }
    if g.gitignore != nil && g.gitignore.IsIgnored(path) {
        return true
    }
    if g.IsInSkillDir(resolved) || g.IsInKeenBashDir(resolved) {
        return false
    }
    // ... home and system path checks
}
```

## Skills Directory Exception

Skill directories are explicitly allowed for read access:
- `~/.agents/skills`
- `~/.keen/skills`
- `~/.claude/skills`

```go
func (g *Guard) IsInSkillDir(path string) bool {
    home, _ := os.UserHomeDir()
    for _, dir := range []string{
        filepath.Join(home, ".agents", "skills"),
        filepath.Join(home, ".keen", "skills"),
        filepath.Join(home, ".claude", "skills"),
    } {
        if strings.HasPrefix(path, dir) {
            return true
        }
    }
    return false
}
```

## Bash Output Directory Exception

The `bash` tool stores oversized stdout and stderr in randomly named files under `~/.keen/bash/`. Read access is granted without an additional prompt for files under that directory. Symlink entries are not auto-granted. This exception is read-only; writes and edits to those paths still require normal approval.

Agents should use artifact paths returned by the `bash` result (`stdout_file` or `stderr_file`) and then inspect them with `read_file` line windows or targeted search.

## PermissionRequester Interface

Tools use `PermissionRequester` to request user approval:

```go
// internal/tools/permission.go
type PermissionRequester interface {
    RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error)
}
```

### Parameters:
- `toolName`: Name of the tool (e.g., "bash", "read_file")
- `path`: Original path as provided
- `resolvedPath`: Absolute resolved path
- `isDangerous`: True for operations that modify files/system

The requester implementation (typically in the CLI/repl layer) prompts the user and returns their decision.

## Tool Integration

All tools follow the same permission pattern:

```go
func (t *SomeTool) Execute(ctx context.Context, input any) (any, error) {
    // 1. Parse parameters
    // 2. Resolve path
    // 3. Check permission
    permission := t.guard.CheckPath(path, operation)

    switch permission {
    case PermissionDenied:
        return nil, fmt.Errorf("permission denied by policy")
    case PermissionPending:
        allowed, err := t.permissionRequester.RequestPermission(...)
        if !allowed {
            return nil, fmt.Errorf("permission denied by user")
        }
    }

    // 4. Execute operation
}
```

## Bash Tool Special Case

The `bash` tool has special handling for dangerous commands:

```go
// If command is marked dangerous, always prompt
if isDangerous {
    allowed, err := t.permissionRequester.RequestPermission(
        ctx, t.Name(), command, "", true,
    )
    // ...
}
```

Dangerous commands include:
- File removal (`rm`, `rm -rf`)
- Git operations that modify repository state
- Process termination
- System modifications

In build mode, non-dangerous Bash commands run when the working directory check passes. In auto mode, an approval subagent also reviews eligible commands before execution.

## Auto Mode

Use `/auto` or `/mode auto` to select auto mode. The approval subagent reviews
eligible shell commands and file changes after the filesystem guard check.
It has no tools and receives no conversation history. A model grant applies
to one operation. It does not create a session or project grant.

Sensitive paths, external paths, detected secrets, large changes, and MCP
calls need manual review. If the model cannot approve an operation, Keen
uses the manual prompt. A hard guard denial remains final.

Read [the usage guide](../docs.md) and [the architecture](../arch.md) for
the limits and trade-offs.

## Project-Level Allow List

Users can pre-allow specific tools for the current project via the `/allow-permission` command. Settings are stored in `.keen/permissions.json`:

```json
{
  "allow": ["bash"]
}
```

- Tools in `allow` skip the interactive prompt entirely (including the dangerous-command prompt for `bash`). The filesystem guard still applies: system directories, `.gitignore`d files, and dotfiles under `$HOME` remain blocked except for explicit read exceptions such as skills directories and `~/.keen/bash/`.
- Tools absent from `allow` follow the normal mechanism described above.

`/reset-permission <tool_names...>` removes tools from the allow list, restoring default behavior.

The lookup order inside `RequestPermission` is:

1. yolo mode (`/mode yolo`) → grant without prompting
2. `autoApprove` (headless mode) → grant
3. project `allow` list → grant
4. session-allowed tools (non-dangerous only) → grant
5. prompt the user

## Yolo Mode

Yolo mode (`/mode yolo`) sets a per-mode flag on the interactive `Requester` so `RequestPermission` returns `(true, nil)` immediately, before the project/session/prompt logic — skipping all interactive prompts, including the dangerous-`bash` prompt. It is separate from headless `autoApprove` (`NewAutoApproveRequester`): leaving yolo mode (`/mode build`) restores normal prompting. Yolo has the full build tool registry (no plan-style tool stripping). Like `/allow-permission`, yolo skips prompts only: a `Guard` `PermissionDenied` verdict (blocked system paths, `.gitignore`, home dotfiles) still denies the operation.
