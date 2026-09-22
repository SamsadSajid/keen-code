# Auto approval architecture

## Decision

Use option B from [the design comparison](design/auto-approval-approaches.html).
Keep the tool guards. Add a small internal approval subagent after the guard
check and before the operation. Use one model request with no tools.

Three Terra review agents used medium effort. Each reviewed all four options.
The [security review](design/review-security.md),
[architecture review](design/review-architecture.md), and
[cost review](design/review-cost.md) all ranked B first.

## Review decisions

Option A misses operations that do not request permission. Option C must copy
tool logic and cover every registry. Option D doubles model cost but does not
close these gaps. B can review the operation at its execution point.

The reviews disagree about child tools. This design follows the security
review: child tools must use the same auto policy when the parent uses auto
mode. Their current unconditional approval must not bypass that policy. Keep
their existing behavior in other modes.

The security review suggests a shell allowlist or manual approval for all
shell commands. Do not use that restriction as the complete feature. The
requested feature needs model classification. Send eligible shell commands
to the subagent, including commands the static classifier does not flag.
Keep known dangerous commands, detected secrets, and oversized commands on
the manual path. The fixed prompt must assess compound commands, network
effects, scripts, substitutions, deletion, and effects outside the workspace.

Use the current provider and model in the first release. This avoids a second
credential setup and works with the existing provider clients. The fixed
review task has small context. It can still be slow or costly with a large
model. A separate model setting can be added later.

## Components

The tools own operation data. Keep the public tool schemas and existing
permission interface. Add an optional typed operation review interface. A
requester that does not implement this interface keeps its present behavior.

The REPL owns auto mode and its manual fallback. Add `/auto` and `/mode auto`.
Show auto in the mode indicator and help. Keep plan write restrictions and
build and yolo behavior. Do not enable auto silently in headless execution.

An internal approval package owns the fixed prompt, bounded input, strict
result parser, and provider call. Reuse the LLM client interface. Do not use
the general subagent runner. That runner can supply tools and extra context.
The approval subagent has no registry, history, project instructions, skills,
memory, parent task, or tool results. Disable tool calls and compaction. Use a
single response. Stop on timeout, cancellation, or excess output.

Provider adapters must stop if the model returns a tool call while tools are
disabled. They must not execute the call or send another model request with
that call. Use a dedicated client for the approval subagent. Do not share
the main conversation client.

## Policy and data

Hard filesystem denials run first. Model approval cannot override them.
Keep explicit user grants separate from model grants. A model grant applies
only to the exact operation in progress. Do not cache it or store it as a
session grant. In auto mode, an operation that needs manual review must not
use an old broad session grant to skip that review.

Review every eligible Bash command in auto mode. Send the full command and
working directory. Do not send an agent-written summary as evidence of safety.
Limit commands to 4 KiB. Require user review above the limit.

For writes and edits, review the exact final change. Use bounded before and
after data or a complete computed diff. Include whether the file exists.
Check for detected secrets before serialization. Limit the complete JSON
operation record to 8 KiB. Path data and JSON escaping count toward this limit.
If the data is too large or cannot be safely represented, ask the user. Do
not truncate the operation or send only a digest for semantic review.

Use local path checks for reads and changes. Require user review for external,
sensitive, memory, and symlink targets. Ordinary local reads need no model
call. Search tools must not leak sensitive files through a broad search.
Keep file access checks in `internal/filesystem`. Apply extra checks for auto
mode without weakening the existing guards.

Keep MCP calls on the manual path. Do not send MCP arguments to the approval
subagent. Unknown review operation types also need manual approval.

Accept only the exact defined approval response. Reject extra fields,
duplicate keys, extra text, empty output, and partial streams. Limit output
to 256 bytes. Use a 15-second timeout. Provider errors and review timeout
fall back to the normal prompt. Parent cancellation stops the operation.

## Trade-offs and limits

Each eligible shell command or file change adds a model call. Local policy
avoids calls for ordinary reads and operations that need manual approval.
There is no decision cache. This costs more but avoids stale grants.

The model has limited context. It cannot prove what an external script or a
network service will do. It must ask the user when effects are uncertain.
Secret detection is incomplete. A model classifier is not an operating
system sandbox. Static policy and manual review remain necessary.

Path checks can reduce symlink risk. They cannot provide a complete sandbox
against a concurrent hostile process. Do not claim that they do. Recheck
auto-granted targets before effects where practical.

## Verification

Use [the verification plan](design/verification.md). Tests must cover the
subagent boundary, all operation paths, fallback behavior, and mode changes.
The baseline test run passed all 1,935 tests before implementation. Tests need
permission to open local HTTP servers in this development environment.

The final race-enabled test run passed 2,006 tests in 39 packages. Build, vet,
format, and whitespace checks passed. See the verification plan for the
requirement evidence and test limits.
