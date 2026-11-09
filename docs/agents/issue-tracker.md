<!-- Written by code-central init 5af8e3b. Re-running init rewrites this file. -->

# Issue tracker: Code Central

Specs and Tickets for this repository live in Code Central, through the `code-central` MCP server (or the `code-central` CLI, with the same operations in kebab-case). Call `get_workflow` before working a Ticket: it holds the rules, and this file only maps the skills' operations to Code Central operations by name.

| When a skill says | Use |
| --- | --- |
| Publish to the issue tracker | `create_spec` for a spec, `create_ticket` for an issue |
| Fetch the relevant ticket | `get_ticket` (its default leaves out Rounds, moves and edits; `parts` asks for them) or `get_spec` (which takes `parts` and `sections` to read less) |
| Search or list issues | `find_tickets` or `find_specs` |
| Comment | `comment` |
| Apply or remove a label | `add_labels` or `remove_labels` |

Triage roles are in `triage-labels.md`.

## Wayfinding

- The map is a Spec with the `wayfinder:map` Label, and its child tickets are Tickets on that Spec.
- Blocking is `add_blocker`, and the frontier is `find_tickets(spec, claimable)`.
- Claiming is `claim(ticket)`, and resolving is `finish`, followed by the human accepting it.
