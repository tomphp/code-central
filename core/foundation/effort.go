package foundation

import (
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// seededEffortLevels are the valid effort levels until the Factory has
// reported its claude's, so effort can be set on a fresh store.
var seededEffortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// ValidEffortLevels are the valid effort levels given r: its levels, or
// the seeded ones while it has none.
func ValidEffortLevels(r domain.ReportedModels) []string {
	if len(r.EffortLevels) == 0 {
		return seededEffortLevels
	}
	return r.EffortLevels
}

// CheckEffort rejects an effort outside the Project's valid levels, in
// field, listing them.
func CheckEffort(r store.Reader, key, field, effort string) error {
	p, _ := r.Project(key)
	levels := ValidEffortLevels(p.ReportedModels)
	if !slices.Contains(levels, effort) {
		return api.InvalidField(field, "effort %q isn't a valid level; use one of %s", effort, strings.Join(levels, ", "))
	}
	return nil
}
