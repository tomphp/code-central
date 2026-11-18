# The UI reads derived rules from Go's views and only formats them

The UI kept TypeScript copies of rules the service already decides: where Edit and the Retro button are offered, a Spec's kind, a plain Spec's first need and whether a Factory is on it, a Requirement's Stage strip, and which parts of the Options form each Landing Method offers. Each copy could drift from the Go rule it mirrored, and only a human noticing a wrong button would catch it. So we decided that Go's views carry each derived rule as a field, and the UI reads the field and only formats it: badge text, segment names, reason labels and headlines stay in the UI, and the decision does not.

Spec code-central-931 moves the rules in three slices: Edit and Retro with Spec kind, need and claimed first, then the Stage strip, then landing capabilities.

- **Edit and Retro**: `TicketView` and `SpecDetail` carry `editable` and `retroable`. Each is one unexported `core` predicate beside the write that refuses the same case (`ticketEditable` and `specEditable` beside `edit_ticket` and `edit_spec`, `ticketRetroable` and `specRetroable` beside `submit_retro`), with an Archived Project folded in. The writes keep their own checks, as each refuses its case with its own message, so the two live in one file to change together, and Go tests pin each rule's case on both.
- **Spec kind, need and claimed**: `SpecRow` and `SpecDetail` carry `kind` (`requirement`, `map`, `long_lived` or `spec`, in that precedence), `need` (the first Needs Me reason of the Spec's Tickets in claim order) and `claimed` (whether any of them holds a live Claim).
- **The Stage strip**: `RequirementView` carries `segments` (each `{stage, progress, ticket?, map?, reason?}`, a Map's or Straight to Spec's first grilling segment included), `current`, `needs` and `default_ticket`, worked out beside `requirementView` from the same evaluation as each Stage's `needs_me_reason`. The UI keeps the segment names (a segment with `map` is "Map"), the headline and the waiting note, and the row filter that picks out a Requirement's Build Tickets. `stages` keeps `needs_me_reason`, as the CLI and the Factory read it.
- **Landing capabilities**: the Project's landing view says what the Options form offers for each Landing Method, so the form reads it instead of branching on the method ([ADR 0039](0039-every-landing-rule-is-a-landing-policy-method-and-no-caller-reads-the-landing-settings-to-decide.md)). `LandingView.methods` is a table keyed by every Landing Method, each worked out as if it were the Default Landing, so the form looks up `methods[draft.method]` as the human clicks between radios, with no round trip; beside it are `runs_pull_request_watcher`, `repository_landing` and `pushes_nothing`. A vitest scan in `web/src/test-setup.test.ts` fails on a comparison with a landing value unless a `// landing: <reason>` marker allows that one line.

## Considered Options

- **Keep the TypeScript copies, with shared test fixtures.** Rejected: fixtures pin the cases someone thought of, not the rule, so a Go change that the fixtures don't cover still drifts silently. Two implementations stay two.
- **A read operation per rule** (such as "may I edit this?"). Rejected: it adds a round trip per row and per button for what every view is built with at hand, and multiplies the operations each door has to wire.

## Consequences

- **Every new field is additive.** The HTTP API, the MCP tools, the Factory and the CLI keep working. `make wiretypes` regenerates the UI's types ([ADR 0042](0042-the-uis-wire-types-are-generated-from-gos-view-structs.md)), and the UI's fake is the real Core ([ADR 0060](0060-the-uis-fake-is-the-real-core-built-to-webassembly.md)), so UI tests get the fields with no fake to update.
- **An Acknowledgement doesn't list `editable` or `retroable`** in `changed`: they are offers that follow from what a write changed, not fields it wrote, so agents' acknowledgements stay as they were.
- **Every read sets the flags**, `get_ticket`'s lighter reads too, whatever parts are asked for.
- **Time-dependent reasons** (a Silent or Long Claim) are read at read time in `need`, as `stages[].needs_me_reason` already is. The Inbox loads Spec rows and Needs Me together, so they agree on a load.
- **Go tests name the glossary rule each case answers**, and UI tests assert what renders from the field on a WASM Core world, not which helper ran.
