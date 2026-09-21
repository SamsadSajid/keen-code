# Architecture review: automatic approval

> Historical review. This document records an independent proposal. The final
> decision is in [arch.md](../arch.md), which supersedes this proposal.
>
> Changed proposals: this review deferred child-tool review. The final design
> applies the parent auto policy to child tools. This review let prior project
> and session grants run before auto review. The final design does not let them
> bypass an operation that requires manual approval. This review allowed a
> reviewer or manual fallback for external reads. The final design uses manual
> approval for external reads.

## Decision

Choose option B. Add a small operation reviewer after each hard guard check.
Use it only in a new `auto` mode. Keep the current requester as the user
fallback. Do not use the general subagent runner for the reviewer.

This is the smallest design that reviews the operation that will run. It keeps
filesystem denials and the manual permission flow.

## Current architecture

The REPL creates one `permissions.Requester`. It passes that requester to the
main tools through `tooling.SetupToolRegistry`.

Each file tool resolves its path. It calls `filesystem.Guard.CheckPath` before
it asks the requester. The guard denies blocked and ignored paths. It grants
reads in the working directory. It sends writes, edits, external reads, and
memory writes to the requester.

`BashTool` checks the working directory first. It asks again only when the
caller or static classifier marks a command dangerous. A normal shell command
can run without a requester call. `CallMCPTool` always asks the requester. It
sends its arguments as the resolved-path field.

Plan mode removes write tools from the effective child registry and blocks
write calls in the LLM layer. Yolo mode makes the requester allow every request.
Both controls are separate from the filesystem guard.

The subagent runner reuses the provider client and resolved configuration. It
does not provide the required isolation. Its tool factory builds registries,
supplies an always-allow requester, and registers MCP calls when a runtime
exists. The runner uses only `OneShot`. It does not disable tool calls.
`collectResult` has no output limit.

## Ranking

| Rank | Option | Assessment |
| --- | --- | --- |
| 1 | B. Guard, then review | It covers the exact operation. It fits tool-owned guards. It has a clear fallback. |
| 2 | A. Review pending requests | It reuses the requester with little code. It misses safe Bash and guard-granted reads. It cannot review edit content. |
| 3 | C. Dispatcher wrapper | It gives one apparent entry point. It must wrap all registries and direct calls. It also duplicates tool path and edit logic. |
| 4 | D. Two reviewers | It depends on B or C for operation capture. It doubles cost and delay. Agreement is not independent assurance. |

## Required design

Add an internal `approval` package. Its `Reviewer` accepts a typed, validated
operation. It returns only `approve` or `ask_user`. It treats timeout,
cancellation, provider errors, malformed output, and oversize data as
`ask_user`.

Build the reviewer with the current provider configuration and `llm.LLMClient`
factory. Do not create a profile or call `subagents.Runner`. Send two messages:
a fixed system instruction and bounded operation data. Call `StreamChat` with
a nil registry. Set `OneShot`, `DisableAutoCompaction`, and
`DisableToolCalls`. Collect a bounded response. The reviewer has no project
instructions, skills, memory, history, activity stream, parent registry, or
MCP runtime.

Use a fixed structured response, for example `{"decision":"approve"}`.
Reject any other response. The parser must reject extra non-whitespace text.

Add `auto` to `llm.AgentMode`, the REPL command, the mode cycle, and mode
display. `auto` selects the reviewer. Build keeps manual requests. Plan keeps
its write restriction. Yolo keeps its unrestricted requester behavior. The
guard always runs before the reviewer. A guard denial never reaches it.

Do not put review policy in `PermissionRequester.RequestPermission`. It does
not receive the command, file content, edit result, MCP arguments, or operation
type. Keep it as the manual fallback. Add a narrow review dependency to the
relevant main tools.

Review these operations after validation and guard checks, before effects:

| Tool | Operation data | Local policy |
| --- | --- | --- |
| `bash` | Exact command and static danger result. | Review every command in auto mode. The static danger result must require the user. |
| `write_file` | Tool name, resolved path, create-or-replace flag, byte count, and bounded replacement content. | Require the user for blocked, external, memory, or secret-like content. |
| `edit_file` | Tool name, resolved path, operation count, byte count, and bounded computed diff. | Require the user for blocked, external, memory, or secret-like content. |
| `read_file`, `glob`, `grep` | Tool name, resolved base path, and bounded non-content parameters. | Let the guard grant ordinary workspace reads. Review only guard-pending reads. |
| `call_mcp_tool` | Server, tool, and bounded arguments metadata. | Require the user. Do not send MCP arguments to the model in the first release. |

Check the content bound before sending data. Do not truncate a command, path,
or edit into a different operation. Ask the user if a file change is too large.
Detect secrets before including change content. Detection is a privacy filter.
It is not an approval control.

The model result is an advisory grant for one in-flight operation. Do not store
it in `sessionAllowedTools`, project permissions, session files, or history.
The tool must execute the validated data it sent for review. Do not rebuild the
operation after a positive decision.

## Compatibility and integration risks

`replModel` and `AppState` hold the mode. Both must recognize `auto`. Session
reset restores Yolo state in the requester. It must also restore Auto state or
clear it to build by an explicit rule.

The headless runner creates the same main registry. It needs an explicit auto
policy or it must remain manual. Do not create a reviewer where no interactive
fallback exists.

Tools used by child subagents use `AutoApprover`. Auto mode must not change
that behavior in the first release. A parent approval reviewer cannot safely
represent individual child operations without a separate delegation design.

Project allow rules and manual session grants run before user prompts. Define
their precedence before coding. Recommended order: guard denial; plan denial;
yolo allow; project allow; manual session allow; auto review; manual prompt.
This keeps explicit grants working without making model decisions persistent.

## Required tests

Add unit tests with a fake `LLMClient` and a deterministic event stream.

- Test each mode command, mode cycle, display state, and session reset.
- Test precedence for plan, yolo, project allow, session allow, auto, and
  manual permission.
- Test that every reviewed call uses a nil registry and all three restrictive
  stream options.
- Test exact parsing. Reject empty output, invalid JSON, extra text, unknown
  decisions, provider errors, incomplete streams, timeouts, and cancellation.
- Test input and output limits. Confirm that commands and paths are never
  truncated. Confirm that an oversize change reaches manual approval.
- Test Bash for a safe-classified command, a static-dangerous command, a user
  dangerous flag, a guard-pending directory, and a reviewer denial.
- Test write and edit operations with a workspace path, an external path, a
  blocked or ignored path, a memory path, a secret-like change, and a diff that
  differs from the executed change.
- Test reads, glob, and grep. Confirm that workspace reads keep guard behavior.
  Confirm that external reads use the reviewer or manual fallback.
- Test MCP calls always require the user and never disclose arguments to the
  reviewer in the first release.
- Keep provider contract tests for disabled tool calls across OpenAI-compatible,
  Responses, Codex, Anthropic, Bedrock, and Genkit clients.

## Deferred work

Do not add cached grants, two reviewers, child-agent tool review, or automatic
MCP approval in the first release. Each needs its own threat model and clear
operation identity.
