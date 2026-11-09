# Stage Guide: spec

You hold a Claim on the spec Stage of a Requirement. The Requirement is the Spec this Ticket belongs to. `get_stage_guide` with stage `spec` serves this guide again if you lose it.

## What this Ticket produces

The Spec's body, written in full. Replace it with `edit_spec`, giving `body`, in this template, with these sections in this order:

1. Problem Statement
2. Solution
3. User Stories
4. Implementation Decisions
5. Testing Decisions
6. Out of Scope
7. Further Notes

Work from the Requirement's text (the Spec's body as the human submitted it) and the grilling Ticket's Rounds and resolution. Read that Ticket's resolution comment with `get_ticket` and `parts` `comments`, and its Rounds (`parts` `rounds`) only when the comment points to one. A Requirement with no grilling Stage whose `map` Tag names a Map was written from it: work from the Map's Decisions so far and the resolution comments they link instead.

Carry every decision, new term and ADR from the resolution into the Spec, so the tickets Stage can assign writing them. Testing Decisions name the seams the tests go through.

Create no Spec and add no Label: the Spec already exists, and its Tickets carry the Stages. Keep the Spec's title unless the work has plainly outgrown it.

A Stage never changes repository files and pushes no branch. Read the repository as much as you need, but write nothing to it.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the Spec when you finish, instead of checking with the user along the way. So a Round is for a real open question only, such as a gap the grilling left, never to approve a draft.

## Finishing

`finish` with a summary that lists the test seams, the key decisions and what is out of scope. Give no `branch` or `commit`: a Stage has none. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: the Spec's body as they wrote it is your draft, so edit it rather than starting again. In the new finish summary, answer each point of the rejection or note.
