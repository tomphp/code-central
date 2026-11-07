# 37: Ticket detail, the Blocked panel and `move`

**What to build:** The human sees everything about a selected Ticket below its action panel, can undo an edit by reading its previous text, can unstick a Ticket stranded behind a Cancelled Blocker, and can move any Ticket to any State except `Failed`. See the [spec](../spec.md), section "UI".

**Blocked by:** 28, 33

**Status:** resolved

- [x] The Ticket detail shows, after the action panel, its Spec, Labels, description, Claim activity, and a collapsed history of moves, comments and edits merged by time
- [x] Each edit reads like "description edited by <Actor>" with a "show previous" toggle; there is no restore action
- [x] The detail offers `move` to any State except `Failed`
- [x] The Blocked panel, for a Ticket blocked by a Cancelled Ticket with no Replacement, names that Blocker and offers remove the Blocker, To Draft and Cancel
- [x] The detail and the Blocked panel are shared components, independent of the inbox layout
- [x] The TypeScript API module and its fake gain `move` and `remove_blocker`, covered by the contract suite
- [x] Component tests against the fake API check the history, the "show previous" toggle and the Blocked panel's actions
