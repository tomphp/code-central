# Session Events are posted by the Factory and live only in Code Central's memory

The human wants to see what a Factory's agent is doing without `claude --resume`-ing its session, which risks two drivers in one transcript ([ADR 0005](0005-the-local-factory-is-plain-code-starting-one-session-per-claim.md)). So we decided that the Factory posts **Session Events** to Code Central: the agent's text, its tool calls and tool results, and the Factory's own log lines for the Ticket. It posts them over the HTTP API, like any client ([ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md)), to `POST /api/session-events`. Each batch goes to the newest Claim with that session ID and counts as activity, as a ping does. Code Central keeps a bounded **Transcript** per Claim in memory only, never in the store ([ADR 0003](0003-the-store-is-yaml-files-loaded-into-memory.md)), and serves it on a read. It treats each event's tool and text as opaque strings: the mapping from Claude Code's `stream-json` lives only in the Factory, so Code Central stays Factory-agnostic. It reads only the kind, one of the four the endpoint defines, to show a Claim's latest event as one short line, such as `Bash result` for a tool result.

## Considered Options

- **The `ping` hook as the source.** Rejected because a hook sees tool calls but never the agent's text. The Factory already reads every line of the session's `stream-json`.
- **Per-Claim log files beside the store.** Rejected because they would grow without bound beside YAML that is meant to stay small and readable by hand, and a Transcript is worth little once the work is done: `claude --resume` still has the whole session.
- **The Factory serving the events itself.** Rejected because it would make the Factory a server the UI has to find and reach, when today it is only a client.

## Consequences

A Transcript is lost when Code Central restarts, and a Transcript begun after its Claim did is marked as a tail, as is one that has dropped its oldest events. A session run by hand, or a human entering a stuck Claim with `claude --resume`, posts no Session Events, so it has no Transcript, or its Transcript goes quiet while they work. Posting must never slow or fail an agent's turn, so the endpoint always answers 204 and the Factory drops what it can't send.
