// The pull request badge (code-central-388): a Ticket's pull request on
// its detail, its rows in the Spec view and its Inbox row, with its
// number, its state and a link to the Code Host.
import type { MouseEvent } from "react";
import type { PullRequestView } from "../api/types";

// PullRequestBadge links to the pull request. Inside a row that is itself
// a link it is a button-like span that opens the pull request in a new
// tab instead, since a link can't hold a link.
export function PullRequestBadge({ pr, inRowLink }: { pr?: PullRequestView; inRowLink?: boolean }) {
  if (!pr) return null;
  const cls = `label pr-badge pr-${pr.state}`;
  const text = `PR #${pr.number} ${pr.state}`;
  if (!inRowLink) {
    return (
      <a className={cls} href={pr.link} target="_blank" rel="noreferrer" aria-label={`Pull request ${pr.number}, ${pr.state}`}>
        {text}
      </a>
    );
  }
  const open = (e: MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    window.open(pr.link, "_blank", "noreferrer");
  };
  return (
    <span className={cls} role="link" tabIndex={0} aria-label={`Pull request ${pr.number}, ${pr.state}`} title={pr.link} onClick={open}>
      {text}
    </span>
  );
}
