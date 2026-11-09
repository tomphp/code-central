# Working with Claude Code

This guide takes one Requirement through every Stage by hand, in a Claude Code session you start yourself, from submitting it to seeing its implementation land. It continues from [Setting up a Project](setting-up-a-project.md), on `~/src/acme-shop`, and ends with how to grill in the terminal without claiming, so you can choose between the two ways. The prompts and output shown are copied from a real run, cut with `…` where they are long. The agent's wording will differ from run to run; the Ticket IDs, States and buttons won't.

Each Stage works the same way. You type the Stage's skill as a slash command, followed by what to claim. The session claims the Ticket and posts any Questions as a Round. You answer the Round in the UI and tell the session to pick up the answers. The session finishes the Ticket, and you accept it in the UI. An implementation Ticket's branch is then merged, and you land it.

| Ticket | What you type |
| --- | --- |
| grilling Stage | `/grilling` |
| spec Stage | `/to-spec` |
| tickets Stage | `/to-tickets` |
| implementation | `/implement` |
| a Map's Charting (`wayfinder:chart`) | `/wayfinder` |
| a Map's grilling (`wayfinder:grilling`) | `/grilling` |
| a Map's task (`wayfinder:task`) | no skill: ask the session to claim it |

Type the slash command yourself. `/to-spec`, `/to-tickets` and `/implement` only start when you type them: asked in plain words to "write the Spec", the session does the work without the skill and skips its process. `/grilling` can also start from a plain request to grill you, but typing it is surer.

## Working a Requirement

1. **Start `claude` and check it reaches the Project.**

   ```console
   $ cd ~/src/acme-shop
   $ claude
   ```

   Run `/mcp`: `code-central` is listed under the Project's `.mcp.json`, with a tick.

   ```text
   Manage MCP servers
   …
     Project MCPs (/Users/you/src/acme-shop/.mcp.json)
   ❯ ✔ code-central 33 tools
   …
   ```

   If it isn't there, go back to step 4 of [A brand new repository](setting-up-a-project.md#a-brand-new-repository). Keep this session open for every step below.

2. **Submit a Requirement from the UI.** Open <http://localhost:7733>, click **New requirement**, type the Requirement's text and click **Submit requirement**. If you have more than one Project, choose `acme-shop` in the dialog. The first line becomes the working title:

   ```text
   Format prices for display

   Add a Go function that turns a price in pence into the string a customer sees, so 1999 shows as £19.99 and 5 shows as £0.05.
   ```

   Check: the UI opens the Requirement, `acme-shop-1 Format prices for display`, with its Grilling, Spec, Tickets and Build strip. Below it is its grilling Ticket, `acme-shop-2 Grilling: Format prices for display`, in Ready. The Requirement says *Waiting for a Factory to claim Grilling. Nothing needs you yet.* You'll claim it from your session instead.

   From a terminal, `code-central submit-requirement --text "…"` submits a Requirement too.

   **New Map**, beside **New requirement**, opens the same dialog to start a Map instead: a destination you'll find the way to one decision at a time, before there is a Requirement to grill. Type where you want to get to and click **Submit map**. Code Central creates the Map, a Spec labelled `wayfinder:map` whose body is your text followed by the empty sections `Destination`, `Notes`, `Decisions so far`, `Not yet specified` and `Out of scope`, and one Ready Charting Ticket for an agent to claim. In the **Requirements** view the Map's row shows how many of its decisions are made, such as *0/1 decided*, in place of the Stage strip, and adds *· 1 needs you* when one of its Tickets does. From a terminal, `code-central submit-map` does the same. Here it is in a new Project:

   ```console
   $ code-central submit-map --text "Offline checkout

   Customers should be able to finish an order with no connection, and have it go through once they're back online."
   {
     "spec": {
       "id": "acme-shop-1",
   …
       "changed": [
         "title",
         "labels",
         "body"
       ]
     },
     "charting": {
       "id": "acme-shop-2",
   …
       "state": "Ready"
     }
   }
   ```

   Selecting the Map opens the Map view where a Requirement's strip would be. Under the Map's title and count is its Destination, and below that its Tickets as a graph read left to right: each Blocker sits left of the Tickets it blocks, with an arrow to each. Each node shows the Ticket's type (`chart`, `grilling`, `research`, `task`, `prototype` or `ticket`), ID, title and State, and its top edge takes the strip's colours: green when done, the reason's colour when it needs you, solid blue while an agent is on it, dashed blue while it waits to be claimed, and grey while it is Draft or blocked. A Draft node that nothing blocks is on the frontier and has **Start**, which approves it to Ready. A dim **Not yet specified** node at the right edge shows that section of the Map when you click it. **Decisions so far** and **Out of scope** sit beside the graph and fold away. Clicking a node opens its Ticket below, as a Stage of a Requirement does. On a new Map there is only Charting, and the sections say *None yet.* until Charting fills them.

