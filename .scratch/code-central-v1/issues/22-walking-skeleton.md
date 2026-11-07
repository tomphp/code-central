# 22: Walking skeleton: create and read a Ticket over HTTP and the CLI

**What to build:** The thinnest end-to-end path through Code Central. The human starts `code-central serve` on a fresh store, creates a Project over the HTTP API, then uses `code-central` commands to create a Spec and a Ticket on it and read them back as JSON. The Ticket survives a restart of the service. This lays down the structure every later ticket builds on: the one Go module, the deep `core` behind its list of named operations (with an injected clock and ID source), the `store` interface with file-backed and in-memory versions, the `httpapi` front door, the Go `client`, and `codecentraltest`. See the [spec](../spec.md), sections "Shape of the system", "Operations and front doors" and "The store".

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `code-central serve` runs in the foreground on port 7733, bound to `127.0.0.1` and `::1`, and refuses to start, naming the conflict, if the port is taken
- [x] The store lives at `$XDG_DATA_HOME/code-central`, falling back to `~/.local/share/code-central`, and can be overridden with `CODE_CENTRAL_STORE` or `serve --store <dir>`
- [x] A new store gets a `store.yaml` with `format: 1`; an unknown `format` refuses startup by name
- [x] The service takes an exclusive lock on the store, so a second `serve` on the same store refuses to start
- [x] `create_project(key, display_name)` and `GET /api/projects/<key>` work over HTTP, writing `project.yaml` under `projects/<key>/`; the display name defaults to the key
- [x] `create_spec`, `create_ticket` (in `Draft` or `Ready`), `get_ticket` and `find_tickets` work through `core`, as `POST` actions and `GET` resources under `/api/p/<key>/…`, and as kebab-case `code-central` commands that print JSON
- [x] Specs are Markdown with strict YAML frontmatter and Tickets are strict YAML, written in a fixed field order with UTC RFC 3339 timestamps; IDs come from one counter per Project, derived at startup, displayed as `<key>-<n>`
- [x] Every write is one file (temp file, fsync, rename) under one store-wide mutex, and memory is updated only after the file is on disk; every read comes from memory
- [x] A file that fails strict validation stops startup and names the file and field
- [x] Errors use one shape, `{code, message, allowed_actions?, field?}`, with HTTP status 404, 409 or 422, and on the CLI as JSON on stderr with exit code `2`
- [x] Every CLI command reaches the service through the `client` package at `CODE_CENTRAL_URL` (default `http://localhost:7733`), and fails with `service_unavailable` naming `serve` and `service install` when it can't connect
- [x] One store contract suite runs against the file-backed store and the in-memory fake (reads after writes, the derived ID counter); file-only behaviour (strict decoding, unknown `format`, the lock, one file per write, field order) is tested on the file-backed store only, in temporary directories
- [x] `codecentraltest.NewServer(t)` starts the real `core`, the in-memory store and a fake clock behind the real `httpapi`, and the `client` tests run against it
- [x] A committed placeholder page lets `go build` and Go tests run without Node
- [x] `make test` runs the test suite and fails if it takes more than 10 seconds of wall-clock time
