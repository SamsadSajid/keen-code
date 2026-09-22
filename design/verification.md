# Verification plan

## Mode and integration

- `/auto` selects auto mode. `/mode auto` selects the same mode.
- The mode indicator shows auto. Help lists the new mode.
- A change to build or plan removes automatic review grants.
- Plan mode keeps its write restrictions. Yolo keeps its existing behavior.
- The main tools and delegated tools use the same auto policy.
- A hard filesystem denial never reaches the approval subagent.

## Approval subagent

- Send one fixed system prompt and one bounded operation record.
- Do not send conversation history, project instructions, skills, or memory.
- Give the subagent no tools. Prevent tool execution and compaction.
- Accept only the defined safe response. Reject extra or invalid data.
- Limit input and output. Do not truncate an operation to obtain approval.
- On timeout or provider failure, use the normal permission prompt.
- On parent cancellation, stop the request and do not execute the operation.
- Do not log review inputs or save model decisions as session grants.

## Tool effects

- Review shell commands even when the static classifier does not flag them.
- Approve an ordinary low-risk command through the subagent.
- Require a user decision for destructive, secret, or uncertain operations.
- Exercise compound commands, redirections, substitutions, and interpreters.
- Approve an ordinary local file change through the subagent.
- Require a user decision for sensitive files and external targets.
- Test symlink targets and symlink parents before an automatic file grant.
- Test large edits and content with detected secrets without a model call.
- Keep MCP calls on the user approval path.
- Check direct tools, delegated tools, and normal REPL dispatch.

## Repository checks

Run the Go test suite with the race detector. Run `go build ./...`,
`go vet ./...`, and the Go format check. Inspect the HTML artifact. Check the
final diff for credentials, transcripts, unrelated files, and whitespace
errors. Commit only the intended files. Push the branch. Make one PR creation
attempt. Record the result.

## Final results

Checks on 22 September 2026 passed:

- `go test -race ./...`: 2,006 tests passed in 39 packages.
- `go build ./...` and `go vet ./...`: passed.
- `gofmt -l .`: no files reported.
- `git diff --check`: passed.
- The HTML design document passed the available Tidy markup check.

The browser connection was unavailable. No visual browser check was possible.
Provider tests use controlled responses. They do not measure live model cost
or classification accuracy.

## Requirement evidence

| Requirement | Evidence |
| --- | --- |
| Learn the code and compare four designs | `auto-approval-approaches.html` includes the code map and four options. |
| Independent Terra reviews at medium effort | `review-security.md`, `review-architecture.md`, and `review-cost.md` record the reviews. |
| Select and document a design | `../arch.md` records option B, the review decisions, and the trade-offs. |
| Implement on a branch from main | `feat/96-auto-approval` starts at main commit `8d63d7d`. |
| Keep the review context small | `internal/approval/reviewer_test.go` checks the two-message boundary, limits, and disabled tools. |
| Use manual fallback on review failure | Reviewer and tool tests cover invalid output, errors, cancellation, and denied effects. |
| Consider risky file operations | Filesystem and tool tests cover sensitive paths, external paths, symlinks, and changed file state. |
| Apply policy to delegated tools | `internal/subagents/auto_policy_test.go` checks reads, writes, Bash, and MCP. |
| Reject stale permission responses | `requester_auto_test.go` checks cancellation, request IDs, concurrency, and mode changes. |
| Stop disabled tool calls in every provider | The six provider test files check that no tool or follow-up request runs. |
| Provide usage instructions | `../docs.md` explains commands, limits, and manual review. |

Test paths in this table are relative to the repository root.
