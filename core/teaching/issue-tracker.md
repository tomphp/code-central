# Issue tracker: Code Central

Specs and Tickets for this repository live in Code Central, through the `code-central` MCP server (or the `code-central` CLI, with the same operations in kebab-case). A Claim's `stage_guide` has what you need to work it, and `get_workflow` holds the whole workflow; this file only maps the skills' operations to Code Central operations by name.

| When a skill says | Use |
| --- | --- |
| Publish to the issue tracker | `create_spec` for a spec, `create_ticket` for an issue |
| Fetch the relevant ticket | `get_ticket` (its default leaves out Rounds, moves and edits; `parts` asks for them) or `get_spec` (which takes `parts` and `sections` to read less; `sections` without `parts` returns only the body) |
| Search or list issues | `find_tickets` (`full`: whole Tickets) or `find_specs` |
| Comment | `comment` |
| Apply or remove a label | `add_labels` or `remove_labels` |

Triage roles are in `triage-labels.md`.

## Wayfinding

- The map is a Spec with the `wayfinder:map` Label, and its child tickets are Tickets on that Spec, each with one Wayfinder Label for its type: `wayfinder:chart`, `wayfinder:grilling`, `wayfinder:research` or `wayfinder:task`.
- Map Tickets are created in `Draft`, except research Tickets, which are approved to `Ready`.
- Blocking is `add_blocker`, and the frontier is the Map's Draft Tickets that aren't blocked: `find_tickets(spec, state: Draft)` and keep those not `blocked`. Starting one is `approve`.
- Claiming is `claim(ticket)`, and resolving is `finish`, followed by the human accepting it.
