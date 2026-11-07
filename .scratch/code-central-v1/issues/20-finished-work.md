# How finished work is kept and hidden

Type: grilling
Status: resolved

## Question

What happens to finished work over months of use? Are `Done` and `Cancelled` Tickets, finished Specs and Projects kept in the store for good, given that [the store](11-store-format.md) is loaded whole into memory? Is there an archive, and is it a flag, a folder or derived from age? How do finished items appear in the UI, in `find_tickets` (which already leaves out `Done` and `Cancelled`) and in `find_specs`? [How a Project is identified](08-project-identity.md) left Project archiving here, with no way to delete a Project in v1.

Each Ticket already records its `moves`, `claims`, `rounds`, `failure_reports` and `comments`. Is anything more needed for the audit trail, such as a record of edits to titles, descriptions and Spec bodies, which are replaced whole?

## Answer

Nothing is ever removed in v1. Finished work stays in place and loaded, it is hidden by being computed from State, and the only thing that can be archived is a Project.

**Keeping.** Every Spec and Ticket file stays where it is and is loaded into memory for good. There is no archive folder and nothing expires by age. Moving files out would break the derived ID counter (a number could be reused) and the reading of Replacements, and one user's work is small: about 5–20 KB per Ticket. A startup test measures load time against a generated store of about 5,000 Tickets, so slowness shows up before it hurts.

**Finished is derived.** A Ticket is finished when it is `Done` or `Cancelled`. A Spec is finished when it has at least one Ticket and all of them are finished. A Spec with no Tickets still shows as `Draft`. Specs and Tickets have no archive flag.

**Archived Project.** This is the human-only `archive_project`, which sets `archived: <time>` in `project.yaml`, with a matching `unarchive_project`.
- It is refused while any Ticket in the Project is unfinished, so archiving can never hide something that needs the human.
- An Archived Project refuses every write, from agents and the human alike, including `create_ticket`, `create_spec` and `import`. Each refusal uses the usual error shape, listing `unarchive_project`. Reads, including `GET` by URL, keep working.
- `init` on its repository says it is archived instead of quietly bringing it back.
- In the UI it moves into a collapsed "Archived" group at the bottom of the left pane. Archive and Unarchive sit in each Project row's menu, and Archive is disabled, with the reason given, while unfinished work remains.

**Finding finished work.**
- `find_tickets` is unchanged: `Done` and `Cancelled` are left out unless the State filter names them.
- `find_specs` leaves out finished Specs unless `finished: true` is passed, and its rows include the derived State summary.
- In the UI, finished Tickets show when the View list is filtered to `Done` or `Cancelled`, newest first, with plain counts. Any Ticket also opens by its URL. There is no Spec browser or search across finished work in v1.

**Edit history.** Each `edit_ticket` or `edit_spec` call that changes something adds one numbered entry to an `edits` list: its number, the Actor, the time, and `previous`, which holds the old value of each changed field. An edit that changes nothing records nothing. Integration updates to linked definitions go through the same path with the `integration` Actor. Changes to Labels, Tags and Project names aren't recorded in v1.
- A Ticket's `edits` sit in its YAML file with its other lists.
- A Spec's `edits` sit in its frontmatter, with old bodies as block scalars, so an edit still writes exactly one file. Keeping one file per write was judged worth the longer file.
- The UI shows edits in a Ticket's collapsed history ("description edited by <Actor>"), each with a "show previous" toggle. There is no restore action: the human copies the old text back by editing. A Spec's edits are reachable only through the HTTP read until a Spec view exists.

Considered and rejected:
- **An archive folder that isn't loaded:** it breaks the derived counter and Replacement reads, and the memory saved isn't needed.
- **Hiding or deleting by age:** finished is already computed from State.
- **Archiving a Project with open work:** it would hide things that need the human.
- **Recording no edits, or edits without the old text:** an agent's whole-body rewrite of a human-written Spec could not be undone, because the store isn't in git.
- **A sibling history file for Specs:** each edit would write two files.
- **Turning Specs into YAML:** the body would no longer be plain Markdown.
- **Counting only the last 30 days of finished work:** an extra rule for little gain.
