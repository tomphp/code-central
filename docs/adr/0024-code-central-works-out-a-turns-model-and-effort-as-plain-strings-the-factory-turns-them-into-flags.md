# Code Central works out a turn's model and effort as plain strings; the Factory turns them into flags

Every session the Factory started ran on the model its `--model` flag named, or Claude Code's default, with no way to set effort. The kinds of work differ too much for one setting, so the human wanted a model and effort per kind of work for each Project, with a Ticket able to override both. The **Model Choice** is worked out separately for model and for effort, highest first: the Ticket's `model` or `effort` Tag, the Project's Models row for its kind of work, the Project default row, the Factory's `--model` or `--effort` flag, then Claude Code's own default, which means passing no flag.

We decided that Code Central works out tiers 1 to 3 and the Factory tiers 4 and 5. The claim result carries the Model Choice for the first turn, and a Factory-only `get_model_choice` read gives it for each resume. Each value comes with its source, `none` when Code Central sets nothing. The Factory fills each unset value from its own flag, passes `--model` and `--effort` only for set values, and reports what it passed with `report_model_choice`, which records them on the Claim. Code Central treats model and effort as plain strings it never interprets, beyond checking effort against the valid levels. This keeps the precedence in one place, beside the Ticket panel's display of each value's source, and keeps Code Central agnostic about which Factory runs the work: it serves the same values to any Factory, and only code-central-factory knows they become `claude` flags. Machine-level fallbacks stay with the machine, as the local Factory's plain-code loop does ([ADR 0005](0005-the-local-factory-is-plain-code-starting-one-session-per-claim.md)).

The values are worked out per turn, not per Claim: a Tag or Models change made while a Ticket waits on a Round, or is cut off, applies to its next turn. The Claim records only what its latest turn ran with, for the Claims history.

The Reported Models, the effort levels and model aliases the Factory's `claude` accepts, the model IDs its sessions used and its own flags, are kept in the Project's file in the store across restarts, as Options are kept by the service ([ADR 0012](0012-options-are-kept-by-the-service-apart-from-settings-yaml.md)). The pickers then offer them while no Factory runs. Until any report arrives, the valid effort levels are seeded as `low`, `medium`, `high`, `xhigh` and `max`, so effort can be set on a fresh store.

The `effort` Tag on a Ticket is validated against the valid levels when it is set, a deliberate exception to Tags having no workflow meaning: `claude` silently ignores an unknown `--effort` and runs at its default, so a mistake would otherwise show only as a session quietly running at the wrong effort. `model` takes any single-line value, as every Tag does; only `claude` decides whether it knows the model. On a Spec, `model` and `effort` are ordinary Tags.

## Considered Options

- **The Factory works out every tier.** Rejected: it would read the Project's Models and the Ticket's Tags itself, so the precedence would live in the Factory while the Ticket panel shows its result, and every other Factory would have to repeat it.
- **Code Central works out every tier, the Factory sending its flags.** Rejected: the flags belong to the machine running the Factory, which can change with each start; sending them on every claim and resume would make Code Central hold machine state per request for no gain.
- **Keep the Model Choice per Claim.** Rejected: a change made while a Ticket waits on a Round, often made alongside the answers, would then wait for a new Claim.

## Consequences

A Factory resuming a session makes one more read first, `get_model_choice`; if it fails, the resume waits a poll, like the other resume failures. The Factory reports what each turn was started with, so the Claims history shows the model and effort after every fallback, including the Factory's flags. A subagent a Factory session starts is untouched: nothing is passed down, so it keeps its own agent definition's model. Agents' `get_ticket` leaves the Model Choice out, to keep their context small.
