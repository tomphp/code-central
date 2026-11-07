# Keeping an agent session going across Questions

Type: research
Status: resolved

## Question

A grilling session posts a Round of Questions, the human answers, and the same session continues with its context and posts more. How can that back and forth be achieved, given that in v1 Code Central does not start or control agents, the human may take hours or days to answer, and agents may run interactively, headless or in a sandbox? Which mechanisms exist for pausing a session and continuing it with its context, what limits each one, and what would each need from Code Central?

## Answer

The same session can wait for days and carry on, and every way of doing that needs the claim to stay with the asking session, so ticket 05's automatic `Needs Answer` to `Ready` must change. In a live interactive session the best mechanism is an MCP task: Code Central answers the post-Round tool with a task handle, Claude Code polls it with no model calls, and the session wakes by itself. Otherwise the session exits and is resumed with `claude --resume`, triggered by the human, a small waiter or a Factory. That works in every mode and survives a reboot. A blocking tool call is the weakest serious option.

Findings: `research/session-continuity.md` on branch `research/session-continuity` (commit `f37f2bd`).

Carry forward:

- Make one rule for every agent: post the Round and end your turn. Only what wakes the session varies.
- Recommended for v1: the post-Round tool returns a task when the client declares the extension, the claim records what a resume needs, the UI shows a resume command, and a `code-central wait` CLI sits on a long-poll endpoint. A Factory uses exit and resume, and Sandcastle already supports that with `resumeSession`.
- An answered Ticket keeps its claim, with a way to release a claim whose session never returns. That needs deciding, as `In Progress` with nobody awake looks like work in progress. Answering part of a Round wakes nobody, and the glossary has no term for Round yet.
- MCP tasks are a flag-controlled public beta, for the terminal CLI and Agent SDK only, and one-shot `claude -p` doesn't get them. Neither Go library implements the extension, so it would be hand-written.
- An HTTP MCP server is told nothing about the session. A hook can inject the session ID into the tool's arguments.
- A blocking call fails at 60 seconds unless SSE headers are flushed, at 5 minutes without progress, and at about 28 hours regardless.
- Transcripts are deleted after 30 days by default, and a late answer re-caches the whole conversation, about $1.14 for 182,000 tokens in the docs' example.

Not verified: the longest probe ran about 6.5 minutes, so waits of hours or days, the 28-hour limit, laptop sleep, reboot and restarting Code Central during a wait were not run. Nothing ran in Docker, `sbx`, Sandcastle or on Linux. `claude --bg` and agent view were not tried, and Agent SDK code was not run, with stream-json standing in for it. Under `-p` an 8-second idle override did not fire, contrary to the docs, and the cause wasn't found.
