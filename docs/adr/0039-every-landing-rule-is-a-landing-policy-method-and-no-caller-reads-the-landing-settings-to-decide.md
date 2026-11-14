# Every landing rule is a Landing Policy method, and no caller reads the landing settings to decide

The Landing Policy is the one place where a Project's Landing Method, Push, Land per Spec and CI Tracking combine into the rules by which accepted work lands. It now lives in `core` as `core.LandingPolicy`, built by `NewLandingPolicy` from the four inputs, by an unexported builder from a `store.Project` for core's own sites, and by `ProjectView.LandingPolicy()` for the Factory and other clients of the view. Those builders are the only code that turns settings fields into a policy; nothing new is serialised. `store` keeps only `Landing.Normalized()`, because store can't import core.

Every landing rule is a method of the policy, and no caller reads the landing settings to decide. This reverses the exception in `docs/agents/recipes.md`, which let a test of one input alone stay a plain comparison: a rule that reads one setting today is the first to need a second, and the comparison is then missed at some call site. A type-matched guard test, with an allowlist for the sites that only display or store a setting, holds the line.

Moving the policy into core leaves [ADR 0003](0003-the-store-is-yaml-files-loaded-into-memory.md) untouched, and follows ADRs 0013 and 0032. It does not re-litigate ADRs 0019, 0028, 0029, 0030, 0031, 0033 or 0034.

## Considered Options

- **Leaving single-input tests as plain comparisons.** Rejected for the reason above.
- **Keeping the policy in store.** Rejected: the Factory and core both need it from a view, and a view is built in core.

## Consequences

Adding or changing a landing rule is one method and one row of the matrix test.
