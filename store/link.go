package store

import "github.com/tomoram/code-central/domain"

import "fmt"

type linkFile struct {
	Integration   string   `yaml:"integration"`
	ID            string   `yaml:"id"`
	URL           string   `yaml:"url,omitempty"`
	ExternalState string   `yaml:"external_state"`
	Detached      bool     `yaml:"detached,omitempty"`
	Outbound      bool     `yaml:"outbound,omitempty"`
	Written       []string `yaml:"written,omitempty"`
}

func encodeLink(l domain.Link) *linkFile {
	if !l.Linked() {
		return nil
	}
	return &linkFile{Integration: l.Integration, ID: l.ID, URL: l.URL, ExternalState: string(l.ExternalState), Detached: l.Detached, Outbound: l.Outbound, Written: l.Written}
}

func decodeLink(f *linkFile) (domain.Link, *LoadError) {
	if f == nil {
		return domain.Link{}, nil
	}
	l := domain.Link{Integration: f.Integration, ID: f.ID, URL: f.URL, ExternalState: domain.ExternalState(f.ExternalState), Detached: f.Detached, Outbound: f.Outbound, Written: f.Written}
	switch {
	case l.Integration == "":
		return domain.Link{}, &LoadError{Field: "link.integration", Msg: "required"}
	case l.ID == "":
		return domain.Link{}, &LoadError{Field: "link.id", Msg: "required"}
	case !l.ExternalState.Valid():
		return domain.Link{}, &LoadError{Field: "link.external_state", Msg: fmt.Sprintf(
			"unknown external state %q (open, closed, deleted or out_of_scope)", f.ExternalState)}
	}
	return l, nil
}
