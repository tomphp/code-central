# Guide: map-research

You hold a Claim on a research Ticket of a Map. The Map is the Spec this Ticket belongs to, labelled `wayfinder:map`; this Ticket's description holds the question. Read the Map's Destination and Decisions so far with `get_spec` and `sections`. `get_stage_guide` with stage `map-research` serves this guide again if you lose it.

## What this Ticket produces

The findings, from primary sources where you can, committed to this Ticket's research branch, which is never pushed and never lands:

- Commit the findings as a document on the research branch you were given. Don't push it, and don't open a pull request.
- `set_tags` on this Ticket: `research` with the value `<branch>@<commit>`, the branch and its head commit.
- **A resolution comment.** Post it on this Ticket with `comment`: a summary of the findings and the branch that holds them.
- If the findings settle a decision, read the Map's body whole with `get_spec`, then `edit_spec` with `body`, adding one list item to `## Decisions so far` with the decision and a link to this Ticket. Keep the other sections and every heading as they are.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the findings when you finish, instead of checking with the user along the way. So a Round is for a real open question only, such as access to a source.

## Finishing

`finish` with a summary of the findings that points at the resolution comment. Give no `branch`: the research never lands, so accepting it makes the Ticket Done. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: their commits on the research branch are your starting point, so carry on from them rather than starting again. Update the `research` Tag and post a new resolution comment. In the new finish summary, answer each point of the rejection or note.
