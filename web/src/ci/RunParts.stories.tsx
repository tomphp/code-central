import type { Meta, StoryObj } from "@storybook/react-vite";
import { CONCLUSION_LABEL, IgnoredMark, RunLink, RunStatusBadge } from "./RunParts";
import { finished } from "./ciFixtures";

const meta = { title: "CI/Run parts" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

// The status badge for every status a CI Run shows.
export const StatusBadges: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
      {["queued", "running", ...Object.values(CONCLUSION_LABEL)].map((s) => (
        <RunStatusBadge key={s} status={s} />
      ))}
    </div>
  ),
};

// The ignored mark, in an ignored entry, which greys it.
export const Ignored: Story = {
  render: () => (
    <span className="ci-entry ignored">
      <RunStatusBadge status="failed" />
      <IgnoredMark />
    </span>
  ),
};

// The link to the run, worded for each CI Provider.
export const RunLinks: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 16 }}>
      <RunLink run={finished("acme", 1)} provider="github_actions" />
      <RunLink run={finished("acme", 1)} provider="azure_pipelines" />
    </div>
  ),
};
