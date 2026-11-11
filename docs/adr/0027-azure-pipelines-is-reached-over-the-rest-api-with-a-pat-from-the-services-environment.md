# Azure Pipelines is reached over Azure DevOps' REST API with a PAT from the service's environment

GitHub Actions goes through `gh` and its own login (ADR 0018). Azure DevOps has no such login for Code Central to lean on, so the **Azure Pipelines** CI Provider calls Azure DevOps' REST API directly, from the CI Watcher and never from the service (ADR 0002, ADR 0004). It authenticates with a Personal Access Token (PAT) that the CI Watcher reads from its own environment, which is the service's, under a variable the Project names (`ADO_PAT` unless it names another). Code Central stores the variable's name and never the PAT. The check mode refuses to turn CI Tracking on while the variable is unset or empty, and its fix says how to set it for the OS's service and restart it.

## Considered Options

- **The `az` CLI with the azure-devops extension.** Rejected: a heavy extra install and login that would have to be on the service's `PATH`.
- **Storing the PAT in Code Central.** Rejected: Code Central keeps no secrets, and the PAT would sit in the Project's files.
- **Copying the variable into the service definition at `service install`.** Rejected: it writes the PAT to disk, in a file the service manager owns.

## Consequences

The human sets the variable for the service, on each OS, and restarts it; the check mode reports when it is missing. Several Projects in different Azure DevOps organizations each name their own variable.
