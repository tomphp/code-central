# The Home Repository can be local-only and Not watched, derived from its clone having no Push remote

Status: accepted. Amends [ADR 0052](0052-a-repository-with-no-clone-url-is-local-only-never-cloned-skipped-by-push-and-refused-under-pull-request.md) and the always-watched Home Repository of [ADR 0050](0050-pull-requests-and-ci-tracking-are-per-repository-with-one-watcher-of-each-kind-per-project.md).

ADR 0052 made a Repository with no Clone URL local-only, but left "the Home Repository unchanged": the Home always had a Clone URL, the Push remote URL the Factory reports, and ADR 0050 made the Home's CI Tracking entry the Project's `ci.repository`, always watched. Yet a Project's Home may be a repository that was never pushed anywhere, and the Factory refused to report an empty Push remote URL, stopped all landing when the remote was missing, and CI always watched the Home.

So the Home Repository can be local-only too, and it is derived, not declared. The Home's Clone URL is its Push remote's URL as the Factory reports it, and is empty when its clone has no Push remote. A Repository with no Clone URL is local-only, the Home included:

- The Factory reports an empty URL instead of failing, and does not stop landing for a missing remote.
- Push skips it, Pull Request is refused for it, and CI Tracking never gates landing there, as ADR 0052 says of any local-only Repository.
- A local-only Home starts Not watched. The Home's CI entry is a per-Repository CI choice like the others, no longer always watched, and the human can still set it by hand.

This amends ADR 0052's "The Home Repository is unchanged" and ADR 0050's "the Home Repository's entry is the Project's `ci.repository`", always watched.

## Considered Options

- **A declared "Home is local-only" Project Option.** Rejected: it would be a second source of truth beside the clone's remotes, so it could say local-only while the clone has a Push remote, or the reverse. Deriving it from the clone having no Push remote needs no setting and cannot disagree.

## Consequences

- A Project whose Home is local-only and whose other Repositories are pushed lands in each by that Repository's own policy ([ADR 0054](0054-landing-has-a-project-default-and-each-repository-may-have-its-own-landing-and-push-remote.md)).
- Adding a remote to the Home's clone later makes the Home pushed again at the Factory's next report; its CI choice stays as it was until the human sets it.
