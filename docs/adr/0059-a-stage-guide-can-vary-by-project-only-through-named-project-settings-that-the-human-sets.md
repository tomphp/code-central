# A Stage Guide can vary by Project, only through named Project settings that the human sets

Until now a Stage Guide was the same for every Project: `get_stage_guide` took no Project, its HTTP route had no Project key, and `claim` built it from nothing but the Ticket's Stage. Code-central-321 added a line to the implementation guide telling the agent to send reading across several files to an Explore subagent. Some Projects run agents where subagents are unavailable or too costly, and the human had no way to leave the line out for one Project.

An optional guide line lives in a fragment file under `core/stageguides/fragments/`, outside the `stageguides/*.md` embed glob so nothing mistakes it for a guide. The guide keeps a placeholder line where the line goes. One named Project setting governs the line, starting with **Subagent Exploration**, on by default. When it is on, Code Central replaces the placeholder with the fragment, as the same bullet; when it is off, it removes the placeholder line whole, leaving no empty bullet. The Project stores the setting as its opposite, so a Project stored without it reads as on, with no migration beyond the format bump. Only the human sets it, over HTTP from Project Options; it has no MCP tool and no CLI command.

Because the guide now depends on the Project, every door serves the Project's own: `claim` builds it from the Ticket's Project, `get_stage_guide` takes the Project key, HTTP is `GET /api/p/{key}/stage-guides/{stage}` with the unkeyed route removed, and the CLI and the Factory's fallback pass their Project.

## Considered Options

- **A Factory start flag.** Rejected: only one run would see it, and the guide must match on every door.
- **A `settings.yaml` key.** Rejected: it is machine-wide and needs a restart.
- **A general per-Project fragment list.** Rejected: speculative with one line. Each later optional line gets its own named setting.
- **Keeping the line in the guide and stripping it.** Rejected: the human preferred the fragment, so the guide's text and the optional text are each edited in one place.

## Consequences

- Every door, including HTTP and the CLI, takes a Project to serve a guide.
- Existing Projects read as on, so upgrading changes nothing.
- A change applies from the next Claim and the next `get_stage_guide` call. A session already running, or one resumed after a Round, keeps the guide in the system prompt it started with, unlike Models, which apply from the next turn.
- The context budget measures the implementation guide with the fragment in, its largest case, and the tickets guide with the Model Assignment fragment in.
- A fragment can be filled in with Project data, as Model Assignment's is with the Project's Reported Models.
- A setting can default to off: it is stored as on (not as its opposite), so a Project stored without it reads off, and the store format bump migrates every existing Project off.
