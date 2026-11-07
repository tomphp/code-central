# The Scratch format in the wild

Type: research
Status: resolved

## Question

What exactly does a Matt Pocock style `.scratch/` folder contain across versions of the skills: spec files, issue files, status and label lines, blocking lines, comments, wayfinder maps and answers? List every variant an Import must handle.

## Answer

A `.scratch/` folder has no schema, version marker or front matter: it is Markdown written from prose instructions, and its on-disk shape changed at each minor release from v1.1.0 to v1.3.0. Today's shape is `<slug>/{spec.md | map.md}` plus `issues/<NN>-<slug>.md`, with `Status:` and `Blocked by:` lines, but real folders depart from it in almost every field. Across 167 Tickets the State line has 3 syntaxes or is missing, and holds 7 kinds of value, only 3 of them in the skills' vocabularies. Blockers can be prose, ranges, wrapped lines or cross-folder references.

Findings: `research/scratch-format.md` on branch `research/scratch-format` (commit `985ea92`). Section 14 is the full variant checklist. The repo has no remote, so the branch is local only, as are the three before it.

Carry forward: State is often stale, because no skill sets a finished State on an implementation Ticket (all 14 `ready-for-agent` Tickets have every box ticked, while all 5 `pocock-builder:review` Tickets say `resolved` with every box unticked), so an Import can't take it at face value. State values also carry meaning that needs a home: `Superseded by 115`, `Broken into 030, 031, 032`, and `Deferred` with a reason. Ticket numbers are per folder in some repos and repo-wide in others, with no marker saying which. Answers can point at research branches that may not exist at Import time.

Not verified: `PRD.md`, root `tickets.md`, `## Parent` and `## Blocked by` sections, `needs-info`, `wontfix`, `ready-for-human`, real `claimed`, `## Triage Notes`, `## Agent Brief`, the single-file map and `## Fog` have no real example and rest on templates. The corpus has one author, and `family-rewards` supplies 117 of the 167 Tickets in a form matching no template found. Public repos were read through READMEs, tracker docs and issue pages only, not raw `.scratch/` files.
