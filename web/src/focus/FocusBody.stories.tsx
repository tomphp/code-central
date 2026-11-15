import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import type { UiLocation } from "../url";
import { ActivityDashboardBody } from "./ActivityDashboardBody";
import { FocusBody, type FocusBodyProps, type FocusBodySession } from "./FocusBody";
import { dashboardData, focusData, focusSession, type DashboardData, type FocusData } from "./storyData";

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

// body renders a story of the world's session with over over it, and with
// props over the body's, made from the world and whatever else the story
// loaded.
type Props = (world: FocusData, loaded: Record<string, unknown>) => Partial<FocusBodyProps>;
const body = (props: Props = () => ({}), over: Partial<FocusBodySession> = {}): Story => ({
  render: (_, { loaded }) => {
    const world = loaded.world as FocusData;
    return (
      <FocusBody api={world.api} now={world.now} data={world.data} session={focusSession(world, over)} loc={ALL} navigate={() => {}} {...props(world, loaded)} />
    );
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
// The next Focus Item on its way: the count, and Loading… under it.
export const NextItemLoading = body(() => ({}), { item: undefined, ticket: undefined });
// Before the Inbox data: no count yet.
export const LoadingAllProjects = body(() => ({ data: undefined }), { item: undefined, remaining: 0 });
export const LoadingOneProject = body(() => ({ data: undefined, loc: ACME }), { item: undefined, remaining: 0 });
