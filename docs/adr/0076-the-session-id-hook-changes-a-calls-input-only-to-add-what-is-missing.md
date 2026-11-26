# The session-id hook changes a call's input only to add what is missing

Auto mode reviews a tool call and then refuses it when a hook changed its input afterwards. The session-id hook used to answer every `claim`, `set_session` and `post_round` with an `updatedInput` that set `session_id` (overwriting a given one) and added `client_version`, so the first `post_round` of every Round failed under auto mode and had to be sent again.

## Decision

- The hook prints nothing when a `claim` or `set_session` already has `session_id` and `working_dir`.
- When one is missing it adds only the missing fields, plus `client_version`, and never overwrites a value the agent gave.
- `client_version` travels only with fields the hook adds. MCP client info carries Claude Code's version, not the CLI's, and the service can't read the agent's environment, so there is no other carrier.
- The hook does nothing for `post_round`, which takes its Claim's session when `session_id` is left out. `init` no longer matches `post_round`; settings files from an older `init` still call the hook for it, and it is harmless.

## Consequences

- A `claim` or `set_session` that leaves out `session_id` or `working_dir` is still changed by the hook, so under auto mode it can still fail once.
- A Round from a session other than its Claim's needs `set_session`.
- A later change that adds a field to every call, or puts `post_round` back in the matcher, brings the bug back.
