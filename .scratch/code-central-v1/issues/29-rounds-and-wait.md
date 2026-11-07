# 29: Rounds and `wait`

**What to build:** An agent holding a Claim asks the human a Round of Questions and ends its turn; the human answers over the HTTP API; the agent's background `code-central wait` exits and the same session picks the answers up and carries on, keeping its Claim throughout. This is the end-to-end agent loop, proved through the real binary. See the [spec](../spec.md), section "Rounds, Questions and Failure Reports".

**Blocked by:** 24, 25

**Status:** resolved

- [x] `post_round(ticket, claim_id, session_id?, questions: [{title, body, recommended?}])` stores a numbered Round in one write, moves `In Progress` to `Needs Answer`, and returns the Round number with the next step: "End your turn now. If you can run background commands, run `code-central wait <id>` in the background first."
- [x] Each Question is open, answered or dismissed; Questions sit only on Tickets
- [x] Human `answer(question, text)` saves one answer at a time, editable until the Round is complete; `accept_recommended(round)` answers every open Question with its recommendation; `dismiss_question(question)` dismisses one
- [x] When the last open Question is answered or dismissed, the Ticket moves automatically to `Answered`, keeping the Claim; answering part of a Round moves nothing
- [x] `pick_up_answers(ticket, claim_id)` moves `Answered` to `In Progress` in one step and returns the latest Round with every answer and dismissal, plus comments added since it was posted
- [x] A human move out of `Needs Answer` dismisses its open Questions and ends the Claim
- [x] An `events` package carries change notices from `core`
- [x] `GET /api/p/<key>/tickets/<n>/wait?from=<State>` holds for up to 60 seconds (on the injected clock) and returns the State, at once if the Ticket has already left that State
- [x] `code-central wait <id>` loops on the long-poll, retries through connection errors, and exits `0` when `Answered` (printing JSON that says to call `pick_up_answers`), `3` when the Ticket left for another State (printing the State and the last move), and `1` on a usage error; it takes no Claim ID
- [x] `wait`'s exit codes and retries are tested in the CLI package against `codecentraltest` with a fake clock
- [x] End-to-end test 1, behind the `e2e` build tag and through the real binary: `init`, an MCP `claim` and `post_round`, the human answers over HTTP, `wait` exits `0`, then `pick_up_answers`, `finish` and `accept`
- [x] `make e2e` builds the binary and runs the end-to-end tier
