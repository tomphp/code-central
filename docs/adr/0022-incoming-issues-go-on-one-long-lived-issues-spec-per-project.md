# Incoming Issues go on one long-lived Issues Spec per Project

With GitHub Issues on, every open issue in the repository, or each one with the filter label, arrives as an **Incoming Issue**: a linked Draft Ticket. Every Ticket belongs to a Spec, so Incoming Issues need one. As for CI Rules ([ADR 0017](0017-a-ci-rule-opens-its-tickets-on-a-long-lived-spec-of-its-own.md)), we decided they go on a long-lived Spec of their own: one **Issues Spec** per Project, created by `set_github_issues` the first time GitHub Issues is turned on and titled after the repository, such as "GitHub Issues: acme/web". It is an ordinary Spec, owned by Code Central, with no Link and no Stages, and serves as the inbox of Incoming Issues and their history.

The Issues Spec's ID is stored on the Project with its GitHub Issues settings. An update writes one record, so `set_github_issues` writes the Spec first and the settings second, as `add_ci_rule` does: the settings never name a missing Spec, and a failure in between leaves only an unused Spec. Turning GitHub Issues off and on again reuses the same Issues Spec. Changing the repository retitles it unless the human has changed its title, in a write after the settings', as editing a CI Rule does; its body names no repository, so it never needs to follow. Incoming Issues from the old repository stay as they are: their Links name their repository.

## Considered Options

- **A Spec per issue.** Rejected: the Spec list would grow by one for every issue, and most would end as Won't fix. To Requirement makes a Spec only for the issues the human takes on ([ADR 0021](0021-to-requirement-snapshots-an-issue-and-turns-its-link-outbound.md)).
- **An existing Spec the human chooses.** Rejected: Incoming Issues would mix with that Spec's own work, and the human would have one more setting to choose before turning GitHub Issues on.

## Consequences

Incoming Issues are kept together and read as the Project's issue inbox. The Issues Spec stays when GitHub Issues is turned off, as the record of its Tickets, and is never Finished by Code Central.
