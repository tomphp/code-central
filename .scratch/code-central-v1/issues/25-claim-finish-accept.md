# 25: Claim, finish, accept

**What to build:** The first full delivery loop. An agent claims the next claimable Ticket in its Project, works it and finishes it into `In Review` with a summary; the human accepts it to `Done` or rejects it back to `Ready` with a comment over the HTTP API. Two agents can never take the same Ticket, and a session whose Claim has ended can't write as its holder. The fixed State workflow lives in one table in `core` that every action is checked against. See the [spec](../spec.md), sections "The workflow" and "Claims".

**Blocked by:** 23

**Status:** ready-for-agent

- [ ] `core` holds the nine States and the action table from the spec, and every action is validated against it; a rejected action returns `action_not_allowed` with the allowed actions by name
- [ ] The HTTP API reads the Actor from a request header, defaulting to `agent`; MCP and the CLI always write as `agent`; every move is recorded in `moves` with from, to, Actor, time and the Claim number
- [ ] `claim(spec?, ticket?, session_id?)` takes the next `Ready` Ticket in claim order (`created`, then number) as one atomic step under the write lock; passing both `spec` and `ticket` is an error; a named Ticket that can't be claimed is rejected with its State as the reason
- [ ] When nothing can be claimed, `claim` returns `{ticket: null}` rather than an error
- [ ] `claim` returns a Claim ID (128 random bits, base32) with the full Ticket view; the Claim ID is stored in the Ticket file and left out of every read
- [ ] `finish(ticket, claim_id, summary)` moves `In Progress` to `In Review`, with the summary required and stored as a comment
- [ ] A write with a Claim ID that doesn't match the live Claim returns `stale_claim`
- [ ] Human `accept` moves `In Review` to `Done`; human `reject(comment)` moves it to `Ready` with the comment required, and the Ticket keeps its place near the front of the claim order
- [ ] Any move out of `In Progress`, `Needs Answer` or `Answered` ends the Claim permanently; a rejected Ticket needs a fresh Claim
- [ ] A test in `core` shows two concurrent claims never take the same Ticket
