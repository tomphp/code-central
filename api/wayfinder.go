package api

import "slices"

// NodeType is a Ticket's type on a Map (NodeTypeOf): one of the
// WayfinderTypes, prototype, or ticket for a Ticket with neither.
type NodeType string

// The node types.
const (
	NodeChart     NodeType = "chart"
	NodeGrilling  NodeType = "grilling"
	NodeResearch  NodeType = "research"
	NodeTask      NodeType = "task"
	NodePrototype NodeType = "prototype"
	NodeTicket    NodeType = "ticket"
)

// WayfinderTypes are the Map's Ticket types, each named as its Label's
// suffix.
func WayfinderTypes() []NodeType { return []NodeType{NodeChart, NodeGrilling, NodeResearch, NodeTask} }

// NodeTypes are the node types: the WayfinderTypes, then prototype and
// ticket.
var NodeTypes = append(slices.Clone(WayfinderTypes()), NodePrototype, NodeTicket)
