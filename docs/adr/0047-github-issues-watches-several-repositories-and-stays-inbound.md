# GitHub Issues watches several repositories and stays inbound

A Project now spans several Repositories, and the human wants GitHub issues to track work across all of them. [ADR 0022](0022-incoming-issues-go-on-one-long-lived-issues-spec-per-project.md) gave a Project one Issues Spec fed from one repository.

So GitHub Issues names a list of repositories, any `owner/repo`, each checked as one is today, and every Incoming Issue from any of them lands on the Project's one Issues Spec, titled "GitHub Issues" once more than one is listed. One failing repository is an error naming it while the others are still watched. A Repository with no GitHub remote, or with issues off, is simply not listed, and Write-back still reports its work.

GitHub Issues stays inbound: Code Central never opens issues or sub-issues in the Changed Repositories. It reports them on the one linked issue: each landed comment names the commit per Changed Repository, `owner/repo@sha` (GitHub's own cross-repository autolink) or `Name@sha`, and the closing comment lists the Spec's Changed Repositories. Comments are posted once and never edited, so a changing set needs no rule.

This amends ADR 0022's one repository per Issues Spec and its "titled after the repository".

## Considered Options

- **One issue per Changed Repository**, opened by Code Central. Rejected: a new outbound feature, issues to keep in step as the set changes, and noise in trackers the human didn't ask to use.
- **A parent issue with sub-issues in the Changed Repositories.** Rejected: cross-owner sub-issue support is unverified, and it is the same outbound feature.
- **One Issues Spec per repository.** Rejected: it splits the Project's one inbox that ADR 0022 chose.

## Consequences

- One inbox per Project across every repository.
- A single-repository Project is unchanged.
- Issues opened in a Changed Repository's own tracker stay outside Code Central unless that repository is listed.
