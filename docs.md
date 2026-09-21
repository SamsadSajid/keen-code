# Use auto mode

Enter `/auto` in the interactive prompt. You can also enter `/mode auto`.
The mode indicator shows `auto`.

Auto mode uses a small approval subagent to assess eligible shell commands
and file changes. It uses the current provider and model. It sends a bounded
operation record. It does not send your conversation, project instructions,
skills, or memory. The approval subagent has no tools.

Review has a 15-second timeout. Commands must fit within 4 KiB. The complete
JSON operation record must fit within 8 KiB. JSON escaping and path data count
toward that limit. The response limit is 256 bytes. Larger operations use the
manual prompt. Keen does not shorten an operation to obtain approval.

## Normal work

Ask Keen to do the task as usual. Auto mode can approve a low-risk command
or a small local file change. Ordinary local reads do not need a model call.
The same auto policy applies to tools used by delegated agents.

Auto mode can still ask for your approval. A prompt does not mean the command
failed. It means the automatic check did not grant permission. Read the
command or file change before you choose an action.

The following operations need manual review:

- Commands that the existing classifier marks as dangerous.
- Operations on sensitive files or paths outside the working directory.
- Operations through symlinks.
- Changes with detected secrets or changes too large for review.
- MCP calls and unknown operation types.
- Operations whose effects the approval subagent cannot determine.

Existing hard filesystem denials remain in effect. Auto mode cannot override
them. The model decision applies to one operation. It does not grant access
for the rest of the session.

Auto mode does not use broad project or session grants to skip manual review.
Those grants keep their normal meaning in the other modes. Automatic searches
omit sensitive files and directories. Request a specific path when you need
to review access to it.

## Change the mode

Enter `/mode build` to return to normal permission prompts. Enter `/mode plan`
for planning with the existing write restrictions. `/mode yolo` retains its
existing behavior. Use `/mode` to show the current mode.

Auto mode is an interactive feature. Headless execution keeps its existing
behavior. No provider key or separate subagent profile is needed beyond the
normal Keen provider setup.

## Review failures

If the provider is unavailable, the response is invalid, or review takes too
long, Keen uses the normal permission prompt. If you cancel the operation,
Keen must not execute it because of a late model response.

The reviewer adds a model call for each eligible shell command or file change.
Provider charges and delay depend on your selected model. There is no cache
of model approval decisions.

Auto mode is not a sandbox. The model can make a wrong decision. Secret
detection can also miss a secret. Use manual review for work that needs a
stronger control.

Read [arch.md](arch.md) for the design, trade-offs, and review records.
