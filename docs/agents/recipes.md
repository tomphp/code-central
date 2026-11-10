# Recipes

Recipes for changes that touch many packages. Each one starts with a single edit and then lets the tests name what's missing: make the first change, run the tests listed, and fill in each gap they report. That's cheaper than tracing an existing operation through every layer by reading.

## Add or change an operation

1. Add it to `core.Operations` in `core/operations.go` with its `Audience` and `Write`, then write the Core method.
2. Run `go test ./core ./frontdoor`. The guard tests name each missing piece:
   - `TestHTTPExposesEveryOperation`: a route in `httpapi.Routes` (`httpapi/httpapi.go`), served through the shared handlers.
   - `TestMCPExposesExactlyTheAgentOperations` and `TestCLIExposesExactlyTheAgentOperations`, for `AgentAndHuman` only: a tool in `tools` (`mcp/mcp.go`, with a one-line description) and a kebab-case command in `cli/cli.go`, which calls the service through a `client/` method.
   - `TestEveryOperationHasACase`: a cross-door case. Put a feature's cases in their own `frontdoor/<feature>_doors_test.go`, registered from `init()` (see `frontdoor/modelchoice_doors_test.go`).
   - `TestAnArchivedProjectRefusesEveryWrite`, for a write: an entry in `writesOnAcme` in `core/archive_test.go`.
3. When the UI uses it: add its types to `web/src/api/types.ts`, an action in `web/src/api/actions/`, its behaviour in `web/src/api/contract.ts` and the matching behaviour in `web/src/api/fake.ts`. `make vitest F=src/api/fake.test.ts` runs the contract against the fake.

Examples to copy: `02bd5d9` (`keep_as_draft`: a human operation with UI) and `8964c77` (`get_model_choice`: Factory-only, with a `client/` method).

## Change a store file

Any change to a store file struct (`store/file.go`), its yaml tags or a checked value set bumps the store format, even an additive one. Run `go test ./core -run TestTheStoreSchemaMatchesItsSnapshot`: when it fails, it prints the four steps and the snapshot to diff against. The example to copy is `8964c77`'s `store/migrate_claim_model_test.go`.

Parallel Tickets each bump the format, so they collide at merge. When merging the base brings in a format your branch also claimed, your migration moves to the next number. Renumber its `From` in `store/migrate.go` and `currentFormat`, keep the base's snapshot for the format it took, and write yours for the next format by running the snapshot test with `-write-store-schema`. Merge commit `8093159` shows this.
