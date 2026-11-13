import { ToggleGroup } from "../controls/ToggleGroup";

// ListToggleBar is the bar above a Ticket list: the grouping, and on the
// All Tickets list the ID search and Show finished too.
export function ListToggleBar(p: {
  tickets: boolean;
  search: string;
  onSearch(search: string): void;
  grouping: "spec" | "ticket";
  onGroup(g: "spec" | "ticket"): void;
  showFinished: boolean;
  onShowFinished(on: boolean): void;
}) {
  return (
    <div className="bs-toggle-bar">
      {p.tickets && (
        <input
          type="search"
          className="bs-search"
          aria-label="Ticket ID"
          placeholder="Ticket ID, e.g. 452"
          value={p.search}
          onChange={(e) => p.onSearch(e.target.value)}
        />
      )}
      <ToggleGroup
        label="Group the list"
        options={[
          { value: "ticket", label: "Ticket" },
          { value: "spec", label: "Spec" },
        ]}
        value={p.grouping}
        onChange={p.onGroup}
      />
      {p.tickets && (
        <div className="bs-toggle">
          <button
            aria-pressed={p.showFinished}
            onClick={() => p.onShowFinished(!p.showFinished)}
          >
            Show finished
          </button>
        </div>
      )}
    </div>
  );
}
