# How external trackers model work

Type: research
Status: resolved

## Question

How do GitHub Issues, Azure DevOps and Jira model hierarchy, workflow state, labels, comments and change notification? How well does Spec-owns-Tickets map onto each, and where does it break?

## Answer

All three systems use a tree with at most one parent, so Spec-owns-Tickets fits in shape: Jira best, Azure DevOps next, GitHub worst by default. It breaks in three ways: parents are optional externally, trees go deeper than two levels, and State isn't one-to-one (GitHub issues are only open or closed, with status held per-Project; Azure DevOps and Jira define workflows per type, and Jira changes status only through transitions). No documented production push path reaches a laptop, so polling is what works on all three.

Findings: `research/external-tracker-models.md` on branch `research/external-tracker-models` (commit `16e59d6`).

Carry forward: an Azure DevOps project can hold several repositories and external trees can span them, which the Project definition doesn't yet cover. Atlassian says apps that collect API tokens don't comply with its requirements, and it doesn't say whether a single-user local tool counts.

Not verified: nothing was tested against a live tenant. The one-parent rule is inferred for GitHub and Jira, and whether comment or parent changes bump update timestamps, which polling relies on, is unconfirmed. The research file lists the remaining gaps and source conflicts.
