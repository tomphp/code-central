# Importing a `.scratch/` folder

Read the repository's whole `.scratch/` folder yourself and map it into one `import` payload. The service never reads the repository: it checks your payload strictly and writes all of it or none of it. The Project must already exist (`code-central init`).

## Specs

One Spec per feature folder (`.scratch/<feature>/`):

- Its body is `spec.md`; or a wayfinder `map.md`, verbatim; or both, `spec.md` then `map.md`.
- A folder with neither gets a Spec titled after the folder, with an empty body.
- `source` is the file's path from the repository root, such as `.scratch/login/spec.md`, or the folder's path, such as `.scratch/login/`, when it has neither.

## Tickets

Each issue file (and each decision ticket of a map) becomes one Ticket on its folder's Spec. `spec` is that Spec's `source`. `source` is the file's path from the repository root, such as `.scratch/login/issues/03-api.md`.

| Scratch State line | `state` |
| --- | --- |
| `resolved`, `done`, `closed` | `Done` |
| `ready-for-agent` | `Ready` |
| `Superseded by N`, `Broken into N, M` | `Cancelled`, with `replacements` |
| `wontfix` | `Cancelled`, no Replacements, the reason as `comment` |
| `Deferred` | `Draft`, the reason as `comment` |
| anything else, or missing | `Draft` |

Nothing comes in as In Progress, Needs Answer, Answered, Failed or In Review, and no Claims are imported.

`blocked_by` and `replacements` name other Tickets by `source`, in this payload only. When a Blocker names something that doesn't exist, drop the link (its text stays in the description) and list it as a doubt.

## Labels and Tags

- `Label:` and `Labels:` header values become `labels`, split on commas. A map's `Label: wayfinder:map` becomes a Label on its Spec. Reserved Labels: `stage:grilling`, `stage:spec` and `stage:tickets`; `wayfinder:map` (Specs only); `wayfinder:chart`, `wayfinder:grilling`, `wayfinder:research`, `wayfinder:task` and `prototype` (Tickets only). A Ticket carries at most one of `stage:*`, `wayfinder:*` and `prototype`. List any other `stage:*` or `wayfinder:*` value, or a reserved Label out of place, as a doubt and drop it.
- Every other `Key: value` header line becomes a Tag with its key lowercased, such as `type: grilling`.
- Keep the original State line as the Tag `scratch.status`, such as `scratch.status: Superseded by 115`.
- Don't give a `source` Tag: the service sets it from `source`.
- Custom bullet fields: a Tag if clearly a key and a value, otherwise leave them in the description.
- Tag keys are lowercase `[a-z0-9._-]`; each value is non-empty and single-line.

## Descriptions

Remove the header lines that became the State, Blockers, Labels or Tags. Keep everything else verbatim: `## Answer`, `## Comments`, `## Triage Notes`, pointers to research branches. Don't rewrite references to other files or numbers in bodies; the `source` Tag traces each item back.

## Order

List Specs by folder and each folder's Tickets by file number. IDs come from the Project's counter in that order: each Spec, then its Tickets.

## Payload

```json
{"payload": {
  "specs": [{"source": ".scratch/login/spec.md", "title": "Login", "body": "...", "labels": [], "tags": {}}],
  "tickets": [{"source": ".scratch/login/issues/01-form.md", "spec": ".scratch/login/spec.md",
    "title": "Form", "state": "Ready", "description": "...", "labels": [], "tags": {"scratch.status": "ready-for-agent"},
    "blocked_by": [], "replacements": [], "comment": ""}]
}, "dry_run": true}
```

## Steps

1. Call `import` with `dry_run: true`. It runs every check and writes nothing. A fault returns `invalid` with a `field` path such as `tickets[12].blocked_by[0]`; a loop of Blockers returns `cycle`. Fix the payload and try again.
2. Present the plan to the human: each source with its provisional ID, title, State, Labels, Tags and Blockers, grouped by Spec, with the count per State.
3. List your doubts: any Ticket whose State disagrees with its body (a `Ready` one with every box ticked or an answer present, a `resolved` one with every box unticked), and every Blocker you dropped. The human picks the State for each.
4. Wait for the human's go-ahead. Then call `import` again without `dry_run`.

An Import runs once: it is refused if any `source` is already in the Project. Afterwards, recommend deleting `.scratch/` and committing that.
