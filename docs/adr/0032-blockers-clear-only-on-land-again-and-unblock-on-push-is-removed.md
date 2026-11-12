# Blockers clear only on land again, and Unblock on push is removed

[ADR 0023](0023-under-unblock-on-push-a-blocker-clears-once-its-work-is-pushed-while-landing-still-waits-for-green.md) let a Project's Blockers clear once their work was pushed and its CI result was running or passed. It is superseded. [ADR 0019](0019-under-merge-and-push-with-ci-tracking-a-ticket-lands-only-on-a-passed-ci-result.md)'s rule stands again: a Ticket waiting Accepted on CI blocks the Tickets it blocks until it lands, so Blockers clear only on land, and under the CI gate only on green.

The option cost more than it now gives. Land per Spec ([ADR 0028](0028-under-land-per-spec-a-specs-work-gathers-on-a-spec-branch-and-lands-together-and-blockers-within-the-spec-clear-on-accept.md), [ADR 0031](0031-land-per-spec-covers-every-spec-but-long-lived-ones-which-a-reserved-spec-only-label-marks.md)) already clears Blockers inside a Spec on accept, and it covers every Spec but Long-lived ones. What was left made the Blocker rule read a Project's CI Runs and the current time, and it would have needed a special case once a later commit's green result can land the Tickets waiting on CI: a Ticket that failed on its own commit would read running again on the next push and unblock its dependants before anything had gone green.

These cases now wait one CI run per link:

- a Project with Land per Spec off;
- a chain of Tickets on a Long-lived Spec;
- a Blocker on another Spec.

Blockers inside a Spec still clear on accept under Land per Spec. The Blocker rule no longer reads CI Runs, a Blocker no longer shows a CI result, and a CI Run report no longer sends a change notice to the Tickets its Ticket blocks. A Project file that still holds `unblock_on_push` loads, and the value is dropped the next time the Project is written; the store format doesn't change.
