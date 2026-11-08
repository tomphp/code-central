# A write returns an acknowledgement, not the record

Writes used to echo the whole record they wrote. `edit_spec` returned the whole Spec, body and edit history, and each edit keeps the whole previous body, so an agent rewriting a large Spec paid for it again on every edit: 35–48K characters a result on code-central-48. No caller read the echo: not the agents, the Factory, the web UI or the Go client's callers. So we decided that a write that returns a whole Spec or Ticket returns an **acknowledgement** instead: the `id`, the new `updated` time, `changed` (the *names* of the fields that changed, never their values, and empty when nothing did) and `edit` when the write recorded an edit. A caller that wants the record passes `full` (an MCP argument, `"full": true` in the HTTP JSON body, `--full` on the CLI) and gets exactly the result the write used to return. It is the default on every door, MCP, HTTP and CLI, Integrations included. Core chooses between the two in one place, from the `full` input, and derives `changed` in one place, by comparing the record's view before and after the write field by field, so the doors stay translation only ([ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md)) and `changed` can't drift from the record's real shape.

## Considered Options

- **Keep echoing the record.** Rejected because the echo costs the agent its size in context on every write, and nobody reads it.
- **Return nothing but the ID.** Rejected because the agent couldn't tell a no-op from a real edit, or confirm what the write did, without reading the record back.
- **Echo the changed fields' values.** Rejected because even a short-looking field can hold a large value, such as a body.
- **A per-door default, such as the full record on HTTP.** Rejected because the doors would drift, and an Integration author would have two contracts to learn.
- **Spelling out `changed` per write.** Rejected because each write would need its own list, kept in step with the view by hand.

## Consequences

A caller that needs the record after a write makes one more read, or passes `full`; in return every agent write is cheap. An outside Integration that reads a write's result breaks until it passes `full: true`, a one-field change. Computed fields in the view, such as `claimable`, are compared like any other, so a write that changes one lists it in `changed`. The writes that already return small views (Labels, Tags, Projects, `set_external_state`, `unlink` and `import`), `claim` and `pick_up_answers` are unchanged.
