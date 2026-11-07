# 31: Silent Claims, ping and hooks

**What to build:** A session that has gone quiet is noticed without guessing. Hooks written by `init` record each session's ID at the claim and with each Round, and ping after every tool call. An `In Progress` Ticket with no activity for 30 minutes becomes a Silent Claim, which clears itself on the next activity; the human can release it. See the [spec](../spec.md), sections "Claims" and "Projects".

**Blocked by:** 24, 25

**Status:** ready-for-agent

- [ ] The session ID is recorded at the claim and with each Round, can be updated with `set_session(ticket, claim_id, session_id)`, and is never checked
- [ ] The CLI uses `CLAUDE_CODE_SESSION_ID` as the default `session_id` when set, with `--session` taking priority
- [ ] `POST /api/ping {session_id}` marks activity on every live Claim with that session ID in any Project, always returns `204` (even for an unknown ID), and ignores pings within 60 seconds of that session's last one
- [ ] Pings update memory only; any write to a Ticket also counts as activity; after a restart, last activity counts from service start
- [ ] `code-central ping` reads the hook's JSON from stdin or takes `--session <id>`, gives up after 300 ms and always exits `0`
- [ ] `code-central hook session-id` returns `updatedInput` with `session_id` added; it always exits without blocking
- [ ] `init` writes a `PreToolUse` hook on `claim` and `post_round` running `code-central hook session-id`, and a `PostToolUse` hook on every tool running `code-central ping`, into `.claude/settings.json`
- [ ] `silent_since` is computed on an `In Progress` Ticket after 30 minutes with no activity, never stored, and clears on the next activity; `Needs Answer` and `Answered` are never Silent
- [ ] A background ticker on the injected clock sends `ticket.changed` when a Claim turns Silent
- [ ] Human `release(note?)` moves `In Progress` or `Answered` to `Ready`, storing any note as a comment, and ends the Claim
- [ ] The 30-minute mark and the 60-second throttle are tested with a fake clock, and `ping`'s 300 ms limit in the CLI package, with no real sleeps
