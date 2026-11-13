import type {
  CIWatcherStatus,
  IssueWatcherStatus,
  PullRequestWatcherStatus,
} from "../api/types";

// WatcherStatus is a Watcher's status, as it last reported it: Watching,
// with until when GitHub rate-limits it (code-central-260), or an error
// with its fix (code-central-253), or, as the service reports it, crashed
// and being restarted (code-central-255). It follows the Project as read,
// so it changes as the Watcher reports. The CI section shows the CI
// Watcher's, and the GitHub Issues section the Issue Watcher's
// (code-central-285).
export function WatcherStatus({
  name,
  repository,
  watcher,
}: {
  name: string;
  repository: string;
  watcher: CIWatcherStatus | IssueWatcherStatus | PullRequestWatcherStatus;
}) {
  return (
    <section className="project-options-watcher" aria-label={name}>
      {watcher.state === "watching" && (
        <p>
          {name}: Watching {repository}.
        </p>
      )}
      {watcher.state === "watching" && watcher.rate_limited_until && (
        <p className="muted">
          GitHub is rate-limiting it until{" "}
          <time dateTime={watcher.rate_limited_until}>
            {new Date(watcher.rate_limited_until).toLocaleTimeString([], {
              hour: "2-digit",
              minute: "2-digit",
            })}
          </time>
          , so it waits until then, and carries on by itself.
        </p>
      )}
      {watcher.state === "not_reported" && (
        <p className="muted">{name}: Starting; it hasn&apos;t reported yet.</p>
      )}
      {watcher.state === "error" && (
        <p className="warning">
          {name}: Error. {watcher.message}
          {watcher.fix && (
            <>
              <br />
              {watcher.fix}
            </>
          )}
          {watcher.problem !== "crashed" && (
            <>
              <br />
              It tries again every 2 minutes, and goes back to Watching once the
              cause is fixed.
            </>
          )}
        </p>
      )}
    </section>
  );
}
