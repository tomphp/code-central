# Azure Boards is a second Integration with its own Watcher and inbox Spec

The human wants Azure DevOps work items to arrive in Code Central as GitHub issues do. [ADR 0020](0020-an-integration-runs-as-a-watcher-supervised-by-the-service.md) already says another system's Integration is one more kind of Watcher, so **Azure Boards** is a second Integration, run by a new kind, the **Work Item Watcher**, which polls Azure DevOps every 2 minutes. Azure DevOps' service hooks would need a public HTTPS endpoint, which a local service doesn't have.

A work item arrives as an **Incoming Issue** and is triaged exactly as a GitHub issue is: Keep as Draft, To Requirement or Won't fix, with Awaiting triage, New issues need me and Write-back widened to either Integration. The behaviour is the same, so the vocabulary and the triage UI are too.

Its Incoming Issues go on a Long-lived Spec of their own, the **Azure Boards Spec**, titled "Azure Boards: org/project", made by `set_azure_boards` the first time Azure Boards is turned on and stored with its settings, as [ADR 0022](0022-incoming-issues-go-on-one-long-lived-issues-spec-per-project.md) made the Issues Spec. Both Integrations may be on at once. This amends ADR 0022 and [ADR 0047](0047-github-issues-watches-several-repositories-and-stays-inbound.md)'s one inbox per Project to one inbox per Integration.

The Work Item Watcher authenticates with a PAT read from an environment variable, the Project's CI PAT variable unless Azure Boards names its own, and Code Central stores only the variable's name. This extends [ADR 0027](0027-azure-pipelines-is-reached-over-the-rest-api-with-a-pat-from-the-services-environment.md) from the CI Watcher to the Work Item Watcher, unchanged.

## Considered Options

- **A shared Issues Spec for both Integrations.** Rejected: its title, its creation and which setting owns it would all have to be reworked across two Integrations, and a Project rarely uses both trackers.
- **A separate vocabulary and triage for work items.** Rejected: the behaviour is identical, so it would duplicate the triage UI for no difference.

## Consequences

- "Is this the Issues Spec?" becomes "is this an Integration's inbox Spec?" wherever core asks it, and Awaiting triage reads the New issues need me of the Integration whose inbox the Ticket is on.
- Each watch read lists only its own Integration's Links and Write-backs.
- A further Integration follows the same shape: a Watcher kind, a setting with its own inbox Spec, and the shared triage.
