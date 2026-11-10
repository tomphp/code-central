# A CI Rule opens its Tickets on a long-lived Spec of its own

A **CI Rule** says "when workflow X fails on branch Y, open a Ticket to fix it", for failures that no Ticket's own CI catches, such as a red `main` after a hand merge or a nightly. Every Ticket belongs to a Spec, so a rule's Tickets need one. We decided each CI Rule gets a long-lived Spec of its own, created by `add_ci_rule` and titled after what it watches, such as "CI: ci.yml on main" or "CI: any workflow on main". The rule is named by its Spec's ID, which is never reused. Editing the rule retitles the Spec unless the human has changed the title. Removing the rule leaves the Spec as the record of its fix Tickets.

The rule is stored on the Project with its CI settings, and its Spec in the store like any other. An update writes one record, so `add_ci_rule` writes the Spec first and the rule second: a rule never names a missing Spec, and a failure in between leaves only an unused Spec. `edit_ci_rule` likewise changes the rule first and retitles its Spec second, so a failure in between leaves the rule changed under its old title; the Spec's body names neither workflow nor branch, so it never needs to follow.

## Considered Options

- **The Spec that caused the failure.** Rejected: rules mostly fire for failures no Ticket caused, such as a flaky test, a dependency or a hand-made commit, so there is rarely a causing Spec, and a fallback Spec would be needed anyway. Where the failing commit is a Ticket's land commit, the fix Ticket's description links that Ticket and its Spec instead.
- **One shared Spec per Project for every CI Rule.** Rejected: fix Tickets for unrelated workflows and branches would pile up together, and "while the rule's Spec has an unfinished Ticket, comment on it instead of opening another" would hold back one rule's failures behind another's.

## Consequences

A rule's fix Tickets are kept together and read as its history, and "no pile-up" is a question about one Spec's Tickets. The Specs stay after their rules are removed, so the Spec list grows by one per rule ever added. They are ordinary Specs, with no Requirement Stages.
