# The Factory runs one Harness, chosen when it starts, behind an interface in the one Factory binary

[ADR 0005](0005-the-local-factory-is-plain-code-starting-one-session-per-claim.md) made the Factory start one `claude -p` session per Claim, and its knowledge of Claude Code spread through the Factory: the argv, the stream-json, how a Turn ended. Other coding-agent CLIs (Codex CLI, Copilot CLI, Gemini CLI, OpenCode) can carry a Claim too (`docs/research/harnesses.md`). This refines ADR 0005 and doesn't supersede it: each Claim is still one session of one CLI, and that CLI is now the Factory's Harness.

## Decision

- The Harness is an interface in `factory/harness`, with neutral types: a Turn in, and an Outcome out (a kind, the exit, the last text, the error class, the denials, whether a review ran, and the accepted efforts when an effort is rejected).
- Each Harness is a package beside it. Claude Code's is `factory/harness/claude`, and only it builds Claude's argv or reads its stream-json.
- A Factory runs one Harness, chosen when it starts. Every Turn it runs, for a Claim or a Chat, is a Turn of that Harness.
- The Factory keeps what is its own: the process group, the stop ladder and the work it tells of. It hands these to the Harness with each Turn, so every Harness is stopped the same way.

## Considered Options

- **A separate Factory binary per tool, sharing a library.** Rejected: the loop, landing, Rounds and the state file would be built into each binary, and a fix to one would have to reach them all.
- **A Harness that must provide every capability, emulating what its tool lacks.** Rejected: emulated Plan Limits, denial lists or review detection would be guesses shown as facts.

## Consequences

- A capability beyond the required core is optional and declared by the Harness; a feature it lacks is absent, not emulated.
- The required core is a headless run, resume, a session ID tied to the Claim, and MCP access to Code Central. A tool without them can't be a Harness.
- Code Central's Claude-specific concepts (Plan Limits, the Context Report, the Claude Args) stay as they are until a second Harness exists.
