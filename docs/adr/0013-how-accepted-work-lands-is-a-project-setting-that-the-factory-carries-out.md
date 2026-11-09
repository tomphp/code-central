# How accepted work lands is a Project setting that the Factory carries out

[ADR 0006](0006-accepted-work-lands-before-it-is-done.md) left how accepted work lands to whoever delivers it, and the Factory always merged the branch into the base locally. Some humans want to merge accepted work themselves, and some want it pushed to a remote, so one way suits nobody who wants another. We decided each Project has a **Landing Method**: **Merge** (the Factory merges the branch into the base, as before, and the default) or **Branch** (the Factory leaves the branch for the human to merge, and lands the Ticket when it sees the merge). Each has a **Push** option naming a git remote, `origin` by default: under Merge the Factory pushes the base after merging, and under Branch it pushes the Ticket's branch on accept.

Code Central stores the Landing Method on the Project, the human sets it over the HTTP API (`set_landing_method`, from the Project Options page), and every Project read returns it. Code Central uses it only to choose a Needs Me reason. It still never sees git: only the Factory merges, pushes and lands, so whether the remote exists is the Factory's check. Agents get no MCP tool or CLI command for it, and the rule that they never push or merge stays.

Branch keeps the Accepted State, so Blockers still clear only on land. Under Merge + Push, Done means pushed: the Ticket lands only once the push has succeeded.

## Considered Options

- **A Factory start flag.** Rejected: it applies to one Factory run only, and nothing else (the UI, Needs Me, another Factory) could see it.
- **Branch accepting straight to Done.** Rejected: a Ticket it blocked would be claimed and started from a base without the work it depends on.

## Consequences

A Project stored without a Landing Method reads as Merge with Push off and remote `origin`, so existing Projects keep today's behaviour with no migration of their files. The Factory reads the setting on every poll, so a change applies to every Ticket still waiting to land. A failed push leaves the Ticket where it is, Accepted or, if the human merged it by hand, In Review, with a comment, and the Factory retries it; it never pulls, rebases or force-pushes. A Pull Request method, which comes later, fits beside these and implies Push.
