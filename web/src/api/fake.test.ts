// The WASM half of the API contract suite (`make test`): createHttpApi on
// a fresh WASM Core world per test, which the Driver plays the agents,
// integrations and the Factory on. The serve half is http.e2e.test.ts.
import { apiContract } from "./contract/index";
import { newWorld } from "./core/core";
import { worldDriver } from "./driver";

apiContract("WASM Core", async () => worldDriver(await newWorld()));
