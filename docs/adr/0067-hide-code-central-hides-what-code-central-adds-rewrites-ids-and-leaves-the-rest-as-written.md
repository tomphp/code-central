# Hide Code Central hides what Code Central adds, rewrites IDs, and leaves the rest as written

[ADR 0055](0055-write-back-to-azure-boards-comments-and-tags-but-never-changes-a-work-items-state.md) has Write-back post comments to a work item and keep one `code-central:` tag. Some teams don't want their Azure DevOps work items to name Code Central, its Specs or its Tickets at all. **Hide Code Central** is a setting of Azure Boards, off by default, that keeps Write-back's comments but removes every trace of Code Central from what it writes.

While it is on, Write-back writes no `code-central:` tag, no line linking back, and no Ticket IDs in landed lines. Core does the hiding, so the Work Item Watcher only posts what it is given. Three choices shape it.

## Decision

- **The marker is an opaque hash.** Write-back posts each comment exactly once by putting a marker in it and skipping a comment whose marker is already on the work item. Under Hide Code Central the marker is a hash that names nothing. Considered and rejected: no marker, which loses exactly-once posting, and matching on the comment's text, which breaks the moment a comment is edited or the text changes between versions. A comment whose branded marker is already on the work item is skipped too, and the reverse, so turning the setting on or off never double-posts.
- **This Project's IDs are rewritten to titles in written text; other prose passes through.** A Spec's body and a Requirement's resolution name Specs and Tickets by ID, or as ID Links. Those are replaced with the item's title, outside code, so the text still reads. Other keys, numbers that name nothing and words that merely contain an ID are left as written, and so is the rest of the prose: Hide Code Central doesn't judge what the human wrote.
- **Existing tags and comments are left alone.** Turning the setting on changes only what is written from then on. Tags and comments already on a work item stay, as Write-back never edits or deletes what it posted.

## Consequences

- The Status label is not written under the setting, and a `code-central:` tag already there is neither replaced nor removed.
- Hide Code Central is no part of the Azure Boards check: changing it alone runs no check and leaves the Work Item Watcher's status alone.
- GitHub Issues has no such setting.
- A human who wants earlier traces gone removes them in Azure DevOps.
