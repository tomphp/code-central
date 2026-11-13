// The component browser (`just serve-components`): a dev-only Storybook that
// shows the UI's shared components on their own. Nothing here is imported by
// src/main.tsx, so it never reaches the embedded dist or `make test`.
import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  framework: "@storybook/react-vite",
  stories: ["../src/**/*.stories.tsx"],
  core: { disableTelemetry: true },
};

export default config;
