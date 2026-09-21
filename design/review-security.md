# Security review: auto approval options

> Historical review. This document records an independent proposal. The final
> decision is in [arch.md](../arch.md), which supersedes this proposal.
>
> Changed proposal: this review proposed a Bash allowlist or manual approval
> for all Bash commands. The final design sends eligible commands to the model.
> It keeps dangerous, secret, and oversized commands on the manual path.

## Recommendation

Choose B, with the required changes below. It reviews an operation after
the existing guard has made its decision. It can use a fixed reviewer with no
tools or ambient context. It also keeps the normal permission prompt as the
fallback.

The reviewer must not be the security boundary. Local policy and the tool
guards must deny or require the user before the reviewer runs.

## Rank

1. **B. Guard first, then review the exact operation.** Best starting point.
   It can cover Bash and file operations at their real execution point. It
   keeps the operation data small and avoids reviewer tools.
2. **C. Review at the tool dispatcher.** It can provide broad coverage. It
   needs every registry and direct execution path to use the wrapper. It also
   needs typed, redacted input. Raw tool JSON is unsafe review context.
3. **A. Classify pending permission requests.** It misses operations that do
   not request permission. Today, a read inside the working directory is
   granted by the guard. A non-dangerous Bash command also passes the initial
   `.` read check. A bypassed request receives no review.
4. **D. Two subagents with a shared decision.** It only repeats a decision.
   It does not add coverage or a hard policy boundary. Two models can miss the
   same shell trick. Do not use this in the first release.

## Required changes for B

* Define a typed operation record. Include tool kind, verb, canonical target,
  workspace status, and a bounded effect summary. Include the full Bash string
  only after secret redaction and size validation.
* Make local policy decide first. It must require the user for every MCP call,
  unknown tool, network action, sensitive path, external path, shell write,
  process-control action, and unsafe file change. It must deny hard filesystem
  policy violations. The reviewer may allow only an eligible operation.
* Do not treat the Bash classifier as complete. Bash runs `bash -c`. It permits
  redirections, expansions, functions, sourced files, substitutions, aliases,
  and interpreter payloads. Classifier uncertainty must require the user. Auto
  approval should use a small read-only allowlist, or prompt for all Bash in v1.
* Review file content only when needed. Redact detected credentials first. If
  redaction, parsing, canonicalization, or the byte limit fails, require the
  user. Do not send content only because it fits the limit.
* Canonicalize targets for policy and review. `ResolvePath` cleans a lexical
  path. It does not resolve parent symlinks. Recheck the opened target before
  the effect. A symlink must not change a reviewed workspace path to an
  external target. Limit this check to auto grants in v1.
* Bind approval to one immutable operation record. Do not grant by tool name,
  path prefix, request ID, or session. Recompute and compare the record at
  execution. Any change requires a new prompt.
* Use a closed decision schema, one short result, a timeout, and cancellation.
  Invalid, missing, extra, timed-out, or provider-error output must prompt the
  user. Never execute on a reviewer failure.
* Give the reviewer no tools, project instructions, skill list, memory,
  transcript, environment, or tool result. Use a fixed prompt and model
  configuration. Do not log unredacted review input or decisions with secrets.

## Specific concerns in the other options

**A:** It is too late in the current flow. `read_file` and search tools call
the requester only for a pending guard result. The guard grants normal reads
inside the working directory. Bash asks again only when the static classifier
or the model-provided danger flag says it is dangerous. A can therefore miss
both secret reads in the repository and unclassified shell writes.

**C:** A dispatcher wrapper must apply to the main registry, child registries,
direct calls, and future tools. It must construct operation records after each
tool has resolved paths and parsed edits. Otherwise it copies security logic,
drifts from the tools, and can review a different operation.

**Child tools:** `subagents.ToolFactory` installs `AutoApprover`. Its
`RequestPermission` method always returns true. The child guard still blocks
hard paths, but it grants every pending external read and write request. It
would bypass the new auto reviewer for delegated Bash, file, and MCP tools.
When the parent is in auto mode, give every child registry a policy-aware
requester that calls the same B operation review. Do not give the child a
separate auto grant. Preserve `AutoApprover` for existing non-auto subagent
modes until their permission behavior is changed deliberately. Add a test for
a delegated external write, dangerous Bash command, and MCP call in auto mode.

**D:** Use it only as a later optional signal inside B. Both reviewers need
the same redacted record and the same hard policy. Disagreement and all errors
must prompt the user. Its extra cost does not fix the bypasses above.

## Tests required before release

Test denied and prompted outcomes for `.env` and key files in the workspace.
Test external and symlinked targets, ignored files, memory files, secret MCP
arguments, and oversized edits. Test Bash redirection, substitutions, `source`,
interpreter forms, pipelines, variables, encoded payloads, and unclassified
commands. Test record changes after review, invalid output, timeout,
cancellation, provider error, and all registry paths. Test a delegated tool
separately. Its auto-mode requester must follow the parent policy.
