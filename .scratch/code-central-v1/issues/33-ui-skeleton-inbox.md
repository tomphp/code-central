# 33: UI skeleton and the inbox, read-only

**What to build:** The human opens Code Central in a browser and sees the triage inbox: Projects with their Needs Me counts on the left, Needs Me grouped by reason in the middle, and the selected Ticket on the right, all updating live. The UI is built into the single binary. Actions arrive in later tickets. See the [spec](../spec.md), sections "Shape of the system" and "UI".

**Blocked by:** 32

**Status:** resolved

- [x] The UI is React and TypeScript built by Vite as a static single-page app, embedded in the binary; built files aren't committed, and the placeholder page still lets `go build` and Go tests run without Node
- [x] `make build` runs Vite then `go build`; in development, Vite's dev server proxies to the Go service
- [x] `GET /api/events` sends SSE events `ticket.changed {project, id}`, `spec.changed {project, id}` and `project.changed {project}`, from `events`
- [x] The UI reaches the HTTP API and SSE only through one TypeScript API module, which has an in-memory fake; a contract suite (each read, the error shape, SSE delivery) runs against the fake in `make test` and against the real module and a real `serve --store <tmpdir>` in `make e2e`
- [x] The left pane lists every Project with its Needs Me count, then a View list (Needs Me by default, or any single State, each with a count); both filters combine
- [x] The middle pane shows Needs Me grouped in the order the server returns, one row per Ticket with ID, title, how long it has waited and its one-line detail; Drafts sit below in a collapsed, quiet group; a State filter shows a flat list with a dot on rows that need the human
- [x] The right pane shows the selected Ticket read-only, with a slot for its action panel first
- [x] The view and the selected Ticket are in the URL (`?view=inbox&ticket=<id>`)
- [x] The UI refetches after each SSE event, and refetches everything after a reconnect
- [x] The tab's title and favicon show the Needs Me count
- [x] Inbox layout tests (Vitest and Testing Library, against the fake API) check group order as received, Drafts collapsed, the flat list with dots, combined filters, the view in the URL, and a refetch on an SSE notice
- [x] `make test` runs `go test ./...` and `vitest run` in parallel and still finishes within 10 seconds
