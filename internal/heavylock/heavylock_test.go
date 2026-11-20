package main

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestSecondRunWaitsAndNamesTheHolder(t *testing.T) {
	base := filepath.Join(t.TempDir(), "lock")
	release, err := acquire(base, "code-central-1186's make test", &bytes.Buffer{}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	got := make(chan struct{})
	go func() {
		r, err := acquire(base, "code-central-1187's make e2e", &out, time.Millisecond)
		if err != nil {
			t.Error(err)
		} else {
			r()
		}
		close(got)
	}()

	select {
	case <-got:
		t.Fatal("second run took the lock while the first held it")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("second run never got the lock after release")
	}
	if want := "waiting for code-central-1186's make test\n"; out.String() != want {
		t.Fatalf("waiting line = %q, want %q", out.String(), want)
	}
}
