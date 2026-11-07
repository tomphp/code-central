# 35: Resume-then-Release panel

**What to build:** For a Ticket that is `Answered` but not picked up, or a Silent Claim, the human sees how long it has waited or been quiet, copies a `claude --resume <id>` command to bring the session back, or releases the Claim knowing what that costs. See the [spec](../spec.md), section "UI".

**Blocked by:** 31, 33

**Status:** resolved

- [x] One shared panel serves both `Answered` and Silent Claim
- [x] It says how long the Ticket has waited or been quiet, then shows a copyable `claude --resume <id>`, or says no session ID is recorded
- [x] Release opens an optional note and the warning that a live session's next write will be rejected and the Claim can't be restored, then calls `release`
- [x] On an `Answered` Ticket the panel also shows the Round's answers, collapsed
- [x] The TypeScript API module and its fake gain `release`, covered by the contract suite
- [x] Component tests against the fake API check the resume command, the no-session case, the Release warning and the payload sent
