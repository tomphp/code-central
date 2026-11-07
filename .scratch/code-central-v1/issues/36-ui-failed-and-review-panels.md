# 36: `Failed` and `In Review` panels

**What to build:** The human resolves a failure and reviews finished work from the inbox. A `Failed` Ticket shows its latest report and the three ways forward; an `In Review` Ticket shows the agent's summary with Accept and Reject. See the [spec](../spec.md), section "UI".

**Blocked by:** 30, 33

**Status:** ready-for-agent

- [ ] The `Failed` panel shows the failure count and the latest report's four fields, then Send back (with an optional note that says no note means nothing has changed), To Draft and Cancel
- [ ] Below the moves, it shows the report's `claude --resume <id>` for investigating, and earlier reports collapsed
- [ ] The `In Review` panel shows the `finish` summary, Accept, and Reject, which can't be sent without a comment
- [ ] Both panels are shared components, independent of the inbox layout
- [ ] The TypeScript API module and its fake gain `send_back`, `to_draft`, `cancel`, `accept` and `reject`, covered by the contract suite
- [ ] Component tests against the fake API check what each panel shows and which action it calls with what payload, including Reject needing a comment
