import type { Meta, StoryObj } from "@storybook/react-vite";
import { NEEDS_ME_REASONS, STATES } from "../api/types";
import { KindBadge } from "./KindBadge";
import { ReasonBadge } from "./ReasonBadge";
import { StageBadge } from "./StageBadge";
import { StateChip } from "./StateChip";

const meta = { title: "Controls/Badges" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const States: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
      {STATES.map((s) => (
        <StateChip key={s} state={s} />
      ))}
    </div>
  ),
};

// A State chip marking the State a row is listed under, and one with a count.
export const StateHere: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8 }}>
      <StateChip state="In Progress" here />
      <StateChip state="Ready">Ready 3</StateChip>
    </div>
  ),
};

// The Reason badge for every Needs Me reason.
export const Reasons: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
      {NEEDS_ME_REASONS.map((r) => (
        <ReasonBadge key={r} reason={r} />
      ))}
    </div>
  ),
};

// The Focus badge: the Reason badge with the "Focus: " prefix.
export const Focus: Story = { render: () => <ReasonBadge reason="needs_answer" prefix="Focus: " /> };

export const Kinds: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8 }}>
      {["Requirement", "Map", "Long-lived", "Spec"].map((k) => (
        <KindBadge key={k} kind={k} />
      ))}
    </div>
  ),
};

export const Stage: Story = { render: () => <StageBadge label="Implementation" /> };
