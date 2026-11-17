# Azure Boards picks its work items with a saved query or a WIQL WHERE clause

Azure Boards is far more configurable than GitHub Issues: work item types, States and fields differ by process (Scrum, Agile, Basic, CMMI and inherited ones), and the human's filter can be as specific as "PBIs or Bugs, assigned to me, Approved, with Component my-project". GitHub Issues' one filter label can't say that.

So the human picks one of two modes, and Code Central keeps the other mode's settings so switching back restores them:

- A **Saved query**: the URL of an Azure DevOps saved query, built and kept in Azure DevOps' own query editor. It must be a flat list; a tree or links query is refused by name.
- A **WIQL filter**: an Azure DevOps project URL and a WHERE clause alone. Code Central wraps it as `SELECT [System.Id] FROM workitems WHERE <clause>` and runs it in that project, so `@Me` and `@project` work and the result is always a flat list of IDs. A clause that starts with SELECT or WHERE is refused.

The Azure Boards Spec is titled after the organization and project the chosen mode runs in.

## Considered Options

- **A structured filter in Code Central** (types, States and fields to pick from). Rejected: it would have to discover each process's types, States and fields, too much for an MVP, and it would still miss what WIQL can say.
- **A full WIQL query.** Rejected: it could be a tree or links query, or carry its own SELECT and ORDER BY, which the Work Item Watcher would then have to reject or reinterpret.

## Consequences

- The filter is as expressive as Azure DevOps' own, and an organisation where saving a query isn't allowed can still use a WIQL filter.
- A bad clause is found by the check, which returns Azure DevOps' own message, not by Code Central parsing WIQL.
- One query per Project, in one organisation.
