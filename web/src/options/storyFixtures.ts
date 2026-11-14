// The Options sections' stories run on a WASM Core world holding the Project
// "acme", set up by the scenario builder, so Save writes to Core and shows
// Core's own refusals. Nothing but the stories imports this.
import type { World } from "../api/core/core";
import { storyWorld } from "../api/core/story";
import type { Scenario } from "../api/scenario";

// optionsWorld is a story's loader: the Project, then setup, and what the
// sections take as props (the Project as read, the api, the world's clock).
export function optionsWorld(setup: (s: Scenario, w: World) => Promise<void> = async () => {}) {
  return storyWorld(async (s, w) => {
    await s.project("acme", "Acme");
    await setup(s, w);
    return { api: w.api, project: (await w.api.listProjects())[0], now: w.now() };
  });
}
