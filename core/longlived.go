package core

import (
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// projectSpec is one of a Project's own Long-lived Specs, created on first
// use and found only by the number the Project stores for it (ADR 0035):
// the CI Fixes, Small Changes and Retros Specs.
type projectSpec struct {
	title, body string
	// number is the Project's field holding the Spec's number, 0 until it
	// exists.
	number func(*domain.Project) *int
}

// The Project's own Long-lived Specs.
var (
	ciFixesSpec      = projectSpec{"CI Fixes", ciFixesSpecBody, func(p *domain.Project) *int { return &p.CIFixesSpec }}
	smallChangesSpec = projectSpec{"Small Changes", smallChangesSpecBody, func(p *domain.Project) *int { return &p.SmallChangesSpec }}
	retrosSpec       = projectSpec{"Retros", retrosSpecBody, func(p *domain.Project) *int { return &p.RetrosSpec }}
)

// projectSpecNumber is the number of key's Spec ps, created on first use;
// the Spec is written before the Project names it, so a failure in
// between leaves at most an unused Spec.
func (c *Core) projectSpecNumber(key string, ps projectSpec) (int, error) {
	n := 0
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if p, _ := w.Project(key); *ps.number(&p) != 0 {
			n = *ps.number(&p)
			return nil
		}
		now := c.now()
		n = w.NextNumber(key)
		return w.PutSpec(key, domain.Spec{Number: n, Title: ps.title, Body: ps.body, Labels: []string{LongLivedLabel}, Created: now, Updated: now})
	})
	if err != nil {
		return 0, err
	}
	err = c.store.Update(func(w store.Writer) error {
		p, _ := w.Project(key)
		if *ps.number(&p) != 0 {
			n = *ps.number(&p)
			return nil
		}
		*ps.number(&p) = n
		return w.PutProject(p)
	})
	return n, err
}
