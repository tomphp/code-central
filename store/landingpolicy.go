package store

// Normalized is l as it is stored: Push is implied under Pull Request, so
// it is cleared there and never kept.
func (l Landing) Normalized() Landing {
	if l.Method == LandingPullRequest {
		l.Push = false
	}
	return l
}
