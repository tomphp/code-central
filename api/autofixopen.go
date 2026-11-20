package api

// AutoFixMarkView is an auto fix Ticket's Auto-fix mark as its view
// carries it.
type AutoFixMarkView struct {
	// Attempt is its Auto-fix attempt, 1 for the first in its chain.
	Attempt int `json:"attempt"`
	// Repository is the Repository Name of the commit it was opened for,
	// left out while the Project has the Home Repository alone.
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit"`
	// Covers are the Tickets its CI failed item listed when it was
	// opened.
	Covers []string `json:"covers"`
	// Previous is the previous attempt's fix Ticket, left out for attempt
	// 1.
	Previous string `json:"previous,omitempty"`
}
