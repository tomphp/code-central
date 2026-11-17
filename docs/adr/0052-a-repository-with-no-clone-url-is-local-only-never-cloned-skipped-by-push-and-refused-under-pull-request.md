# A Repository with no Clone URL is local-only: never cloned, skipped by Push and refused under Pull Request

[ADR 0062](0062-a-projects-repositories-are-a-project-setting-in-the-store-not-a-file-in-the-home-repository.md) made every Repository carry a Clone URL and ruled out a per-Repository Landing Method or Push remote. Not every repository the human works in has been pushed anywhere: some live only on their machine, and work lands in them just by merging into their base. Such a repository couldn't be added without inventing a remote, which the Factory would then clone from, check its Push remote against and push to.

So a Repository other than the Home Repository needs a Clone URL, a local path or both. One with no Clone URL is local-only:

- The Factory never clones it. It adopts the git repository already at its local path with no remote check, and raises Repository missing if nothing usable is there, or if the path is another Repository's clone.
- Push skips it. A Ticket lands there once its branch is merged into that Repository's base, as under Merge without Push, and CI Tracking never gates landing there.
- Pull Request refuses it, in both directions: a Repository with no Clone URL can't be added or have its Clone URL cleared while Pull Request is on, and Pull Request can't be turned on while any Repository has none.
- CI Tracking starts it Not watched.

This amends ADR 0062's "every Repository has a Clone URL". The Landing Method stays Project-wide and the skip is not a setting: it follows from the missing Clone URL. The Home Repository is unchanged.

Under Push, "Done" now means "on the remote wherever there is one". A Ticket that changed a pushed Repository and a local-only one is Done once the first is pushed (and green, with CI Tracking) and the second is merged.

## Considered Options

- **Refuse Push while any Repository is local-only.** Rejected: it would block a pushed Home Repository with a local-only Repository beside it, though the human wants Push for the Home.
- **Require the local path only when the default path is empty.** Rejected: a Repository with no Clone URL and no local path would be adopted or missing at a default path the human never chose, and "needs a Clone URL or the local path of a clone it already has" is a rule the service can check on the fields alone.

## Consequences

- [ADR 0039](0039-every-landing-rule-is-a-landing-policy-method-and-no-caller-reads-the-landing-settings-to-decide.md) stands: every landing rule is still a Landing Policy method, and the per-Repository Landing Policy is the Project's policy with Push off for a local-only Repository. [ADR 0049](0049-check-every-repository-before-landing-any-no-all-or-nothing-landing-across-repositories.md) is unchanged: each Repository's readiness is decided by its own policy.
- A Ticket whose Changed Repositories are all local-only has no CI gate and lands once merged.
- The core can't always resolve default clone paths, so the Factory also raises Repository missing when a Repository resolves to a path an earlier one or the Home already holds.
- Adding a Clone URL later makes the Factory check the existing clone's Push remote against it; it never adds or rewrites a remote itself.
