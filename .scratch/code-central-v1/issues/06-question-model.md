# How agents ask and the human answers

Type: grilling
Status: resolved
Blocked by: 05, 18

## Question

What is a Question, as distinct from a comment? What structure does it carry (rounds, recommended answers, options), can it sit on a Spec as well as a Ticket, and how does an agent learn its Questions have been answered and resume, given that The State workflow returns an answered Ticket to `Ready` for any agent to claim? What must a Ticket hold so that a new agent can continue from the Questions and answers alone?

## Answer

Amended by [How agents claim work](09-claiming-work.md), which records the session ID at the claim as well as with each Round, and proves the holder with a Claim ID instead of the session ID.

A Question is a first-class record on a Ticket, and it always stops the work. A comment is free text with no workflow effect. Only an agent holding a Ticket can post Questions; an agent triaging a `Draft` Ticket leaves a comment instead. A human who wants to tell an agent something writes a comment or moves the Ticket.

Questions sit only on Tickets, because a Spec has no stored State for `Needs Answer` to live in. The Spec view can show its Tickets' Questions as a read-only roll-up, and a grilling Ticket writes its outcome into the Spec when it finishes.

**Rounds.** Questions are posted in Rounds. A Round is stored and numbered on the Ticket and posted in one write, so a single Question is a Round of one. The Ticket stays in `Needs Answer` until every Question in the Round is answered. Answers are saved as they are given, so answering part of a Round wakes nobody.

**A Question** has a title, a body and an optional recommended answer. Any choices go in the body as prose, which matches the grilling skill's format, so there is no options field. It is open, answered or dismissed.

**An answer** is free text. The UI pre-fills the box with the recommended answer as a draft, and nothing is stored until the human submits that answer or uses "accept all recommended". Only the final text is stored; whether the recommendation was accepted is found by comparing the two. An answer can be edited until the Round is complete, and after that a correction is a comment.

**Moving a Ticket by hand.** If the human moves a Ticket out of `Needs Answer`, its open Questions become dismissed. They stay on record, so the next agent can see they were deliberately left unanswered.

**Continuing in the same session.** The session that asked carries on with its context, as Keeping an agent session going across Questions found possible. Every agent follows one rule: post the Round and end the turn. Only what wakes the session differs:

- `code-central wait <ticket>` is run in the background and exits when the Ticket is answered, which wakes a live session.
- The claim records the session ID, added to each Round by a hook set up with the repository. The UI shows a copyable `claude --resume <id>` on an answered Ticket, which covers a session that has exited, or a reboot.
- A Factory can use exit and resume. Code Central still never starts an agent.

**The claim survives the wait.** This amends The State workflow, which returned an answered Ticket to `Ready`. A new ninth State, `Answered`, means every Question is answered and the asking session has not yet picked the work up. It keeps the claim and counts as Needs Me. A live session leaves it within moments, so it only stays visible when a session needs resuming or has been lost.

| From | To | Who | When |
| --- | --- | --- | --- |
| `Needs Answer` | `Answered` | Automatic | Last open Question is answered |
| `Answered` | `In Progress` | Agent | The claim holder picks up the answers |
| `Answered` | `Ready` | Human | Releases the claim, so a new agent can take it |

**If the session is lost.** The human releases the claim, and a new agent claims the Ticket from `Ready` and continues from the Ticket, its Questions and their answers. A session that wakes after being released gets a rejected move and stops. No timeout is needed here.

**The grilling skill stays unchanged.** Its ask, wait and repeat loop needs no knowledge of Code Central. The instructions a repository is set up with say that when a Ticket is being worked, a Round is posted to the Ticket rather than printed in the terminal. The skill's final confirmation is an ordinary one-Question Round.

Left out of v1: a required Handoff note with each Round, because it costs tokens that aren't needed yet, and MCP tasks as a wake mechanism, because they are a flag-controlled beta that would be hand-written in Go.

Considered and rejected: returning an answered Ticket to `Ready` with a new agent each Round, because the session's context is lost. Moving it back to `In Progress`, because `In Progress` with nobody awake looks like work happening, which is what ADR 0001 exists to prevent. A blocking tool call, because it fails on client time limits. An options field, because the pre-filled answer does the same job. A Code Central variant of the grilling skill, because it would drift from the original.

Not verified: real waits were tested for about 6.5 minutes at most, so waits of hours or days, laptop sleep, reboots and Docker have not been run. Context size across many Rounds and the cost of re-caching after a late answer are a known concern, to be judged in use.
