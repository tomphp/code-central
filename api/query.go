package api

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// DecodeQuery reads the Tag filter from q's tag parameters.
func (in *FindTicketsInput) DecodeQuery(q url.Values) (err error) {
	in.Tags, err = tagParams(q)
	return err
}

// EncodeQuery adds the Tag filter to q as tag parameters.
func (in FindTicketsInput) EncodeQuery(q url.Values) { addTagParams(q, in.Tags) }

// DecodeQuery reads the Tag filter from q's tag parameters.
func (in *FindSpecsInput) DecodeQuery(q url.Values) (err error) {
	in.Tags, err = tagParams(q)
	return err
}

// EncodeQuery adds the Tag filter to q as tag parameters.
func (in FindSpecsInput) EncodeQuery(q url.Values) { addTagParams(q, in.Tags) }

// tagParams reads repeated tag=<key>=<value> query parameters.
func tagParams(q url.Values) (map[string]string, error) {
	if len(q["tag"]) == 0 {
		return nil, nil
	}
	tags := map[string]string{}
	for _, p := range q["tag"] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, &Error{Code: Invalid, Field: "tag", Message: fmt.Sprintf("a Tag filter is tag=<key>=<value>, not %q", p)}
		}
		tags[k] = v
	}
	return tags, nil
}

// addTagParams adds each Tag as a tag=<key>=<value> query parameter, in
// key order.
func addTagParams(q url.Values, tags map[string]string) {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		q.Add("tag", k+"="+tags[k])
	}
}
