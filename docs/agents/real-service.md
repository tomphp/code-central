# The real service on 7733

The Code Central service on port 7733 is the human's own instance, with their real Projects. The MCP server in `.mcp.json` points at it for **tracker work only**: claiming, reading and writing Tickets.

Never do any of this to it:

- run `serve`, `service install`, `service uninstall`, `pkill` or a restart against it;
- run `init` or `ping` (they reach whatever `CODE_CENTRAL_URL` names, else 7733) from a test, a demo or a scratch repo;
- `curl` a write to it;
- point `CODE_CENTRAL_URL` at it from a test, a demo or a script.

Work that needs a running Code Central (looking at the UI, trying the CLI, running `init`, a test) starts its own on another port and points at it:

```
code-central serve --store <scratch> --port <spare>
export CODE_CENTRAL_URL=http://127.0.0.1:<spare>
```

A Make target that starts a server takes a port that is not 7733 and sets `CODE_CENTRAL_URL` for every child process (Vite, CLI seeding, `init`) rather than letting it default to 7733. `make e2e` and `web/src/e2e/serve.ts` already do this.

## What `init` does to the service

`code-central init` calls the service at `CODE_CENTRAL_URL` (default `http://localhost:7733`): it reads the Project for the key, and when the key is unknown it creates the Project there (`create_project`), then writes `.mcp.json` and the guide files into the repository. So `init` in a scratch repo, with `CODE_CENTRAL_URL` unset, would register a scratch Project in the human's real service. In a scratch repo, set `CODE_CENTRAL_URL` to the test instance first. Re-running `init` in this repository to refresh its files is the one intended use of the real service, and is the human's to run. `ping` only reads, but it too needs `CODE_CENTRAL_URL` set to the test instance.

## What stops a test

- Go: `client` refuses, by panic, any request from a test binary to port 7733 or to a non-loopback address (`client/realservice.go`), so every package that builds a client is covered without a `TestMain`.
- Vitest: `web/src/test-setup.ts` makes `fetch` throw on port 7733, and `web/vite.config.ts` fails a test run whose `CODE_CENTRAL_URL` names it.
