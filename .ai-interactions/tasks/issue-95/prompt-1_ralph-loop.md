You are implementing `/yolo` approval-less mode for keen-code (issue #95).

Goal: a `yolo` agent mode where ALL permission prompts are skipped, even for dangerous `bash` commands.
Users activate it via `/yolo` command and via `shift+tab` cycling `build` → `plan` → `yolo`.
In `yolo` mode the REPL shows the same input text area as build/plan, but in red.

Issue: https://github.com/mochow13/keen-code/issues/95
> Right now, Keen at least requires approval for `bash` commands that are classified as risky.
> Using `/yolo` command, users can skip all permissions and let Keen fly.

Progress checklist: `.ai-interactions/tasks/issue-95/PROGRESS.md` (create it on the first
invocation if missing, using the Task list below; keep the same `- [ ]` / `- [x]` format).

Work autonomously, but complete **exactly one** unchecked task per invocation, in order.
Do not begin a later task until all prior tasks are checked off. Treat a task as incomplete
unless its stated implementation and test requirements are satisfied.

## Required workflow for this invocation

1. Read this prompt and the progress checklist (`PROGRESS.md` in the same directory; create it
   from the Task list below if it does not exist yet).
2. Inspect `git status --short`, the current implementation, and tests relevant to the next task.
3. Implement only that task. Make minimal, idiomatic changes that preserve current behavior
   outside the task.
4. Run `gofmt` on every modified Go file and run `go mod tidy` after the change.
5. Run focused tests for the changed package(s). If this completes the final task, also run
   `go test -race ./...`.
6. Update `PROGRESS.md` in the same directory: check off the completed task; add a concise note
   with changed files and tests run; record any blocker instead of claiming completion.
7. Inspect `git diff --check` and `git status --short` before responding.

## Codebase orientation (from investigation)

- Modes: `internal/llm/systemprompt.go:12-17` (`AgentMode`, `ModeBuild`/`ModePlan`),
  `Build()` at `systemprompt.go:88-122` appends `buildModePrompt` / `planModePrompt`.
- State: `internal/cli/repl/appstate/state.go:34` (default `ModeBuild`), `SetMode` at
  `:334-339` (coerces anything non-plan to build — must be extended), `Mode()` at `:341-346`,
  `EffectiveToolRegistry()` at `:323-328` (plan strips write/edit tools), `systemPromptMessage()`
  at `:185-190`.
- REPL model: `internal/cli/repl/repl_helpers.go:430-455` (`currentMode`, `setMode`, `toggleMode`
  plan↔build), `internal/cli/repl/repl.go:73,227,241,274` (permission requester wiring),
  `repl.go:869-885` (textarea `Focused.Prompt` switch + `renderInputArea` call).
- Input rendering: `renderInputArea` at `repl_helpers.go:595-635`, rule-style chain at `:604-615`,
  chip-style at `:629-633`; theme at `internal/cli/repl/theme/styles.go:131-136` (input rules),
  `:234-235` (`ModeBuildChipStyle`/`ModePlanChipStyle`), `:237-241` (`InputRulePlanStyle`,
  `PromptPlanStyle`).
- Keybinding: `internal/cli/repl/handlers.go:595-597` (`shift+tab` → `toggleMode()` only when no
  suggestion is visible).
- Slash commands: `internal/cli/repl/commands/commands.go:23` (`Mode = "/mode"`), `All` at `:47-73`,
  `Suggestions` at `:75-110`; dispatch at `internal/cli/repl/command_handlers.go:73-76`,
  `handleModeCommand` at `:419-437` (`/mode plan|build` only).
- Permissions: `internal/cli/repl/permissions/requester.go:37-43` (`Requester` struct),
  `:53-57` (`NewAutoApproveRequester` for headless), `:59-99` (`RequestPermission` order:
  autoApprove → project allow → session-allowed → prompt). Headless wiring at
  `internal/cli/repl/headless_run.go:87-89`. Tool wiring at
  `internal/cli/repl/tooling/tool_registry.go:20-57`.
- Bash: `internal/tools/bash.go:133-163` — workdir `CheckPath` prompt, then dangerous-command
  prompt (`isDangerous || IsDangerousCommand(command)`). This is the prompt `/yolo` must skip.
- Permission UI: `internal/cli/repl/stream_permission.go`, `handlers.go:869-915`
  (`handlePermissionKeyMsg`). Guard policy (`internal/filesystem/guard.go`,
  `docs/permission-system.md`) stays authoritative: yolo skips *prompts*, never `PermissionDenied`
  blocks (same boundary as `/allow-permission`).
- Help/tips text: `repl_helpers.go:34` (`` `Shift+Tab` swaps plan ↔ build ``),
  `:55` (`` `/mode build` exits plan-only mode ``), `internal/cli/repl/tips.go:7,26`,
  mode line at `command_handlers.go:428`.

## Tasks

- [ ] Task 1 — `ModeYolo` + system prompt (`internal/llm/systemprompt.go`)
  Add `ModeYolo AgentMode = "yolo"`, a `yoloModePrompt` (build-leaning, approval-less: agent may
  run any tool including dangerous bash without asking; keep the never-expose-secrets safety
  line), and route it in `Build()` (`yolo` → yolo prompt, `plan` → plan prompt, else build).
  Tests: extend `internal/llm/systemprompt_test.go` — yolo prompt contains yolo marker and does
  not contain the plan read-only restriction; existing build/plan assertions still pass.

- [ ] Task 2 — AppState mode handling (`internal/cli/repl/appstate/state.go`)
  `SetMode` accepts `ModeYolo` (only unknown values coerce to build); `Mode()` returns yolo when
  set; `EffectiveToolRegistry()` returns the full registry in yolo (same as build — plan still
  strips write/edit). Tests: extend `appstate/state_test.go` — set/get yolo round-trips,
  yolo registry includes write/edit/bash, plan still excludes write/edit.

- [ ] Task 3 — Permission bypass in the requester (`internal/cli/repl/permissions/requester.go`)
  Add a yolo flag (e.g. `yoloMode bool` + `SetYoloMode(bool)`) so `RequestPermission` returns
  `(true, nil)` immediately when set, before project/session/prompt logic. Do NOT touch the
  `autoApprove` (headless) path or the `Guard` policy path. Tests: extend
  `permissions/requester_test.go` — yolo requester allows dangerous + non-dangerous without a
  pending request; toggling yolo off restores prompting; existing autoApprove tests pass.

- [ ] Task 4 — Wire yolo into the REPL model
  (`internal/cli/repl/repl.go`, `repl_helpers.go`, `command_handlers.go:1162-1170`)
  `setMode` accepts yolo, syncs `appState.SetMode` AND `permissionRequester.SetYoloMode(mode ==
  yolo)`; `toggleMode` cycles `build → plan → yolo → build`; `/new`+`/clear` session-restore path
  preserves yolo the same way it preserves plan (do not force-reset to build). Tests: extend
  `repl_helpers_test.go` / `command_handlers_test.go` — set/toggle cycle covers all three modes,
  appState + requester stay in sync, clear/new preserve yolo.

- [ ] Task 5 — `/yolo` + `/mode yolo` slash commands
  (`internal/cli/repl/commands/commands.go`, `command_handlers.go`)
  Add `Yolo = "/yolo"` const with `All`/`Suggestions` entries ("Enable approval-less yolo mode");
  add a dispatch case for `/yolo` (activates yolo via `setMode`) and extend `handleModeCommand`
  to accept `plan|build|yolo` (update `Usage: /mode plan|build|yolo`, bare-`/mode` hint, and the
  `Mode:` status line). Tests: extend `commands_test.go` + `command_handlers_test.go` —
  `/yolo` enters yolo, `/mode yolo` enters yolo, `/mode invalid` still shows usage and keeps mode,
  `/help` lists `/yolo`.

- [ ] Task 6 — `shift+tab` three-state toggle + help text
  (`internal/cli/repl/handlers.go:595-597`, `repl_helpers.go:32-63`, `tips.go`)
  `shift+tab` cycles build→plan→yolo (via the Task-4 `toggleMode`); update loading tip
  (`` `Shift+Tab` swaps plan ↔ build `` → mention yolo), `/mode build` hint, `tips.go`
  plan/shift-tab tips. Tests: extend `handlers_test.go` — three consecutive `shift+tab`
  presses cycle build→plan→yolo→build with appState in sync.

- [ ] Task 7 — Red yolo input UI
  (`internal/cli/repl/theme/styles.go`, `repl_helpers.go:595-635`, `repl.go:869-885`)
  Add `ModeYoloChipStyle` (red background, e.g. `ErrorColor`), `YoloInputRuleStyle` /
  `PromptYoloStyle` (red foreground); `renderInputArea` uses the yolo rule style when focused in
  yolo mode and the yolo chip for the mode label (same shape as build/plan, only color differs);
  `repl.go` sets `Focused.Prompt = PromptYoloStyle` in yolo mode. Tests: extend `repl_test.go` —
  yolo view contains the yolo chip render and the yolo rule-style color prefix (mirror the
  existing plan-mode assertions at `repl_test.go:308-313`).

- [ ] Task 8 — Docs + full verification
  Update `docs/cli-usage.md` (`/yolo`, `/mode plan|build|yolo`, 3-state `shift+tab`, red input
  indicator) and `docs/permission-system.md` (yolo lookup order: yolo → autoApprove → project
  allow → session → prompt; guard `PermissionDenied` still blocks). Run `gofmt`, `go mod tidy`,
  `go test -race ./...`, `git diff --check`. Record results in `PROGRESS.md`.

## Locked design constraints

- Yolo skips interactive permission *prompts* only (including the dangerous-bash prompt in
  `internal/tools/bash.go:152-163`). Filesystem `Guard` `PermissionDenied` verdicts
  (blocked system paths, `.gitignore`, home dotfiles) still deny — same boundary as
  `/allow-permission`.
- Do not reuse `autoApprove` for yolo: headless (`headless_run.go`) keeps its own
  `NewAutoApproveRequester`; interactive yolo is a per-mode flag on the existing `Requester`
  so leaving yolo mode restores normal prompting.
- Yolo has the full build tool registry (no plan-style tool stripping) and its own system-prompt
  branch; plan-mode restrictions are untouched.
- Same input text area shape in all modes — only the color/chip changes to red in yolo.
- Keep existing public tool contracts and permission checks; file operations must go through
  `internal/filesystem` guards. No new dependencies. No secrets in code, logs, or commits.

## Completion protocol

Do not emit the completion marker until all Tasks 1–8 are done, all checklist entries are
checked, no blocker remains, `go mod tidy` has passed, `gofmt` has been run on modified Go files,
and `go test -race ./...` passes.

When and only when everything is complete, end your final response with this exact standalone line:

<YOLO_IMPLEMENTATION_COMPLETE>

Otherwise, summarize the one task completed, tests run, and the next unchecked task. Do not emit
the marker.
