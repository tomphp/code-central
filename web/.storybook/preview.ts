// Load the app's global CSS and the area CSS the components lean on, so a
// story looks as it does in the app. The area files are listed here because
// a badge's rules can live in the area file of the view that first used it.
import type { Preview } from "@storybook/react-vite";
import "../src/styles.css";
import "../src/activity/activity.css";
import "../src/board/board.css";
import "../src/ci/ci.css";
import "../src/focus/focus.css";
import "../src/options/options.css";
import "../src/usage/usage.css";

const preview: Preview = {
  parameters: {
    controls: { matchers: { color: /(background|color)$/i } },
  },
};

export default preview;
