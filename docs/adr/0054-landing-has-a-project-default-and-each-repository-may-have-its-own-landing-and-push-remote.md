# Landing has a Project default, and each Repository may have its own Landing and Push remote

Status: accepted. Reverses the Project-wide Landing of [ADR 0062](0062-a-projects-repositories-are-a-project-setting-in-the-store-not-a-file-in-the-home-repository.md) and amends [ADR 0052](0052-a-repository-with-no-clone-url-is-local-only-never-cloned-skipped-by-push-and-refused-under-pull-request.md). [ADR 0039](0039-every-landing-rule-is-a-landing-policy-method-and-no-caller-reads-the-landing-settings-to-decide.md) stands.

ADR 0062 decided that "Landing stays Project-wide: no per-Repository Landing Method or Push remote", and ADR 0052 repeated that the Landing Method stays Project-wide. A Project's Repositories now differ: one is on GitHub and wants Pull Request, another is local-only or reviewed by merging, and a pushed one may use a remote not called `origin`. One Landing Method and one Push remote for all of them forces the same choice everywhere.

So the Project's Landing Method and Push option become the **Default Landing**, followed by every Repository with no **Own Landing**. A Repository may have its own Landing Method, Push option and Push remote, set in place of the Default Landing; the Home may have one. A Repository's Landing Policy is its Own Landing, or else the Default Landing, with Push off when the Repository is local-only. The Push remote is `origin` unless the human names another, for the Project (the default) or for a Repository. Rules that are Project-wide hold when any Repository's policy calls for them: the Pull Request Watcher runs when any Repository's policy is Pull Request, and the Factory's fetch, push and base search use each Repository's own Push remote.

Pull Request is refused as a local-only Repository's Own Landing, and as the Default Landing while a local-only Repository follows it; it no longer needs every Repository to have a Clone URL. "Done" under Push keeps ADR 0052's meaning: on the remote wherever there is one.

## Considered Options

- **Derive Push from remotes alone.** Rejected: whether a Repository has a Push remote says whether it can be pushed, not how it lands. It cannot put Pull Request on some Repositories and Merge on others, and cannot choose Branch over Merge for a pushed one.

## Consequences

- ADR 0039 stands: every landing rule is still a Landing Policy method, and there is now one policy per Repository, with Project-wide rules asking whether any of them calls for them. [ADR 0049](0049-check-every-repository-before-landing-any-no-all-or-nothing-landing-across-repositories.md) is unchanged: each Repository's readiness is decided by its own policy.
- Project Options gains Repository rows, where a Repository's Own Landing, Push remote and CI choice are set.
- A Project with no Own Landing set behaves as before.
