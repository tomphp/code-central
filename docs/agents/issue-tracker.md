<!-- Written by code-central init 5af8e3b. Re-running init rewrites this file. -->

# Issue tracker: Code Central

Specs and Tickets for this repository live in Code Central, through the `code-central` MCP server (or the `code-central` CLI, same operations in kebab-case). The CLI reads the Project from `.mcp.json`, so run it at the repository root (or `--project <key>`); `code-central <operation> --help` shows the flags. A Claim's `stage_guide` has what you need; `get_workflow` has the rest.

| When a skill says | Use |
| --- | --- |
| Publish to the issue tracker | `create_spec` for a spec, `create_ticket` for an issue |
| Fetch the relevant ticket | `get_ticket` (`parts` adds Rounds, moves and edits) or `get_spec` (`parts` and `sections` read less; `sections` alone returns only the body) |
| Search or list issues | `find_tickets` (`full`: whole Tickets) or `find_specs` |
| Comment | `comment` |
| Start a Retro | `submit_retro` |
| Apply or remove a label | `add_labels` or `remove_labels` |

Triage roles: `triage-labels.md`. Link a Spec or Ticket as `[<id>](<id>)`, an ID Link.

## Wayfinding

- The map is a Spec with the `wayfinder:map` Label, and its child tickets are Tickets on that Spec, each with one Wayfinder Label for its type: `wayfinder:chart`, `wayfinder:grilling`, `wayfinder:research` or `wayfinder:task`.
- Map Tickets are created in `Draft`, except research Tickets, approved to `Ready`.
- Blocking is `add_blocker`, and the frontier is the Map's Draft Tickets that aren't blocked: `find_tickets(spec, state: Draft)` and keep those not `blocked`. Starting one is `approve`.
- Claim with `claim(ticket)`, resolve with `finish`; the human accepts it.
