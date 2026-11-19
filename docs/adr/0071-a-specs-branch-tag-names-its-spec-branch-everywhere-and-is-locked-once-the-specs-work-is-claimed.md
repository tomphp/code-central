# A Spec's `branch` Tag names its Spec Branch everywhere, and is locked once the Spec's work is claimed

Status: accepted. Builds on [ADR 0028](0028-under-land-per-spec-a-specs-work-gathers-on-a-spec-branch-and-lands-together-and-blockers-within-the-spec-clear-on-accept.md) and [ADR 0031](0031-land-per-spec-covers-every-spec-but-long-lived-ones-which-a-reserved-spec-only-label-marks.md).

Under Land per Spec a Spec's work gathers on one Spec Branch, named `spec/<spec id>`. Under Pull Request that branch is pushed and the Spec's one pull request is opened from it. A team whose work comes from Azure Boards wants the branch to carry the work item's ID, such as `feat/4711`, so that Azure DevOps and the team can see which work item it is.

So a Spec's Spec Branch is named by its **`branch` Tag**, a reserved Spec Tag key beside `map` and `effort`:

- **The name lives in a Tag the human can edit**, and isn't worked out from the Spec's Link. To Requirement fills it in from the Integration's Branch format, but the human can set it on any Spec, linked or not, and correct it before work starts. One function in core gives a Spec's Spec Branch, the Tag's value or else `spec/<spec id>`, and nothing else builds the name. Every Ticket read carries it as its Spec's `branch`.
- **There is one name**, used locally, on the remote and in every Repository, so there is no mapping from a local name to a remote one. What the human sees in the Factory's clone matches the pull request.
- **Code Central checks the value** inside the store update that writes it, on every path that sets a Spec's Tags. It must be a valid branch name, by the rules used for a Base. It can't start with `spec/`, which stays for default names, be a Repository's Base, or have the shape of a Ticket ID, which names Ticket branches. No other Spec that isn't Finished may have the same value. A Ticket's `branch` Tag means nothing and isn't checked.
- **The Tag is locked once any implementation Ticket of the Spec has been claimed.** From then on the Spec Branch may exist, and under Pull Request it may have a pull request, so renaming it would leave that branch and pull request orphaned. A change or removal is refused, saying the Spec Branch already exists under that name. Setting the same value again isn't a change. Stage Tickets don't count, so a Requirement can still be renamed during its Stages.
- **The Tag names only the Spec Branch.** With Land per Spec off, under Branch, or on a Long-lived Spec there is no Spec Branch, so the Tag has no effect, and Ticket branches keep their Ticket-ID names.

## Considered Options

- **Working the name out from the Outbound Link when the Spec lands.** Rejected: the human couldn't see or change the name before the push, a Spec with no Link couldn't have one, and the name would change if the Link or its format changed during the Spec's work.
- **Keeping `spec/<spec id>` locally and pushing it under another name.** Rejected: two names for one branch would have to be mapped everywhere the Factory pushes, force-pushes, opens pull requests and sweeps. A human reading the clone would also see a name the pull request doesn't show.
- **Letting the Tag change at any time.** Rejected: the Factory would start a new branch under the new name while the old branch and its pull request stayed open.
- **Naming Ticket branches from the work item too.** Rejected for now: the Spec Branch is the one that is pushed and gets a pull request under Land per Spec, which is where the work item's ID is wanted.

## Consequences

A Spec's Spec Branch can't be renamed once its work has started. Renaming a live Spec Branch, or moving an existing `spec/<id>` branch and its pull request to a new name, is out of scope. A Finished Spec's Tag doesn't block reusing its name, since its branch has landed and been swept.
