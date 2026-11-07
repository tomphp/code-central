# 43: Running as a service

**What to build:** The human installs Code Central as a login item so agents can always reach it, restarts it with one command after an upgrade, and can work around a port clash or reach it from Linux Docker without touching committed files. A CLI of a different version warns but keeps working. A restart during an agent's `wait` loses nothing. See the [spec](../spec.md), sections "Shape of the system" and "Operations and front doors".

**Blocked by:** 29

**Status:** resolved

- [x] `code-central service install` writes a launchd agent on macOS or a systemd user unit on Linux that starts at login and restarts after a crash, recording the binary's absolute path as invoked without resolving symlinks; running it again rewrites the unit and restarts the service
- [x] `code-central service restart` restarts the installed service
- [x] A local setting changes the port; `CODE_CENTRAL_URL` then points agents at it, so committed files never change
- [x] `--bind` adds one specific address to the listeners and to the `Host` allowlist, and the docs warn against a LAN address
- [x] `make build` stamps `git describe --tags --always --dirty` as the version, shown in a response header and `code-central --version`
- [x] The CLI warns on stderr, without failing, when its version differs from the service's, naming the older side when both are clean tags ("run `code-central service restart`" or "this CLI is older than the service"); the `client` tolerates unknown response fields
- [x] On shutdown, the service finishes in-flight writes and closes long-polls
- [x] The version warning is tested in the CLI package against `codecentraltest`
- [x] End-to-end test 2: the service is stopped during a `wait` and started again; `wait` resumes, the state survives, and a second `serve` on the same store refuses to start
- [x] A manual checklist for when v1 is built covers Docker Desktop for Mac, Linux Docker with `--bind`, `sbx`, `service install`, and real Claude Code (hooks, `updatedInput`, `CLAUDE_CODE_SESSION_ID`)
