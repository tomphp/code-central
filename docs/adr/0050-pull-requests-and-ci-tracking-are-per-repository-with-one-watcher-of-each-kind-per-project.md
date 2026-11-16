# Pull requests and CI Tracking are per Repository, with one Watcher of each kind per Project

A Ticket can change several Repositories, each with its own remote, Code Host and CI. GitHub has no pull request dependency across repositories and no cross-repository check (research in code-central-831). [ADR 0049](0049-check-every-repository-before-landing-any-no-all-or-nothing-landing-across-repositories.md) already says a Ticket lands only once it has landed in every Changed Repository.

So under Pull Request the Pull Request Watcher opens one pull request per Changed Repository, into that Repository's own Base, or one per Spec Branch per Changed Repository under Land per Spec. Each pull request's body lists its sibling pull requests in the landing order, kept up to date by the watcher, and mentions the Spec's linked GitHub issue with no closing keyword; Write-back already closes the issue. The human decides the merge order, and Code Central doesn't enforce it. Each merge records that Repository's landed commit, at the host's merge commit, and the Ticket stays Partly landed until every pull request has merged. Pull request closed is raised per pull request: sending the Ticket back closes the open siblings, Repositories that already merged keep their landed commit and have their Ticket branch moved to the Base tip, and Cancel is offered only while no sibling has merged. The Code Host is per Repository, worked out from its Clone URL, and the PAT variable stays one Project-wide setting.

CI Tracking stays one switch for the Project. Each Repository has its own CI Provider and what to watch, filled in from its Clone URL, or is Not watched; the Home Repository's entry is the Project's `ci.repository`. A Ticket has a CI result per Changed Repository that isn't Not watched, combined into the Ticket's CI result as running > failed > cancelled > passed. Under the CI gate each Repository lands on its own passed CI result; under Branch + Push and Pull Request the result stays informational. A red build is one CI item per pushed commit, named `Name@sha`, and Land anyway, Re-run failed jobs, New fix Ticket and Send back to fix each act on that Repository's commit only.

There is one Pull Request Watcher and one CI Watcher per Project, each watching every Repository. A failing Repository is named in the error, and the watcher carries on with the others. Turning Pull Request on, or adding a Repository while it is on, needs every Repository, or the new one, to pass the watcher's check.

This amends:

- [ADR 0018](0018-the-service-starts-and-supervises-a-ci-watcher-per-project.md): still one CI Watcher per Project, now watching every Repository, with a CI Provider and target per Repository instead of one `ci.repository`.
- [ADR 0019](0019-under-merge-and-push-with-ci-tracking-a-ticket-lands-only-on-a-passed-ci-result.md): each Repository lands on its own passed CI result.
- [ADR 0029](0029-pull-request-is-a-third-landing-method-carried-out-by-a-pull-request-watcher-and-lands-at-the-code-hosts-merge-commit.md): one pull request per Changed Repository, and the Code Host per Repository.
- [ADR 0034](0034-under-the-ci-gate-a-ticket-lands-once-the-base-is-green-at-any-commit-containing-its-work.md): it holds per Repository, at any commit on that Repository's Base containing the work.

## Considered Options

- **Code Central enforcing the merge order through draft pull requests.** Rejected: more machinery, and drafts behave differently on GitHub and Azure Repos.
- **One combined CI failed item per Ticket.** Rejected: it breaks today's rule that one commit's item covers every Ticket sharing that commit.
- **One Watcher per Repository.** Rejected: more processes to start and supervise, for no gain over one Watcher per Project that names the failing Repository.
- **One CI Provider per Project.** Rejected: it can't handle a Project that mixes GitHub and Azure, or Repositories with no CI.

## Consequences

- A Project can mix GitHub and Azure Repos, and can have Repositories without CI.
- A red build in one Repository holds back only that Repository's landing for the Ticket, and the Ticket shows Partly landed.
- A Partly landed Ticket can't be cancelled.
- A single-repository Project is unchanged: one pull request, one CI Provider and target, and one CI result per Ticket.
