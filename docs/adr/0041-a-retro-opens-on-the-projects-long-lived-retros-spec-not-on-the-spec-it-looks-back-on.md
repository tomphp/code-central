# A Retro opens on the Project's Long-lived Retros Spec, not on the Spec it looks back on

A Retro looks back on a Finished Ticket or Spec. We decided it opens on one Long-lived Spec per Project, the Retros Spec, which Code Central creates on first use, titled `Retros` and labelled `long-lived`, and finds by its stored number, as it does the Small Changes Spec ([ADR 0035](0035-a-small-change-opens-one-ready-ticket-on-a-long-lived-small-changes-spec.md)). The Tickets a Retro creates for the changes the human picks go on the same Spec.

That way finished work stays Finished, and Land per Spec never waits on a Ticket that never lands. A Long-lived Spec's Tickets are already left out of Land per Spec, so leaving Retros out by Label as well is belt and braces.

## Considered Options

- **The target's Spec.** Rejected: a Retro opened on a Finished Spec would add an unfinished Ticket to it and un-finish it, and under Land per Spec the Spec would wait on a Ticket that never lands.
- **A Spec per Retro.** Rejected: it would fill the Project with Specs that each hold one Ticket and finish nothing.
