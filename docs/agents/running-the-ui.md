# Running the UI to look at a change

The service on 7733 serves the UI embedded in its binary and is the human's (see `real-service.md`). `npm --prefix web run dev` serves the source on 5173 with hot reload and proxies `/api` to `CODE_CENTRAL_URL` (default 7733, so always set it).

## Data to look at

`make demo-store` (alias `make demo`; `PORT=` to change 7801) builds the binaries, starts `code-central serve` on a scratch store and seeds a `demo` Project through the HTTP API: a Requirement in each Stage, a Map, a Small Change, a Spec with Tickets in every State, a Long-lived Spec and a Ticket waiting on the human. A second run reuses the store (`/tmp/code-central-demo-store`, or `-store`). It prints the URL to open and the `CODE_CENTRAL_URL` for Vite, and stays in the foreground.

```
make demo-store                                                    # run_in_background
CODE_CENTRAL_URL=http://127.0.0.1:7801 npm --prefix web run dev    # run_in_background
```

To change what it seeds, edit `internal/demostore/main.go`.

## Gotchas

- Start servers with the Bash tool's `run_in_background`, so TaskStop can end them. One started with `&` can't be stopped afterwards: the auto-mode classifier denies `pkill`.
- `make typecheck` (like any target that runs `npm ci`) reinstalls web dependencies and stales Vite's dependency cache. Restart a running dev server with `npx vite --force`.
- A port already in use is an error, not a service to reuse: pick another `PORT`.

## Screenshots without the Chrome tools

`web/node_modules/playwright-core` drives a browser; `make e2e` installs Chromium (`npx playwright-core install chromium`), and the e2e tests use the installed Chrome unless `PLAYWRIGHT_CHANNEL` says otherwise. A script in the scratchpad, run from `web/` so the import resolves:

```js
import { chromium } from "playwright-core";
const browser = await chromium.launch(process.env.PLAYWRIGHT_CHANNEL === "" ? {} : { channel: "chrome" });
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
await page.goto("http://127.0.0.1:5173/p/demo");
await page.screenshot({ path: "/tmp/ui.png" });
await browser.close();
```

Then Read the PNG.