3. **Grill the Requirement.** In the session, type:

   ```text
   > /grilling Claim the grilling Stage of acme-shop-1 and grill me on it.
   ```

   The session claims `acme-shop-2`, reads the repository and posts its Questions as a Round instead of asking them in the terminal:

   ```text
   I've claimed acme-shop-2, the grilling ticket for spec acme-shop-1 ("Format prices for display"), and posted Round 1 in Code Central. The ticket is now in Needs Answer.

   Each of the six questions is worded so that "yes" accepts my recommendation:

   1. Currency: GBP only, with `£` hard-coded.
   2. Input type: a plain `int64` count of pence.
   …
   6. Package and name: a new package `money` with `Format(pence int64) string`, plus table-driven tests. …

   Answer the questions in the Code Central UI, since only answers given there are recorded. …
   ```

   It may ask to run `code-central wait acme-shop-2` in the background. If you allow it, the session picks the answers up by itself once you've answered. If you don't, tell it when you're done, as below.

   Check: **Needs Me** in the UI shows 1, and the Requirement's row reads *Grilling: Questions*. Click the row: `acme-shop-2` is in Needs Answer, with *Round 1, 6 of 6 open*.

4. **Answer the Round in the UI.** Each Question has the agent's recommended answer filled in as a draft. Nothing is saved until you answer. For each Question you can:

   - edit the text if you want, and click **Answer**,
   - or click **Dismiss** to leave it unanswered, so the agent decides without it.

   **Accept all recommended**, at the top of the Round, answers every open Question with its draft.

   In this run, Q6 was edited to fix the Go module path, *"Yes: package money with Format(pence int64) string. Use the module path acme-shop."*, and answered. **Accept all recommended** then answered the other five.

   Check: `acme-shop-2` is in Answered. The panel reads *Answered just now, no session has picked it up.*, with the session's Resume Command (`cd /Users/you/src/acme-shop && claude --resume …`) in case you've closed it.

5. **Tell the session to pick up the answers.**

   ```text
   > I've answered the Round in the UI. Pick up the answers.
   ```

   ```text
   I picked up your answers and finished the grilling: acme-shop-2 is now In Review. You accepted every recommendation, and for Q6 you also set the Go module path to `acme-shop`.

   What I wrote in Code Central:
   - Spec retitled to "GBP price formatting (money.Format)". The body is still your original text, for the spec stage to replace.
   - Resolution comment on acme-shop-2. It lists the six decisions with their reasons and a table of examples:
   …
   If you accept the review, the spec stage (acme-shop-3) can be claimed next.
   ```

   If a Round's answers raise new Questions, the session posts another Round instead of finishing. Answer it the same way.

6. **Accept the grilling Ticket.** In the UI, `acme-shop-2` is In Review, with a *Review the finished work* panel holding the session's summary. Read it, and the resolution comment in the Ticket's history, then click **Accept**. To send it back, write what to change in *What to change (required to reject)* and click **Reject**: the Ticket goes back to Ready, for your session (or a Factory) to claim again.

   Check: the strip's Grilling segment is done, and the next Stage is waiting, `acme-shop-3 Spec: Format prices for display` in Ready.

7. **Do the same for the spec and tickets Stages.** Each is one prompt, then a review in the UI. The session posts a Round only if it has a Question the earlier Stages didn't settle. If it does, answer it as in steps 4 and 5.

   ```text
   > /to-spec Claim the spec Stage of acme-shop-1 and write its Spec.
   ```

   ```text
   I claimed acme-shop-3 and wrote the full Spec into acme-shop-1 without asking you anything new, because the grilling had already settled every decision. The ticket is now In Review.
   …
   Please check two choices I made myself. The grilling left these open and you didn't answer them directly:
   - Tests are required.
   - The package sits at the root of the module, imported as `acme-shop/money`.
   …
   ```

   Check the Spec's body on `acme-shop-1`, then **Accept** `acme-shop-3`.

   ```text
   > /to-tickets Claim the tickets Stage of acme-shop-1 and break its Spec into implementation Tickets.
   ```

   ```text
   I claimed acme-shop-4 and broke the Spec into one implementation ticket, acme-shop-5, without asking you any questions. The tickets stage is now In Review.

   acme-shop-5: Add money.Format: render pence as a GBP display string. It's Ready but blocked by acme-shop-4, so no one can start it until you accept the breakdown. …
   ```

   **Accept** `acme-shop-4`.

   Check: Grilling, Spec and Tickets are done on the strip, and Build shows *0/1*. `acme-shop-5` is in Ready, with its acceptance criteria. A larger Requirement gets several implementation Tickets, ordered by Blockers. Work each one as in the next step.

