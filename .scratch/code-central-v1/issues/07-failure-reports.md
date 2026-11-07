# What a failure report contains

Type: grilling
Status: resolved
Blocked by: 05

## Question

When work fails, what must an agent record so the UI can show what is broken, why, and what needs fixing? The State workflow has the human send a `Failed` Ticket back to `Ready`: what else, if anything, does resolving a failure need, and how does the next agent pick up the work?

## Answer

A Failure Report is a first-class record on a Ticket, posted when an agent has no way forward. Only the agent holding the claim can post one, and only from `In Progress`.

**When an agent fails.** An agent that hits trouble has three exits, and these tests are published with the workflow:

- **Question:** it can say what it needs, and the human providing that would let this session carry on, such as a missing API key.
- **`Failed`:** it can't name a way forward, or nothing the human could say would let this session continue, such as a build broken by something outside the Ticket, an unreachable network, or tests still failing after it has run out of ideas.
- **Clean give-up (`In Progress` to `Ready`):** nothing is wrong with the work and nothing needs the human, but this session can't finish, such as when it is out of context or shutting down.

**A Failure Report** has four required Markdown fields, and a report missing any of them is rejected, leaving the Ticket in `In Progress`:

- **What broke:** a short headline, used in the Needs Me list, plus the observed symptom.
- **Why:** the cause. If the agent doesn't know, it says so and gives its best guess; "cause unknown" is valid.
- **What to fix:** the action needed, and who should do it where that's clear.
- **Where the work stands:** the branch or commit, what's finished, and whether anything will be lost, or "nothing was changed". A session pushes or commits what it can first, and says so if it can't.

There is no category and no attachments. Logs go in as Markdown excerpts or are referenced by path.

**Reports are permanent.** Like Rounds, they are stored and numbered on the Ticket and can't be edited once posted. A correction is a comment. The UI shows the failure count.

**The claim ends at `Failed`.** The report keeps the session ID, so the UI can still show `claude --resume <id>` for investigating what happened. A resumed session has no special standing and claims from `Ready` like any other agent.

**Resolving a failure.** The human has three moves, shown beside the report. This amends The State workflow, which named only the first and last:

| From | To | Who | When |
| --- | --- | --- | --- |
| `Failed` | `Ready` | Human | Sends it back: the problem is dealt with, or worth retrying |
| `Failed` | `Draft` | Human | The Ticket needs rewriting before an agent tries again |
| `Failed` | `Cancelled` | Human | Abandons it |

Send back has an optional note, stored as a comment. No note means "try again, nothing has changed", and the published workflow says so, so an agent never assumes a fix was made. If the fix is separate work, the human creates a Ticket for it, makes it a Blocker and sends this one back. The failing agent doesn't create that Ticket: the report says what to fix, and the human decides how.

**`Failed` is the exception to "the human can move any Ticket to any State".** It can only be entered by posting a report, so a `Failed` Ticket always has one to show. A human who sees a problem writes a comment, or moves the Ticket to `Draft`, `Ready` or `Cancelled`.

**The next agent.** Claiming returns the same complete view as reading the Ticket: the Spec link, Questions and answers, every Failure Report in order, and comments, including any send-back note. The published workflow tells agents to act on the latest report and any note before starting. There is no special retry mode. The exact call belongs to What the API looks like.

**A clean give-up also says where the work stands.** This amends The State workflow: the move requires that note, stored as a comment, because the next agent starts cold there too. The Handoff note was left out of Rounds because the session carries on with its context, which doesn't apply to a session that is leaving.

A session that dies leaves no Failure Report and no note. That case belongs to How agents claim work.

Considered and rejected: a single free-text body, because nothing would guarantee all the fields or give the Needs Me list a headline. Overwriting the last report, because the next agent would repeat failed attempts. Failure categories, because nothing in v1 acts on them and agents would pick them inconsistently. Keeping the claim through `Failed`, because a session that has said it's stuck would hold a Ticket nothing would use. A required send-back note, because a one-off blip has nothing to say. Letting the human set `Failed` by hand, because the UI couldn't explain a `Failed` Ticket with no report.
