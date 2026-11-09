# Importing `.scratch/` into a Project

This guide turns the specs and tickets in a repository's `.scratch/` folder into Specs and Tickets in its Project. It continues `acme-api` from [Setting up a Project](setting-up-a-project.md), whose `.scratch/` folder holds one spec and two tickets from planning. The output shown is copied from a real run, cut with `…` where it is long.

An Import is one-way and runs once: afterwards Code Central owns the work, and you delete `.scratch/`. There are two ways to do it, and you need only one:

- [Ask Claude Code](#by-asking-claude-code) to do it. The agent reads `.scratch/`, maps it as `get_import_guide` tells it, and shows you the plan before it writes anything.
- [Use the CLI](#by-the-cli): you write the Import's JSON yourself and run `code-central import`.

## Before you start

1. **Set the repository up as a Project.** Follow [An existing repository](setting-up-a-project.md#an-existing-repository), up to an agent that can reach the Project. init leaves `.scratch/` alone.

2. **Look at what you're importing.** `acme-api`'s `.scratch/` has one feature folder, `rate-limits`, with its spec and two tickets:

   ```console
   $ cd ~/src/acme-api
   $ find .scratch -type f
   .scratch/rate-limits/spec.md
   .scratch/rate-limits/issues/01-token-bucket.md
   .scratch/rate-limits/issues/02-configurable-limit.md
   $ cat .scratch/rate-limits/issues/02-configurable-limit.md
   # Read the limit from the environment

   Status: needs-triage
   Blocked by: 01-token-bucket.md

   ## What to build
   …
   ```

   Each feature folder becomes a Spec, and each file in its `issues/` becomes a Ticket on that Spec. A ticket's `Status:` line sets its State, its `Blocked by:` line its Blockers, its `Labels:` line its Labels, and any other `Key: value` line a Tag.

## By asking Claude Code

1. **Ask for the Import.** Start `claude` in `acme-api` and ask:

   ```text
   > Import .scratch/ into Code Central.
   ```

   The agent calls `get_import_guide`, which tells it how to map a `.scratch/` folder, reads every file in `.scratch/`, and calls `import` with `dry_run`: the service checks the whole Import and writes nothing. Claude Code asks before each tool it hasn't used yet; allow them. The agent then shows you the plan, with the ID each Spec and Ticket will get (Claude Code renders its Markdown):

   ```markdown
   I ran a dry run of the import and it passed with no errors. Nothing has been written to Code Central yet. Here's what it would create:

   **Spec acme-api-1: "Rate limits"** (from `.scratch/rate-limits/spec.md`, body copied word for word)

   | ID | Source | Title | State | Labels | Tags | Blocked by |
   |---|---|---|---|---|---|---|
   | acme-api-2 | `issues/01-token-bucket.md` | Token bucket per API key | Ready | `backend` | `scratch.status: ready-for-agent` | — |
   | acme-api-3 | `issues/02-configurable-limit.md` | Read the limit from the environment | Draft | — | `scratch.status: needs-triage` | acme-api-2 |

   **Counts:** 1 Ready, 1 Draft.

   **Doubts:** None. Both tickets' states match their bodies: every acceptance box is unticked and neither has an answer. The one blocker link points to a ticket in this import, so nothing was dropped. `needs-triage` comes in as Draft, following the import guide.

   Should I go ahead with the real import? Once it's done, I'd suggest deleting `.scratch/` in a separate commit.
   ```

   `ready-for-agent` comes in as Ready, so an agent can claim it straight away. Any other status that `get_import_guide` doesn't map comes in as Draft, for you to approve when it's ready to be worked. The original line is kept as the Tag `scratch.status`.

   Check: nothing has been written yet. The Project has no Tickets:

   ```console
   $ code-central find-tickets
   []
   ```

2. **Check the plan, then go ahead.** Read the doubts first: the agent lists any ticket whose State disagrees with its body (say, Ready with every box ticked) and any Blocker it had to drop, and you pick the State for each. Tell it what to change, or tell it to go ahead:

   ```text
   > Go ahead.
   ```

   It calls `import` again without `dry_run`, and the Import is written all at once:

   ```markdown
   The import is done. Code Central now has the same records the dry run showed:

   - **acme-api-1** is the "Rate limits" spec.
   - **acme-api-2**, "Token bucket per API key", is Ready and labelled `backend`.
   - **acme-api-3**, "Read the limit from the environment", is a Draft blocked by acme-api-2.

   Next, I'd delete `.scratch/` and commit that on a branch, so Code Central is the only place these specs and tickets live. …
   ```

3. **Check the Import in the UI.** Open `http://localhost:7733/?project=acme-api`. In the left pane, `acme-api` shows **Draft 1** and **Ready 1**.

   - Choose **Ready**: `acme-api-2 Token bucket per API key` is listed. Select it: under its title it names its Spec, `acme-api-1 Rate limits`, and the Label `backend`, with the ticket's description below.
   - Choose **Draft**: `acme-api-3 Read the limit from the environment` is listed. Select it: below its description it says `Blocked by acme-api-2 Token bucket per API key (Ready)`.

   **Requirements** stays empty: an imported Spec isn't a Requirement, as it didn't start from your free text and go through the Stages. Its Tickets are worked like any others.

   The Tags don't show in the UI. `code-central get-ticket` shows them, with `source`, which traces each Ticket back to its file:

   ```console
   $ code-central get-ticket acme-api-3
   {
     "id": "acme-api-3",
     "title": "Read the limit from the environment",
     "state": "Draft",
     …
     "tags": {
       "scratch.status": "needs-triage",
       "source": ".scratch/rate-limits/issues/02-configurable-limit.md"
     },
     "blocked_by": [
       "acme-api-2"
     ],
     …
   ```

4. **Delete `.scratch/` and commit that,** so the Project is the one place this work lives. Let the agent do it, or do it yourself:

   ```console
   $ git rm -rq .scratch
   $ git commit -m "Move .scratch/ into Code Central"
   [main 3358835] Move .scratch/ into Code Central
    3 files changed, 40 deletions(-)
   …
   ```

`acme-api`'s planned work is now in its Project. `acme-api-2` is Ready now. Approve `acme-api-3` when it's ready to be worked: in the UI, move it to Ready.

## By the CLI

`code-central import` takes the same Import as JSON, from a file or stdin. You map `.scratch/` yourself, by the same rules the agent follows. These steps start again from `acme-api` before any Import: an Import is refused if any of its files is already in the Project (see [Importing twice](#importing-twice)).

1. **Read the rules.** `code-central import-guide` prints what `get_import_guide` gives an agent, as JSON. `jq -r .guide` prints it as Markdown:

   ```console
   $ code-central import-guide | jq -r .guide
   # Importing a `.scratch/` folder

   Read the repository's whole `.scratch/` folder yourself and map it into one `import` payload. The service never reads the repository: it checks your payload strictly and writes all of it or none of it. The Project must already exist (`code-central init`).
   …
   ```

   It covers which State each status becomes, how Labels, Tags and Blockers are written, and the order IDs are given in.

2. **Write the Import.** Put it in a file outside the repository, here `~/acme-api-import.json`: `specs` with one Spec per feature folder, and `tickets` with every ticket. `source` is each file's path from the repository root, and `spec` and `blocked_by` name other files by their `source`. Take out the header lines that became the State, Blockers, Labels or Tags; keep the rest of each file as it is.

   ```json
   {
     "specs": [
       {
         "source": ".scratch/rate-limits/spec.md",
         "title": "Rate limits",
         "body": "# Rate limits\n\n## Problem Statement\n\nOne client can flood the API …"
       }
     ],
     "tickets": [
       {
         "source": ".scratch/rate-limits/issues/01-token-bucket.md",
         "spec": ".scratch/rate-limits/spec.md",
         "title": "Token bucket per API key",
         "state": "Ready",
         "description": "## What to build\n\nA middleware that keeps a token bucket per API key …",
         "labels": ["backend"],
         "tags": {"scratch.status": "ready-for-agent"}
       },
       {
         "source": ".scratch/rate-limits/issues/02-configurable-limit.md",
         "spec": ".scratch/rate-limits/spec.md",
         "title": "Read the limit from the environment",
         "state": "Draft",
         "description": "## What to build\n\nRead the limit from `RATE_LIMIT_PER_MINUTE`, defaulting to 100. …",
         "tags": {"scratch.status": "needs-triage"},
         "blocked_by": [".scratch/rate-limits/issues/01-token-bucket.md"]
       }
     ]
   }
   ```

   Check: `jq empty ~/acme-api-import.json` prints nothing, so the file is valid JSON. The next step checks the rest.

3. **Preview it with `--dry-run`.** The service checks the whole Import and writes nothing. It prints the plan, with the ID each Spec and Ticket will get and the count per State:

   ```console
   $ code-central import --dry-run --file ~/acme-api-import.json
   {
     "dry_run": true,
     "specs": [
       {
         "source": ".scratch/rate-limits/spec.md",
         "id": "acme-api-1",
         "title": "Rate limits",
         …
         "tickets": [
           {
             "source": ".scratch/rate-limits/issues/01-token-bucket.md",
             "id": "acme-api-2",
             "title": "Token bucket per API key",
             "state": "Ready",
             "labels": [
               "backend"
             ],
             …
           },
           {
             "source": ".scratch/rate-limits/issues/02-configurable-limit.md",
             "id": "acme-api-3",
             "title": "Read the limit from the environment",
             "state": "Draft",
             …
             "blocked_by": [
               "acme-api-2"
             ],
             …
     "state_counts": {
       "Draft": 1,
       "Ready": 1
     },
     …
   }
   ```

   A mistake comes back as `invalid`, naming the field, and the command exits 2. Here a `blocked_by` is missing its `.md`:

   ```console
   $ code-central import --dry-run --file ~/acme-api-import.json
   {"code":"invalid","message":"\".scratch/rate-limits/issues/01-token-bucket\" is not the source of a Ticket in this payload; drop the link and list it as a doubt","field":"tickets[1].blocked_by[0]"}
   ```

   Fix the file and run the dry run again until it prints the plan.

4. **Import it.** Run the same command without `--dry-run`. The output is the same plan, now written, with `"dry_run": false`:

   ```console
   $ code-central import --file ~/acme-api-import.json
   {
     "dry_run": false,
     "specs": [
       {
         "source": ".scratch/rate-limits/spec.md",
         "id": "acme-api-1",
         …
     "next_step": "Delete `.scratch/` and commit that, so Code Central is the one source of truth for this work."
   }
   ```

   Check: the Tickets are in the Project.

   ```console
   $ code-central find-tickets
   [
     {
       "id": "acme-api-2",
       "title": "Token bucket per API key",
       "state": "Ready",
       "spec": "acme-api-1",
       …
     },
     {
       "id": "acme-api-3",
       "title": "Read the limit from the environment",
       "state": "Draft",
       "spec": "acme-api-1",
       "labels": [],
       "blocked": true,
       …
     }
   ]
   ```

   They show in the UI as in step 3 of [By asking Claude Code](#by-asking-claude-code).

5. **Delete `.scratch/` and commit that**, as in step 4 of [By asking Claude Code](#by-asking-claude-code), and delete `~/acme-api-import.json`. Check: `git status --short` prints nothing, and `.scratch/` is gone.

## Importing twice

An Import runs once. If any `source` in it is already in the Project, it is refused whole, dry run or not, and nothing is written:

```console
$ code-central import --file ~/acme-api-import.json
{"code":"invalid","message":"this Project already has 3 of these sources, so this Import has run before; nothing was imported: .scratch/rate-limits/spec.md (acme-api-1), .scratch/rate-limits/issues/01-token-bucket.md (acme-api-2), .scratch/rate-limits/issues/02-configurable-limit.md (acme-api-3)","field":"specs[0].source"}
```

To change something you've imported, edit the Spec or Ticket in Code Central.
