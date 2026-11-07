# What Sandcastle needs from a ticket source

Type: research
Status: resolved

## Question

How does Matt Pocock's Sandcastle orchestrate work: how does it find and claim work, run agents, and report outcomes, and how do sandboxed agents reach services on the host? What must Code Central's API offer so that a Sandcastle-based Factory could use it later?

## Answer

Sandcastle exists (`@ai-hero/sandcastle`, read at `ed8e716`, 2026-11-05), but it is a library that runs one agent in one sandbox, not a scheduler. It has no runtime tracker and no claim concept: the caller's script and the agent's prompt find and close work through shell commands. So a Sandcastle-based Factory needs shell-callable list, view, close and comment operations with JSON output, a configurable base URL because the host address differs by sandbox provider, and an atomic claim if more than one loop runs at once.

Findings: `research/sandcastle-needs.md` on branch `research/sandcastle-needs` (commit `126a014`). Section 8 sorts the requirements into required, required only with concurrency, long-term and not needed.

Not verified: whether containers can reach a service bound only to `127.0.0.1`, how the separate `factory daemon` tool claims work (its source could not be read), and MCP from inside a sandbox (nothing was run).
