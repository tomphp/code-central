# The running service is the only writer to the store

A Claim must be one atomic step across every process that can write, so we decided that only one process can write: the running Code Central service. Every `code-central` command, including `init` and Import, is an HTTP client of that service and never opens the store itself. The service takes an exclusive lock on the store at startup, so a second copy on another port refuses to start.

## Considered Options

- **Each command writes the files directly, with cross-process file locks.** Rejected because every command, and every future tool that touches the store, would have to get locking right, and a claim's atomicity would rest on all of them.

## Consequences

Every command, including `init` and Import, fails when the service isn't running, and says how to start it. Hand-editing a file in the store is still possible, but the service is the one that has to notice it.
