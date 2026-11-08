# The local Factory is plain code that starts one `claude -p` session per Claim

The local Factory polls about every 30 seconds, claims up to a cap, makes worktrees and wakes work when a Round is Answered. Stuck work has to be visible and enterable. The first idea was to start it by hand inside a Claude Code session, so we looked at that first. We decided instead that the Factory is plain, deterministic Go code with no model in its loop. It is its own binary in this repo, an HTTP client of Code Central like any Integration ([ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md)), and you start it by hand as a foreground command in a terminal. Each Claim runs as its own `claude -p` session. The Factory picks that session's ID and passes it to `claim`, and it runs `-p --resume` on the session when its Round is Answered. Everything that knows about Claude Code stays in that binary, so Code Central's model stays Factory-agnostic.

## Considered Options

- **A Claude Code session running the loop, with subagents doing the Claims.** Rejected for these reasons:
  - `/loop` and cron tick in whole minutes, with jitter, and only while the session is idle.
  - Every tick depends on the model following its prompt.
  - Compaction can lose the Factory's map from Ticket to agent to Claim ID.
  - Subagents share the parent's session ID, so a stuck one stays hidden while the parent is busy.
  - `claude --resume` can't open a subagent.
- **A Claude Code session that starts real sessions instead of subagents.** Rejected because once the loop needs no model, a session gains nothing over a terminal command. It only adds the session's own lifetime and background-command timeouts.
- **The Agent SDK (TypeScript or Python).** Rejected because the terms on using a subscription login with it are unclear, and `claude -p` already gives the stream, the session ID and resume.
- **`claude --bg` background sessions** (a supervisor, `claude agents --json`, `claude attach`). These were attractive because permission prompts wait for you and `attach` has no two-driver hazard. We deferred them, not rejected them: it isn't yet known whether `--bg` takes `--session-id`, how to wake an idle session when its Round is Answered, or whether `claude agents --json` is stable enough to poll.
- **A `code-central factory` subcommand.** Rejected because it would put the knowledge of Claude Code inside Code Central's own binary.

## Consequences

Every start and resume passes `--permission-mode auto --permission-prompts none`, because `-p --resume` doesn't restore the mode. Anything auto mode won't approve is denied at once and listed in `permission_denials` on a `success` result, so the Factory has to read that list. A start flag can switch every session it starts to `bypassPermissions` instead. Extra allow and deny rules for a repository live in its checked-in `.claude/settings.json`. `--bare` must never be used, because it skips hooks, `.mcp.json` and the subscription login. The Factory runs on the subscription login as long as `ANTHROPIC_API_KEY` is unset. A human entering a stuck Claim with `claude --resume <id>` and the Factory resuming the same session would interleave in one transcript, so the Factory has to avoid that.
