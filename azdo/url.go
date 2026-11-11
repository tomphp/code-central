package azdo

import "github.com/tomoram/code-central/azdourl"

// ParseProjectURL turns what a human typed into the stored project URL.
// It lives in azdourl, which Code Central's core also uses to validate a
// project URL, as core can't depend on this package.
func ParseProjectURL(text string) (string, error) { return azdourl.ParseProjectURL(text) }
