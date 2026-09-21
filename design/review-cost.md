# Auto-approval review

> Historical review. This document records an independent proposal. The final
> decision is in [arch.md](../arch.md), which supersedes this proposal.
>
> Changed proposal: this review proposed a fixed low-cost model and review of
> every Bash command. The final design uses the current provider and model. It
> sends only eligible Bash commands to the reviewer. Dangerous, secret, and
> oversized commands need manual approval.

## Recommendation

Choose B: guard first, then review the exact operation.

It gives the reviewer the facts that matter. The filesystem guard enforces the
decision. The reviewer has no tools, history, project instructions, skills,
memory, or parent conversation.

Use one fixed, low-cost reviewer model. Give it one bounded request. Require
one exact JSON result. Treat every result except `allow` as a user prompt. Do
not store reviewer decisions.

## Ranking

| Rank | Option | Cost | UX | Failure behavior | Test burden |
| --- | --- | --- | --- | --- | --- |
| 1 | B | One call for each Bash command and pending file change. No call for local reads. | Few prompts for routine work. One clear prompt when review cannot decide. | Guard denial stays final. Timeout, invalid output, secret, or overflow prompts the user. | Focused tests at each guarded operation. |
| 2 | A | Lowest build cost. One call only when the requester receives a request. | Familiar prompt flow. | It has coverage gaps. Safe Bash commands and granted local reads can bypass review. | Small tests, but they prove incomplete coverage. |
| 3 | C | One call per wrapped tool. More calls if every tool is wrapped. | It is consistent in theory. Users see prompts for tools that guards would allow. | Raw input can expose secrets. Direct calls and child registries can skip the wrapper. | Tests for every registry and tool schema. |
| 4 | D | Two model calls and double latency for every reviewed operation. | Delays grow during tool-heavy tasks. Disagreement adds prompts. | Both models can make the same error. Guards and exact data are still required. | All B tests, plus agreement and failure cases. |

## Why B fits the code

`Requester.RequestPermission` receives only the tool, path, resolved path, and
danger state. It cannot assess write content or an edit result. It also
auto-allows in headless mode.

`write_file` builds content before it requests permission. `edit_file` builds
the final content before it requests permission. Review the exact final write
before the atomic write.

`bash` checks the working directory. It then classifies dangerous commands.
Review one normalized Bash operation once. Do not make two reviewer calls for
one command.

The guard denies blocked paths and resolves paths. Do not move these rules into
the model. The existing subagent runner supports a one-shot request. Its child
prompt adds working-directory and profile context. Build a dedicated reviewer
path so neither enters reviewer context.

## Required constraints

1. Review Bash execution, `write_file`, and `edit_file` only. Let the
   filesystem guard allow ordinary workspace reads without a model call.
2. Run the guard and resolve paths first. A denied path must never reach the reviewer.
3. Use an internal reviewer with no registry, tools, delegates, history, skill
   catalog, memory, project instructions, or parent task.
4. Send a typed operation. Include version, kind, tool, resolved path, danger
   flags, and the exact command or final-content digest.
5. Send change text only when it has no detected secret and is at most 8 KiB.
   Send the full command only when it is at most 4 KiB.
6. Do not truncate a command or change. Oversize input must prompt the user
   without a model call.
7. Detect secrets before serialization. A detected or uncertain secret must
   prompt the user. Do not send it to the model.
8. Require exactly `{"decision":"allow"}` or `{"decision":"prompt"}`.
   Reject extra fields, malformed JSON, duplicate keys, and output above 256
   bytes.
9. Use a five-second deadline. Cancellation, timeout, provider error, or parse
   error must return the normal user approval prompt.
10. Bind review to one operation. For a file change, include SHA-256 of the
    final content. Execute only that content.
11. Keep manual permission as the only session or project grant mechanism.
    Reviewer approval must not change it.
12. Keep plan restrictions and filesystem denials ahead of auto mode. Route
    MCP tools and unknown tool kinds to manual approval in release one.

## Tests required for B

Test the reviewer adapter with valid allow, prompt, malformed JSON, extra
fields, duplicate keys, oversized output, and cancellation.

Test Bash with safe, destructive, compound, redirected, subshell, and
secret-bearing commands. Check one review call per command.

Test writes and edits with workspace, external, blocked, symlink-resolved,
secret-bearing, and oversized changes. Check that the reviewed digest matches
the written content.

Test guard denial before review. Test provider failure, timeout, and an
unavailable reviewer. Each must fall back to the normal prompt.

Test `/auto` transitions from build, plan, and yolo. Plan must still block edits. Yolo must retain its current behavior.

Test that no reviewer result creates a session grant. Test that MCP and unknown
tools remain manual prompts.
