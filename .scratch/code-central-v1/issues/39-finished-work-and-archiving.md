# 39: Finished work and Archived Projects

**What to build:** The human puts away a Project once all its work is finished; it then refuses every write but stays readable, and `init` says so rather than quietly reviving it. Finished Tickets stay findable in the UI. See the [spec](../spec.md), sections "Projects" and "UI".

**Blocked by:** 24, 28, 33

**Status:** resolved

- [x] Human `archive_project` sets `archived: <time>` in `project.yaml`, and is refused while any Ticket in the Project is unfinished; `unarchive_project` clears it
- [x] An Archived Project refuses every write from any Actor, including `create_ticket`, `create_spec` and `import`, with the usual error shape listing `unarchive_project`; reads, including `GET` by URL, keep working
- [x] `init` on an archived Project's repository says it is archived instead of bringing it back
- [x] In the UI, Archived Projects sit in a collapsed "Archived" group at the bottom of the left pane; Archive and Unarchive sit in each Project row's menu, and Archive is disabled, with the reason given, while unfinished work remains
- [x] Filtering the View list to `Done` or `Cancelled` shows finished Tickets newest first with plain counts, and any Ticket opens by its URL
- [x] A startup test loads a generated store of about 5,000 Tickets so slowness shows up early
- [x] Archive rules are tested in `core`; the UI parts have component or layout tests against the fake API
