// Load the app's global CSS and the area CSS the components lean on, so a
// story looks as it does in the app. The area files are listed here because
// a badge's rules can live in the area file of the view that first used it.
import type { Preview } from "@storybook/react-vite";
import "../src/styles.css";
import "../src/board/board.css";
import "../src/chat/chat.css";
import "../src/ci/ci.css";
import "../src/focus/focus.css";
import "../src/options/options.css";
import "../src/usage/usage.css";

// The web fonts load lazily, once text first uses them, so a capture of a story with little text could
// shoot before they arrive and show the fallback. Load them before each story renders.
const fonts = ['600 12px "Red Hat Mono Variable"', '400 14px "Hanken Grotesk Variable"', '600 14px "Hanken Grotesk Variable"'];

const preview: Preview = {
  loaders: [async () => { await Promise.all(fonts.map((f) => document.fonts.load(f))); }],
  parameters: {
    controls: { matchers: { color: /(background|color)$/i } },
  },
};

export default preview;
