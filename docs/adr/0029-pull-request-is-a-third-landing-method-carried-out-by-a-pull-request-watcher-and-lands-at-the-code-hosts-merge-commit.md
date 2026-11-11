# Pull Request is a third Landing Method, carried out by a Pull Request Watcher, and lands at the Code Host's merge commit

Many teams don't merge accepted work themselves, locally or by hand: on GitHub or Azure Repos work reaches the base only through a pull request, with review, CI and the host's branch policies and merge strategies. Under Branch + Push the human opens each pull request by hand, and Code Central decides whether work landed from git ancestry, which a squash or rebase merge breaks, so the Ticket never lands and its dependants stay blocked.

**Pull Request** becomes the third Landing Method beside Merge and Branch, with Push implied: the stored Push flag is ignored under it and the Project Options form has only the Push remote. On accept the Factory pushes the Ticket's branch and reports the pushed commit and the branch pushed. A new kind of Watcher, the **Pull Request Watcher**, opens a pull request into the base on the Project's **Code Host**, watches it and reports its link, its state (open, conflicting, closed or merged) and, once merged, the host's merge commit. On its next poll the Factory lands the Ticket at that merge commit, whatever the merge strategy; git ancestry plays no part. The watcher writes over the HTTP API only, as the CI Watcher does, so the service never calls the Code Host ([ADR 0002](0002-the-service-is-the-only-writer.md), [ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md)); the service supervises it as [ADR 0018](0018-the-service-starts-and-supervises-a-ci-watcher-per-project.md) and [ADR 0020](0020-an-integration-runs-as-a-watcher-supervised-by-the-service.md) describe, whether or not a Factory runs.

The Code Host is GitHub, reached through `gh`, or Azure Repos, reached over the REST API with a PAT from the service's environment ([ADR 0027](0027-azure-pipelines-is-reached-over-the-rest-api-with-a-pat-from-the-services-environment.md)). It is worked out from the Push remote URL the Factory reports, reusing the CI section's URL parsing, so nothing is configured twice and nothing depends on CI Tracking. The human names only the environment variable holding the Azure Repos PAT, `ADO_PAT` by default. Turning the method on is refused, with a reason and a fix, when no URL has been reported, when the host is unsupported, and, once the watcher exists, when its check mode finds `gh` or the PAT unable to contribute to pull requests.

The CI result informs the human but doesn't gate landing: the human's merge is the decision, as under Branch + Push. Under Land per Spec a Requirement Spec's work still gathers on its Spec Branch, and once the Spec is ready to land the Spec Branch is pushed and the Spec gets one pull request; all its waiting Tickets land together when it merges.

It amends:

- [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md): a third Landing Method, whose landing the watcher reports at the host's merge commit instead of the Factory seeing the merge.
- [ADR 0019](0019-under-merge-and-push-with-ci-tracking-a-ticket-lands-only-on-a-passed-ci-result.md): the CI gate holds only under Merge + Push + CI Tracking; under Pull Request there is no gate and Unblock on push has no effect.
- [ADR 0028](0028-under-land-per-spec-a-specs-work-gathers-on-a-spec-branch-and-lands-together-and-blockers-within-the-spec-clear-on-accept.md): the Spec Branch is pushed under Pull Request, though never under Merge.

## Considered Options

- **An option under Branch + Push.** Rejected: opening and watching pull requests, and landing at a merge commit, are a different carrying out of landing, and the combination would leave Branch's ancestry check in place.
- **The Factory calling the Code Hosts itself.** Rejected: the Factory runs only while the human has started it, but merges happen and pull requests must be opened whether or not one runs, and Code Central holds the integrations in Watchers.

## Consequences

Switching away from Pull Request stops the watcher and leaves any open pull requests on the host; Code Central comments on each affected Ticket and Project Options says how many are still open.