8. **Work the implementation Ticket.** Unlike the Stages, it changes the repository: the session works on a branch named after the Ticket and commits there.

   ```text
   > /implement acme-shop-5 Claim acme-shop-5 and work it on a branch named acme-shop-5. Commit there, don't push, and finish the Ticket when it's done.
   ```

   It asks before running commands such as `go test` and `git commit`. Allow them.

   ```text
   `money.Format` is built and committed on the `acme-shop-5` branch, which isn't pushed, and acme-shop-5 is now In Review. All 18 test cases pass and `go vet` reports nothing.

   What's on the branch (two commits, head `e77cb03`):
   - `go.mod` for the new module `acme-shop`, set to Go 1.27.3, the installed version.
   - `money/format.go`: `Format(pence int64) string` is the package's only export. …
   …
   Once you accept the ticket, the branch still has to be merged into `main` and recorded in Code Central with `land` before it counts as Done.
   ```

   Check: `acme-shop-5` is In Review, and `git log --oneline main..acme-shop-5` lists the session's commits.

9. **Accept the implementation, merge it and land it.** Review the summary and the branch, then click **Accept**. Because the session named its branch when it finished, the Ticket goes to Accepted, not Done: the panel reads *Accepted, waiting to land*, with the branch and its head commit. Merge the branch into the base, `main`:

   ```console
   $ git switch main
   $ git merge --no-ff acme-shop-5 -m "Merge acme-shop-5: Add money.Format"
   Merge made by the 'ort' strategy.
    GLOSSARY.md          |  3 +++
    go.mod               |  3 +++
    money/format.go      | 33 +++++++++++++++++++++++++++++++++
    money/format_test.go | 43 +++++++++++++++++++++++++++++++++++++++++++
    4 files changed, 82 insertions(+)
   …
   $ git rev-parse --short HEAD
   59ce378
   ```

   Then land it: paste the merge commit into *Commit in the base (required to land)* and click **Land**. From a terminal, `code-central land acme-shop-5 --commit 59ce378` does the same. You can also ask the session to land it.

   Check: `acme-shop-5` is Done, Build shows *1/1*, and the Requirement's row reads *Finished · 1 of 1 Tickets Done*.

## Grilling in the terminal, without claiming

You can grill a Requirement in the session without claiming its grilling Ticket. The questions then stay in the terminal, and nothing is recorded in Code Central. Here a second Requirement, `acme-shop-6`, was submitted from the CLI and grilled in a new session:

```console
$ code-central submit-requirement --text "Show prices with VAT

Customers should see prices including VAT at 20%, using money.Format."
{
  "spec": {
    "id": "acme-shop-6",
…
  "stages": [
    {
      "id": "acme-shop-7",
…
      "state": "Ready"
    },
…
```

It prints the new Requirement, `acme-shop-6`, and its three Stage Tickets. The first, `acme-shop-7`, is the grilling Ticket. Then, in the session:

```text
> /grilling Grill me on Requirement acme-shop-6 here in the terminal. Don't claim it.
```

```text
I read acme-shop-6 (I haven't claimed it). The whole Spec is one line: "Customers should see prices including VAT at 20%, using money.Format." …

Round 1:

❓ Q1 - Are stored prices net or gross?: Do the pence values moving through the code exclude VAT, so we add 20% before display? …

➡️ Stored prices are net (ex-VAT) pence. …
…
```

You answer by typing in the terminal. Check: in the UI, `acme-shop-7 Grilling: Show prices with VAT` is still Ready, with no Claim and no Round, and Needs Me doesn't change.

Choose between the two ways:

- **Claimed**, as in [Working a Requirement](#working-a-requirement): the questions and your answers are kept as Rounds on the Ticket, the session finishes the Stage, and the next Stage can start. You can answer from the UI or Focus Mode, away from the terminal.
- **Unclaimed**: quicker for thinking a Requirement through at the terminal, but the decisions live only in that session. The grilling Ticket is still Ready, so the Stage still has to be worked, by you claiming it as above or by a Factory.
