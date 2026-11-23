package store

import "github.com/tomoram/code-central/domain"

type workItemStatesFile struct {
	InProgress *workItemStateFile `yaml:"in_progress,omitempty"`
	Accepted   *workItemStateFile `yaml:"accepted,omitempty"`
	Done       *workItemStateFile `yaml:"done,omitempty"`
}

type workItemStateFile struct {
	Name  string `yaml:"name"`
	Since string `yaml:"since"`
}

// encodeWorkItemStates leaves a blank mapping out, and all of them while
// every one is blank.
func encodeWorkItemStates(s domain.WorkItemStates) *workItemStatesFile {
	f := workItemStatesFile{InProgress: encodeWorkItemState(s.InProgress), Accepted: encodeWorkItemState(s.Accepted), Done: encodeWorkItemState(s.Done)}
	if f == (workItemStatesFile{}) {
		return nil
	}
	return &f
}

func encodeWorkItemState(s domain.WorkItemState) *workItemStateFile {
	if s.Name == "" {
		return nil
	}
	return &workItemStateFile{Name: s.Name, Since: formatTime(s.Since)}
}

// decodeWorkItemStates reads each mapping, naming the one whose since
// time doesn't parse by its key.
func decodeWorkItemStates(f *workItemStatesFile) (s domain.WorkItemStates, field string, err error) {
	if f == nil {
		return s, "", nil
	}
	for _, m := range []struct {
		key  string
		file *workItemStateFile
		into *domain.WorkItemState
	}{{"in_progress", f.InProgress, &s.InProgress}, {"accepted", f.Accepted, &s.Accepted}, {"done", f.Done, &s.Done}} {
		if m.file == nil {
			continue
		}
		since, err := parseTime(m.file.Since)
		if err != nil {
			return s, m.key + ".since", err
		}
		*m.into = domain.WorkItemState{Name: m.file.Name, Since: since}
	}
	return s, "", nil
}
