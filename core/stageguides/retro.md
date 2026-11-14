# Guide: retro

You hold a Claim on a Retro: a look back over the sessions that delivered a Finished Ticket or Spec, to propose changes to the agents' environment. This Ticket's description holds the target, the human's note and the Claims to read. `get_stage_guide` with stage `retro` serves this guide again if you lose it.

## What this Ticket produces

A Round of improvement candidates, a Ticket for each one the human picks, and a resolution comment. Nothing is committed:

- Read this Ticket's description: its target, the human's note and the Claims to read. For each Ticket named there, read its record with `get_ticket` (its comments, Failure Reports, and Rounds when a comment points to one).
- Read each listed Claim's session, including the subagents that session started. Name every Claim whose session couldn't be read, then carry on. If no listed Claim's session can be read, `report_failure`.
- Trace each improvement candidate to a moment in a session: the Ticket, the Claim and what happened.
- `post_round` with one Question per candidate, most severe first. Each Question's title is the candidate in short. Its body gives the category, the session moment and the proposed fix, and its recommended answer is "Create it". If there are no candidates, skip the Round, and the resolution comment says so.
- After `pick_up_answers`, for each candidate the human picked, `create_ticket` on this Retro's own Spec (the Retros Spec) with State Ready, no Labels, and a description that states the fix, takes in the human's note on the answer, and links back to this Retro.
- **A resolution comment.** Post it on this Ticket with `comment`: every candidate, picked or not, and the Ticket created for each one picked.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the Retro when you finish, instead of checking with the user along the way. So a Round is for choosing which candidates become Tickets, never to approve the Retro.

## Finishing

When the resolution comment is posted, `finish` with a summary that points at it. Give no `branch`: a Retro never lands, so accepting it makes the Ticket Done. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. The human's answers to the earlier Round stand: offer again only the candidates that have no Ticket yet, and don't create a second Ticket for one that has. Then build on what the earlier Claims left: their Round, the human's answers and the Tickets they created are your starting point, so carry on from them rather than starting again. Post a new resolution comment. In the new finish summary, answer each point of the rejection or note.
