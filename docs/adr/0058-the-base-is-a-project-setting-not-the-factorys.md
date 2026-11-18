# The base is a Project setting, not the Factory's

The base was whatever branch the Factory had checked out when it started: work was cut from it, Merge merged into it, and the Factory reported it to Code Central as `PushRemoteBase` for the Pull Request Watcher. [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md) and [ADR 0030](0030-under-pull-request-the-factory-fetches-and-fast-forwards-the-base-and-a-diverged-base-stops-landing-and-new-branches.md) both assume that checked-out branch. The human could not see it, change it or have Needs Me name it.

The base is now a **Project setting**, kept in the Project's Landing settings and set with `set_landing_method` (`base`, optional: left out keeps the stored value). It is one branch for all four jobs, under every Landing Method: work is cut from it, Merge merges into it, Pull Request targets it, and Branch watches it. The service checks only the name's syntax, by the rules of `git check-ref-format --branch`; it never checks the branch exists, because Code Central never sees git. The Project read returns the resolved base in `landing.base`, and the Factory and the Pull Request Watcher read it from there.

This amends ADR 0013 and ADR 0030: where they say "the base" is the Factory's checked-out branch, it is the Project's setting. Their other rules (the Factory never rebases or force-pushes; under Pull Request it fetches and fast-forwards the base) stand, on the Project's base.

## Considered Options

- **A merge target separate from the branch work is cut from.** Rejected: a blocked Ticket would start without the work it depends on, as ADR 0013 already argued for Branch.
- **`--base` as a per-run override, or as a fallback.** Rejected: a per-run flag is invisible to the UI, to Needs Me and to the Pull Request Watcher.

## Consequences

- A Project with no stored base reads the `PushRemoteBase` a Factory last reported, else `main`. No files are migrated.
- `PushRemoteBase` is frozen: `report_push_remote` ignores a `base` an older Factory sends, and only the fallback reads the stored value.
- A change of base applies to every step not yet taken. An Accepted Ticket not yet merged merges into the new base; work already merged into the old base and waiting on its push or CI result finishes there. An open pull request keeps its target. No per-Ticket base is stored.
- A base that exists neither locally nor on the Push remote is **Base missing**, which the Factory reports through `report_base`; until it is found the Factory neither lands nor cuts new branches.
