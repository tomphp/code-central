package api

import "github.com/tomoram/code-central/domain"

// SubmitRequirementInput is submit_requirement's input: the Requirement's
// free text, whose first line becomes the Spec's title.
type SubmitRequirementInput struct {
	Text string `json:"text" desc:"The Requirement's text, in Markdown; its first line becomes the Spec's title." required:"true"`
	// Map is the finished Map the Requirement is written from, if any
	// (Write the Spec): its Decisions take the grilling Stage's place.
	Map Ref `json:"map,omitempty" desc:"A Map with no unfinished Tickets to write the Spec from: the Requirement gets only the spec and tickets Stages, and a map Tag naming the Map."`
	// SkipGrilling is Straight to Spec: no grilling Stage, so the spec
	// Stage works from the text alone. It can't be given with Map.
	SkipGrilling bool `json:"skip_grilling,omitempty" desc:"Straight to Spec: create no grilling Stage, so the spec Stage is written from the text alone. Can't be given with map."`
	// Full asks for the Spec and Stage Tickets in place of their
	// Acknowledgements.
	Full bool `json:"full,omitempty" desc:"Return the whole Spec and Stage Tickets instead of their acknowledgements."`
	// Draft saves every Ticket the submit writes as Draft, not Ready, so
	// nothing is claimed until the human moves them to Ready.
	Draft bool `json:"draft,omitempty" desc:"Save the Spec's Tickets as Draft instead of Ready, so nothing is claimed until the human moves them to Ready; not with map."`
}

// SubmitRequirementResult is the new Requirement: its Spec and its Stage
// Tickets (three, or two when made from a Map), in order, each
// acknowledged, or with full each the record.
type SubmitRequirementResult struct {
	Spec   Written[SpecView]     `json:"spec"`
	Stages []Written[TicketView] `json:"stages"`
}

// RequirementView is the requirement block on a Spec read, present only
// when the Spec is a Requirement: it has at least one Stage Ticket.
// Nothing in it is stored.
type RequirementView struct {
	// Map is the Map the Requirement was made from, from its map Tag, set
	// only when it has no grilling Stage.
	Map string `json:"map,omitempty"`
	// Stages are the Stage Tickets in Stage order.
	Stages []StageView `json:"stages"`
	// Build is how far the implementation Tickets have got.
	Build BuildProgress `json:"build"`
	// WaitingForFactory is whether the next Ticket can be claimed but
	// nothing has claimed it.
	WaitingForFactory bool `json:"waiting_for_factory"`
	// Segments are the Stage strip's: one per Stage, then Build, with a
	// first grilling segment for a Requirement made from a Map (done) or
	// one submitted Straight to Spec (skipped).
	Segments []Segment `json:"segments"`
	// Current is the Stage of the first segment neither done nor skipped,
	// else build.
	Current SegmentStage `json:"current"`
	// Needs are every Ticket of the Requirement waiting on the human: the
	// Stage Tickets in Stage order, then the Build Tickets in claim order.
	Needs []RequirementNeed `json:"needs"`
	// DefaultTicket is the Ticket the Requirement opens on: the first of
	// Needs, else the current segment's, else the first unfinished Build
	// Ticket, else the last Build Ticket, else the last segment's with one.
	DefaultTicket string `json:"default_ticket,omitempty"`
}

// SegmentStage is a Stage strip segment's Stage: a Stage's name, or build.
type SegmentStage string

// SegmentBuild is the Build segment's Stage.
const SegmentBuild SegmentStage = "build"

// SegmentStages are the segments' Stages in strip order.
func SegmentStages() []SegmentStage {
	return []SegmentStage{SegmentStage(domain.StageGrilling), SegmentStage(domain.StageSpec), SegmentStage(domain.StageTickets), SegmentBuild}
}

// SegmentProgress is how far a Stage strip segment has got.
type SegmentProgress string

// The segments' progress values (the glossary's Stage strip).
const (
	// ProgressDone is finished.
	ProgressDone SegmentProgress = "done"
	// ProgressSkipped is a Cancelled Stage, or the grilling of a
	// Requirement submitted Straight to Spec: passed over as a done one is.
	ProgressSkipped SegmentProgress = "skipped"
	// ProgressNeeds is waiting on the human.
	ProgressNeeds SegmentProgress = "needs"
	// ProgressQueued is claimable, waiting for a Factory.
	ProgressQueued SegmentProgress = "queued"
	// ProgressRunning is a Factory on it.
	ProgressRunning SegmentProgress = "running"
	// ProgressWaiting is not started, behind an earlier segment.
	ProgressWaiting SegmentProgress = "waiting"
)

// SegmentProgresses are the segments' progress values.
func SegmentProgresses() []SegmentProgress {
	return []SegmentProgress{ProgressDone, ProgressSkipped, ProgressNeeds, ProgressQueued, ProgressRunning, ProgressWaiting}
}

// Segment is one segment of a Requirement's Stage strip.
type Segment struct {
	Stage    SegmentStage    `json:"stage"`
	Progress SegmentProgress `json:"progress"`
	// Ticket is the Stage Ticket; Build and a Map's segment have none.
	Ticket string `json:"ticket,omitempty"`
	// Map is the Map the Requirement was made from, on the segment that
	// stands in for its grilling Stage.
	Map string `json:"map,omitempty"`
	// Reason is why the segment needs the human: its Stage Ticket's
	// reason, or Build's first need's.
	Reason NeedsMeReason `json:"reason,omitempty"`
}

// RequirementNeed is one Ticket of a Requirement waiting on the human,
// with its Stage when it is a Stage Ticket.
type RequirementNeed struct {
	Ticket string           `json:"ticket"`
	Reason NeedsMeReason    `json:"reason"`
	Stage  domain.StageName `json:"stage,omitempty"`
}

// StageView is one Stage Ticket: its Stage name, Ticket ID, State and
// Needs Me reason, if any.
type StageView struct {
	Name          domain.StageName `json:"name"`
	Ticket        string           `json:"ticket"`
	State         domain.State     `json:"state"`
	NeedsMeReason NeedsMeReason    `json:"needs_me_reason,omitempty"`
}

// BuildProgress is the implementation Tickets Done over their total,
// leaving out Cancelled ones.
type BuildProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}
