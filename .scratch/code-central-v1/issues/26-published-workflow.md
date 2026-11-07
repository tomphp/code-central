# 26: The published workflow

**What to build:** An agent session learns how to use Code Central from the server, not from copies in a repository. On connecting, it gets short MCP `instructions`; before working a Ticket it calls `get_workflow` and gets the whole workflow, generated from the same table `core` enforces; and every write tells it the next step at the moment it matters. A test keeps all this text within its context budget. See the [spec](../spec.md), sections "The workflow", "Rounds, Questions and Failure Reports" and "Context budget for teaching agents".

**Blocked by:** 25

**Status:** ready-for-agent

- [ ] `get_workflow` is an MCP tool, `GET /api/workflow` and `code-central workflow`, returning structured data plus prose
- [ ] The structured part is generated from `core`'s action table: States, meanings, actions, who can take each, required content and what counts as Needs Me, so it can't drift from what is enforced
- [ ] The prose covers the three exits on trouble, that a send-back with no note means nothing has changed, the branch rule (a branch named after the Ticket ID, committed and pushed often), and the limits of holding several Claims
- [ ] The MCP `instructions` say what Code Central is, to call `get_workflow` before working a Ticket, and the Round rule: while a Claim is held, every question for the human is a Round posted with `post_round`, never printed in the terminal, including each numbered grilling question as one Question with its recommendation
- [ ] Writes return their one-sentence next step; `finish` returns "stop working this Ticket" (the other actions' next steps arrive with their tickets)
- [ ] A `core` test fails if the MCP `instructions` exceed about 150 words, any tool description exceeds one line, the `get_workflow` prose exceeds about 1,500 tokens, or a next step exceeds one sentence
