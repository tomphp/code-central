# How agent sessions are taught to use Code Central

Type: grilling
Blocked by: 12

## Question

How does an agent session learn to use Code Central? When an agent sets up a repository, what does it write there so that sessions point at the workflow the MCP server publishes rather than copying it, as The State workflow decided? Does this replace `docs/agents/issue-tracker.md` for this repo and others, and how do the skills that read that file keep working? How do these instructions make the unchanged grilling skill post Rounds to the Ticket, end the turn and start `code-central wait`, and how is the hook that adds the session ID to each Round installed? How a Project is identified decided that `code-central init` is run by the human and writes `.mcp.json`. What, if anything, should it write besides that?

[How agents claim work](09-claiming-work.md) adds to this: the hook now also adds the session ID to the claim and sends the throttled `PostToolUse` ping, and the published workflow gains the branch rule and the limits of holding several Claims. Where does that branch rule go, given that Code Central knows nothing about git?

[Process shape and web stack](10-process-and-stack.md) adds the setup outside the repository: `code-central service install` to keep the service running, `sbx policy allow network localhost:7733` for Docker Sandboxes, and `--bind` for plain Linux Docker. Where are these taught, and does `init` mention them?

[The MCP and HTTP surface](12-api-surface.md) puts some teaching into the server itself: the MCP `instructions` point agents to `get_workflow`, and each write's response carries the next step, such as "end your turn and run `code-central wait <id>` in the background". The hook fills in `session_id` on `claim` and `post_round` through `updatedInput`, and the `PostToolUse` ping runs `code-central ping` with the hook's JSON on stdin. It is throttled on the server and always exits `0`. Given that, what still has to be written into the repository, and how much can the instructions there leave to the server?
