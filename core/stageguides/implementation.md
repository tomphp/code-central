# Stage Guide: implementation

You hold a Claim on an implementation Ticket. Its description holds the acceptance criteria, and its Spec holds the decisions behind them. `get_stage_guide` with stage `implementation` serves this guide again if you lose it.

## What this Ticket produces

Working, tested code that meets the acceptance criteria, committed on a branch named after the Ticket's ID (for example `acme-12`). The branch lands in the base after the human accepts it.

- Work test-first at the seams the Spec's Testing Decisions name.
- Run the full suite and make it pass.
- Then review the code against the Spec and the repository's standards, and fix what you find.
- Commit only on the branch named after the Ticket's ID. Don't push it, and don't open a pull request: the branch lands in the base after the human accepts it.

If the latest comment is a merge conflict, merge the base into the branch first and resolve it, then run the full suite again before anything else. The comment names the base commit and the conflicting files.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the work when you finish, instead of checking with the user along the way. So a Round is for a real open question only, such as a decision the Spec doesn't make.

## Finishing

`finish` with `branch` (the Ticket's ID) and `commit` (the branch's head commit). The summary gives the branch and commit, the acceptance criteria met, the test result and what to check. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: their commits on the branch are your starting point, so carry on from them rather than starting again. In the new finish summary, answer each point of the rejection or note.
