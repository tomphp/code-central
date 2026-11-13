# A Small Change opens one Ready Ticket on a Long-lived Small Changes Spec

The human could only ask for work through a Requirement or a Map, both heavy for a typo or a one-line fix. A **Small Change** (`submit_small_change`, **+ New ▾**) turns the human's text straight into one Ready implementation Ticket: the first line is the title and the whole text the description. Every Ticket belongs to a Spec, so we decided Small Changes go on a Long-lived Spec of their own: one **Small Changes Spec** per Project, created on first use, titled `Small Changes` and labelled `long-lived`, as the CI Fixes Spec is ([ADR 0022](0022-incoming-issues-go-on-one-long-lived-issues-spec-per-project.md)).

Its number is kept on the Project (`small_changes_spec`), and Code Central finds it only by that number, so the human may retitle it. As for the CI Fixes Spec, the Spec is written first, the Project records its number second and the Ticket is written third: a failure in between leaves at most an unused Spec, never a Ticket without one. The Ticket has no Stage Label, so the implementation Stage Guide builds it like any other.

## Considered Options

- **A Spec per change.** Rejected for the same reason as [ADR 0022](0022-incoming-issues-go-on-one-long-lived-issues-spec-per-project.md): the Specs list would grow by one for every change.
- **A Requirement with its Stages pre-cancelled.** Rejected: it changes the shape of a Requirement and still makes a Spec per change.
- **Tickets on whichever Spec the human picks.** Rejected: they would mix with that Spec's own work and wait for it under Land per Spec.

## Consequences

Small Changes land one by one, outside Land per Spec ([ADR 0031](0031-land-per-spec-covers-every-spec-but-long-lived-ones-which-a-reserved-spec-only-label-marks.md)), and the Small Changes Spec is never Finished. If the human takes its `long-lived` Label off, Land per Spec covers its Tickets, as for any Spec.
