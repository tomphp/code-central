# Under Land per Spec, a Spec's work gathers on a Spec Branch and lands together, and Blockers within the Spec clear on accept

Under Merge the Factory merged each accepted Ticket into the base on its own, and with Push pushed the base each time. A Requirement is usually delivered by several implementation Tickets that only make sense together, so the base and the remote kept getting half a feature, and with CI Tracking each partial push ran CI separately. A dependant also waited for its Blocker to land, which with CI Tracking meant a push and a green run, though it only needed the Blocker's work as a starting point.

Merge gets a **Land per Spec** option, per Project beside Push, stored with the Landing Method and on by default. A stored Project without it reads as on, so the stored field tells unset from off. It is kept whatever the method but has effect only under Merge. Under it, each Requirement Spec has one **Spec Branch**, `spec/<spec id>`, cut from the base at the first claim of one of its implementation Tickets. Ticket branches keep their names but are cut from the Spec Branch's tip. On accept the Factory merges the Ticket into the Spec Branch, not the base, and the Ticket waits Accepted, shown as Waiting for the Spec, with no new State ([ADR 0001](0001-a-ticket-is-always-in-exactly-one-state.md)). Once every Ticket on the Spec is Accepted, Done or Cancelled and the tickets Stage is finished, the Factory merges the Spec Branch into the base, and all the waiting Tickets land together by the Push and CI gate path: at the merge commit, once the one base push succeeds, or once its CI result is passed. Only the base is ever pushed.

A Blocker on the same Spec stops blocking once it is Accepted and merged into the Spec Branch, so dependants start from a branch that already holds its work. A Blocker on another Spec still clears only when it lands, or under Unblock on push. A Spec's CI failure is one item on a lead Ticket, and Send back to fix is not offered for it, since the failure belongs to the whole Spec.

[ADR 0031](0031-land-per-spec-covers-every-spec-but-long-lived-ones-which-a-reserved-spec-only-label-marks.md) amends which Specs Land per Spec covers: every Spec but Long-lived ones, not only Requirements.

It amends:

- [ADR 0006](0006-accepted-work-lands-before-it-is-done.md): Accepted no longer means about to land. Under Land per Spec an Accepted Ticket may wait for its Spec, and Accepted but not landed leaves such Tickets out.
- [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md): the Landing Method gains the option, and the Factory carries out the Spec Branch merges.
- [ADR 0019](0019-under-merge-and-push-with-ci-tracking-a-ticket-lands-only-on-a-passed-ci-result.md): the CI gate holds a Spec's Tickets as a group on one pushed commit.
- [ADR 0023](0023-under-unblock-on-push-a-blocker-clears-once-its-work-is-pushed-while-landing-still-waits-for-green.md): a Blocker on the same Spec clears on its Spec merge.

## Considered Options

- **Stacking Ticket branches on their Blockers' branches.** Rejected: a diagram with several Blockers, or none, has no single branch to start from.
- **A third Landing Method.** Rejected: it applies only under Merge and combines with Push and CI Tracking.
- **A per-Spec setting.** Rejected: a Project-wide option is enough, and long-lived Specs never finish.
- **Clearing Blockers at In Review.** Rejected: work the human might still send back would be built on.

## Consequences

Turning the option on is a behaviour change for every existing Project on upgrade; Tickets already accepted are unaffected.
