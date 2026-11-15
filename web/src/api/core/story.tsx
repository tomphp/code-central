// A story's WASM Core world (Spec code-central-580): a Storybook loader
// builds it with the scenario builder before the story renders, and the
// story's cleanup drops it, so it lives as long as the story. A decorator
// renders the story inside <Live> over the world, so its live reads work
// with no set-up. Spread it into a story's meta and read what build
// returned from loaded.world:
//
//   const meta = {
//     ...storyWorld(async (s, w) => { await s.project("acme"); return { api: w.api }; }),
//     render: (_args, { loaded }) => <Section {...loaded.world} />,
//   } satisfies Meta<typeof Section>;
//
// A meta that sets decorators of its own must add them to storyWorld's.
import type { Decorator } from "@storybook/react-vite";
import { Live } from "../../live";
import type { World } from "./core";
import { newWorld } from "./core";
import { scenario, type Scenario } from "../scenario";

export function storyWorld<T>(build: (s: Scenario, w: World) => Promise<T>) {
  const live: Decorator = (Story, { loaded }) => (
    <Live api={(loaded.core as World).api}>
      <Story />
    </Live>
  );
  return {
    loaders: [
      async () => {
        const w = await newWorld();
        return { core: w, world: await build(scenario(w), w) };
      },
    ],
    decorators: [live],
    // Storybook runs a story's cleanups when it leaves or re-renders it.
    beforeEach: ({ loaded }: { loaded: Record<string, unknown> }) => () => (loaded.core as World).drop(),
  };
}
