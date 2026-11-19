// The API contract suite: one set of tests that pins createHttpApi
// against the WASM Core in the fast suite (fake.test.ts) and against a
// real `code-central serve --store <tmpdir>` in the e2e tier
// (http.e2e.test.ts). Both halves register every feature's cases here.
//
// The cases are one file per feature in this folder. A Driver (driver.ts)
// sets up the world the way agents, integrations and the Factory would,
// with their operations over HTTP. Tests use the IDs the driver returns,
// never literal IDs (Specs and Tickets share one counter).
import { afterEach, beforeEach, describe } from "vitest";
import type { Driver } from "../driver";
import { chatContract } from "./chat";
import { ciContract } from "./ci";
import { intakeContract } from "./intake";
import { integrationContract } from "./integration";
import { labelsContract } from "./labels";
import { landingContract } from "./landing";
import { needsMeContract } from "./needs_me";
import { optionsContract } from "./options";
import { projectSettingsContract } from "./project_settings";
import { projectsContract } from "./projects";
import { reviewContract } from "./review";
import { roundsContract } from "./rounds";
import { storageContract } from "./storage";
import { ticketContract } from "./ticket";
import { ticketsContract } from "./tickets";
import { transcriptContract } from "./transcript";
import { usageContract } from "./usage";

export { wantAcknowledgement } from "./support";

export function apiContract(name: string, newDriver: () => Promise<Driver>) {
  describe(`API contract: ${name}`, () => {
    let d: Driver;
    beforeEach(async () => {
      d = await newDriver();
    });
    afterEach(async () => {
      await d.close();
    });

    chatContract(() => d);
    ciContract(() => d);
    intakeContract(() => d);
    integrationContract(() => d);
    labelsContract(() => d);
    landingContract(() => d);
    needsMeContract(() => d);
    optionsContract(() => d);
    projectSettingsContract(() => d);
    projectsContract(() => d);
    reviewContract(() => d);
    roundsContract(() => d);
    storageContract(() => d);
    ticketContract(() => d);
    ticketsContract(() => d);
    transcriptContract(() => d);
    usageContract(() => d);
  });
}
