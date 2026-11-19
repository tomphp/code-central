package api

// AttentionTicket is one Ticket as notify watches it.
type AttentionTicket struct {
	Project        string
	ID             string
	Title          string
	Rounds         int
	FailureReports int
	// SilentClaim is the number of the live Claim while it is Silent,
	// else 0.
	SilentClaim int
	// LongClaim is the number of the live Claim while it is Long, else 0.
	LongClaim int
	// CIFailedCommit is the pushed commit while the Ticket is CI failed,
	// else "".
	CIFailedCommit string
	NeedsMe        bool
}
