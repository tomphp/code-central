import type { Meta, StoryObj } from "@storybook/react-vite";
import { Count } from "./Count";
import { Empty, Loading } from "./Empty";
import { ErrorBanner } from "./ErrorBanner";
import { ProjectName } from "./ProjectName";
import { SpecPaneHeader } from "./SpecPaneHeader";

const meta = { title: "Controls/Shared notes" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const EmptyNote: Story = { render: () => <Empty>No Tickets yet.</Empty> };
export const LoadingNote: Story = { render: () => <Loading /> };
export const Error: Story = { render: () => <ErrorBanner>Something went wrong.</ErrorBanner> };
export const Counts: Story = {
  render: () => (
    <p>
      Board <Count n={4} /> Needs Me <Count n={2} loud />
    </p>
  ),
};
// Focus Mode's "N left".
export const CountOf: Story = { render: () => <Count n={2} of="left" /> };
// A count block's number, as the Activity Dashboard's, and its dash before the count is known.
export const BigCounts: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 24, background: "var(--s-ready)", color: "#fff", padding: 24 }}>
      <Count n={12} big />
      <Count big />
    </div>
  ),
};
export const Project: Story = { render: () => <ProjectName name="Acme Web" projectKey="acme" /> };
export const SpecHeader: Story = {
  render: () => <SpecPaneHeader id="acme-12" kind="Spec" title="Storybook coverage" now={new Date()} created={new Date(Date.now() - 3 * 3600_000).toISOString()} />,
};
