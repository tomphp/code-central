# Guide: prototype

You hold a Claim on a Prototype: a cheap, throwaway artifact for the human to try and react to, so a decision can be made. This Ticket's description holds the question it answers; its Spec holds the context. `get_stage_guide` with stage `prototype` serves this guide again if you lose it.

## What this Ticket produces

The artifact, committed on this Ticket's prototype branch, which is never pushed and never lands, and what the human learned from it:

- Build the smallest thing that answers the question, and commit it on the prototype branch you were given. Don't push it, and don't open a pull request.
- `set_tags` on this Ticket: `prototype` with the value `<branch>@<commit> <worktree path>`, the branch, its head commit and the folder it is checked out in.
- `post_round` with "Try it" as the first Question: exact instructions in its body (what to run, where to look, what to try). Then one Question for each thing to react to, each with your recommended answer. The answers are the feedback; iterate with further commits and Rounds as needed.
- **A resolution comment.** Post it on this Ticket with `comment`: what was learned, and which decision it informs.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews what was learned when you finish, instead of checking with the user along the way. So a Round is for trying the artifact and reacting to it, never to approve it.

## Finishing

When the resolution comment is posted, `finish` with a summary that points at it. Give no `branch`: a Prototype never lands, so accepting it makes the Ticket Done. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: their commits on the prototype branch and the human's answers are your starting point, so carry on from them rather than starting again. Update the `prototype` Tag and post a new resolution comment. In the new finish summary, answer each point of the rejection or note.
