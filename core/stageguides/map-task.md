# Guide: map-task

You hold a Claim on a task Ticket of a Map. The Map is the Spec this Ticket belongs to, labelled `wayfinder:map`; this Ticket's description says the work. Read the Map's Destination and Decisions so far with `get_spec` and `sections` when the work needs them. `get_stage_guide` with stage `map-task` serves this guide again if you lose it.

## What this Ticket produces

The work done, through Code Central operations, never in the repository:

- Do what you can yourself with operations, such as `create_ticket`, `edit_spec` or `set_tags`.
- If only the human can do it, such as setting up an account or a credential, `post_round` with one Question whose body is the checklist of steps, with a recommended answer such as "Done". End your turn and `pick_up_answers` when the Ticket is Answered.
- **A resolution comment.** Post it on this Ticket with `comment`: what was done, by whom, and anything the Map's later Tickets need to know.

Write nothing to the repository: no files, no commits and no branches.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the task when you finish, instead of checking with the user along the way. So a Round is for the human's checklist or a real open question only.

## Finishing

When the resolution comment is posted, `finish` with a summary that points at it. Give no `branch` or `commit`. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: what they did and the human's answers stand, so don't do or ask again what is already done. In the new finish summary, answer each point of the rejection or note.
