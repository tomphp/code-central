import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import type { UiLocation } from "../url";
import { ActivityDashboardBody } from "./ActivityDashboardBody";
import { FocusBody, type FocusBodyProps } from "./FocusBody";
import { dashboardData, focusData, type DashboardData, type FocusData } from "./storyData";

// On the WASM Core: Flaky build Failed in Acme and Login page In Review in
// Beta, read back as Focus Mode reads them. The world's Api is what the
// Focus Item's panel and + New ▾ act through; Skip, Exit and the links go
// nowhere.
const meta = {
  title: "Focus/FocusBody",
  parameters: { layout: "fullscreen" },
  ...storyWorld(focusData),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const ALL: UiLocation = { view: "focus" };
const ACME: UiLocation = { view: "focus", project: "acme" };

// body renders a story of the world's session, with more over its props,
// from the world and whatever else the story loaded, and over its session.
type More = (f: FocusData, loaded: Record<string, unknown>) => Partial<FocusBodyProps>;
const body = (more: More = () => ({}), session: Partial<FocusBodyProps["session"]> = {}): Story => ({
  render: (_, { loaded }) => {
    const f = loaded.world as FocusData;
    const s: FocusBodyProps["session"] = {
      item: f.item,
      ticket: f.ticket,
      api: f.api,
      remaining: f.remaining,
      clear: false,
      onlySkipped: false,
      skip: () => {},
      again: () => {},
      say: () => {},
      ...session,
    };
    return <FocusBody api={f.api} now={f.now} data={f.data} session={s} loc={ALL} navigate={() => {}} {...more(f, loaded)} />;
  },
});

const clear = { item: undefined, ticket: undefined, remaining: 0, clear: true };

// The queue: its first Focus Item, and "N left" in the header.
export const QueueAllProjects = body();
// One Project's: its name in the header.
export const QueueOneProject = body(() => ({ loc: ACME }));
export const WithNote = body(() => ({}), { note: "Requirement added" });
export const AllClearAllProjects = body(() => ({}), clear);
export const AllClearOneProject = body(() => ({ loc: ACME }), clear);
// The Activity Dashboard in place of All clear, when it is chosen, with
// its own story's data.
export const ActivityDashboardInPlace: Story = {
  loaders: [async () => ({ d: await dashboardData() })],
  ...body((_, { d }) => {
    const { now, data, ciRuns } = d as DashboardData;
    return { dashboard: <ActivityDashboardBody now={now} data={data} changes={{}} ciRuns={{ data: ciRuns }} loc={ALL} navigate={() => {}} /> };
  }, clear),
};
export const OnlySkipped = body(() => ({}), { item: undefined, onlySkipped: true });
// Before the Inbox data: no count yet.
export const LoadingAllProjects = body(() => ({ data: undefined }), { item: undefined, remaining: 0 });
export const LoadingOneProject = body(() => ({ data: undefined, loc: ACME }), { item: undefined, remaining: 0 });
