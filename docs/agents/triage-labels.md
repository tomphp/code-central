<!-- Written by code-central init 5af8e3b. Re-running init rewrites this file. -->

# Triage labels

The skills speak of five triage roles. In Code Central, three are States and two stay Labels. Call `get_workflow` for how Tickets move between States.

| Role in the skills | In Code Central |
| --- | --- |
| `needs-triage` | `Draft` State (`create_ticket`, `defer`) |
| `needs-info` | `needs-info` Label |
| `ready-for-agent` | `Ready` State (`approve`) |
| `ready-for-human` | `ready-for-human` Label |
| `wontfix` | `Cancelled` State (`cancel`) |

When a skill applies a role that is a State, use the operation beside it instead of adding a Label.
