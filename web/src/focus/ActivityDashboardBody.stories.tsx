import type { Meta, StoryObj } from "@storybook/react-vite";
import { ApiError } from "../api/types";
import type { UiLocation } from "../url";
import { ActivityDashboardBody, type ActivityDashboardBodyProps } from "./ActivityDashboardBody";
import { dashboardData, type DashboardData } from "./storyData";

// Acme and Beta read from a WASM Core world, as the Activity view shows
// them: Acme has CI Tracking on with two runs going. Links go nowhere.
const meta = {
  title: "Focus/ActivityDashboardBody",
  parameters: { layout: "fullscreen" },
  loaders: [async () => ({ d: await dashboardData() })],
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const ALL: UiLocation = { view: "activity" };
const ACME: UiLocation = { view: "activity", project: "acme" };

// body renders a story in the Activity view's shell, with more over the
// world's props.
const body = (more: (d: DashboardData) => Partial<ActivityDashboardBodyProps>): Story => ({
  render: (_, { loaded }) => {
    const d = loaded.d as DashboardData;
    return (
      <main className="view-shell">
        <ActivityDashboardBody now={d.now} data={d.data} changes={{}} loc={ALL} navigate={() => {}} {...more(d)} />
      </main>
    );
  },
});

// Counts and In Progress Tickets across every Project, each with its
// Project's name; no CI Running without CI Tracking in scope.
export const AllProjects = body(() => ({}));
export const OneProject = body(() => ({ loc: ACME }));
// CI Running beside the counts, its runs listed under the In Progress ones.
export const CIRunningAllProjects = body((d) => ({ ciRuns: { data: d.ciRuns } }));
export const CIRunningOneProject = body((d) => ({ ciRuns: { data: d.ciRuns }, loc: ACME }));
// Before the first read of the runs: a dash, and no list yet.
export const CIRunningLoading = body(() => ({ ciRuns: {}, loc: ACME }));
export const NoCIRunsRunning = body((d) => ({ ciRuns: { data: { ...d.ciRuns, runs: [] } }, loc: ACME }));
export const CIRunningError = body(() => ({ ciRuns: { error: new ApiError(503, "unavailable", "The CI Watcher can't be reached.") }, loc: ACME }));
export const NothingInProgress = body((d) => ({ data: { ...d.data, tickets: d.data.tickets.filter((t) => t.state !== "In Progress") } }));
