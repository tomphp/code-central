package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/core"
)

// Blockers, Replacements and finding work (ticket 28).
func init() {
	for name, route := range map[string]struct {
		pattern string
		h       handlerFunc
	}{
		"add_blocker": {"POST /api/p/{key}/tickets/{n}/add_blocker", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.BlockerInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.AddBlocker(r.Context(), r.PathValue("key"), in)
		}},
		"remove_blocker": {"POST /api/p/{key}/tickets/{n}/remove_blocker", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.BlockerInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.RemoveBlocker(r.Context(), r.PathValue("key"), in)
		}},
		"cancel": {"POST /api/p/{key}/tickets/{n}/cancel", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.CancelInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.Cancel(r.Context(), r.PathValue("key"), in)
		}},
		"get_spec": {"GET /api/p/{key}/specs/{n}", func(c *core.Core, r *http.Request) (any, error) {
			q := r.URL.Query()
			if err := onlyParams(q, "part", "section"); err != nil {
				return nil, err
			}
			return c.GetSpec(r.Context(), r.PathValue("key"), core.GetSpecInput{Spec: core.Ref(r.PathValue("n")), Parts: q["part"], Sections: q["section"]})
		}},
		"find_specs": {"GET /api/p/{key}/specs", func(c *core.Core, r *http.Request) (any, error) {
			q := r.URL.Query()
			if err := onlyParams(q, "title", "label", "tag", "finished", "requirements"); err != nil {
				return nil, err
			}
			in := core.FindSpecsInput{Title: q.Get("title"), Labels: q["label"]}
			var err error
			if in.Tags, err = tagParams(q); err != nil {
				return nil, err
			}
			finished, err := boolParam(q, "finished")
			if err != nil {
				return nil, err
			}
			in.Finished = finished != nil && *finished
			requirements, err := boolParam(q, "requirements")
			if err != nil {
				return nil, err
			}
			in.Requirements = requirements != nil && *requirements
			return c.FindSpecs(r.Context(), r.PathValue("key"), in)
		}},
	} {
		Routes[name] = route.pattern
		handlers[name] = route.h
	}
}

// tagParams reads repeated tag=<key>=<value> query parameters.
func tagParams(q url.Values) (map[string]string, error) {
	if len(q["tag"]) == 0 {
		return nil, nil
	}
	tags := map[string]string{}
	for _, p := range q["tag"] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, &core.Error{Code: core.Invalid, Field: "tag", Message: fmt.Sprintf("a Tag filter is tag=<key>=<value>, not %q", p)}
		}
		tags[k] = v
	}
	return tags, nil
}

// boolParam reads an optional true/false query parameter.
func boolParam(q url.Values, name string) (*bool, error) {
	if !q.Has(name) {
		return nil, nil
	}
	b, err := strconv.ParseBool(q.Get(name))
	if err != nil {
		return nil, &core.Error{Code: core.Invalid, Field: name, Message: fmt.Sprintf("%s is true or false, not %q", name, q.Get(name))}
	}
	return &b, nil
}
