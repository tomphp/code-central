# Plan Limits are read from each Turn's rate_limit_event and carried on the Heartbeat, in memory only

The human wants to see how much of the Claude account's 5-hour and weekly limits is used, so they can tell when the Factory is about to run dry. We decided that the Factory's Turn reader, which already reads each stream-json message, also reads the `rate_limit_event` message and takes the `five_hour` and `seven_day` windows from its `rate_limit_info.unifiedWindows`, each as a used percentage and a reset time. The Factory keeps the latest figures, stamped with when it read them, and every [Heartbeat](0025-a-factory-announces-itself-with-a-heartbeat-that-lives-only-in-memory.md) it sends carries them as `plan_limits`. Code Central keeps them with the Heartbeat in memory only, never in the store, and shows the figures from the freshest Factory that isn't Offline in the Limits popup. The UI's wire types come from the Go structs ([ADR 0042](0042-the-uis-wire-types-are-generated-from-gos-view-structs.md)). This builds on ADR 0025: after a restart the next Heartbeat brings the figures back.

## Considered Options

- **The `api/oauth/usage` endpoint.** Rejected: it is undocumented, needs the OAuth token from the keychain, and could break without warning.
- **Idle probe Turns.** Rejected: they spend the limit they measure.

## Consequences

- The figures go stale while the Factory is idle, since only a Turn brings new ones. We accept this, and the UI marks figures that are more than 10 minutes old.
- A Factory that has run no Turn since it started sends no Plan Limits.
- The limits are account-wide, so any Factory's Turns can update them.
