package domain_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
)

func TestAClaimIsStoppedWhileItIsLiveAndItsLatestStopHasNoResume(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name  string
		claim domain.Claim
		want  bool
	}{
		{"no Stops", domain.Claim{}, false},
		{"latest Stop not Resumed", domain.Claim{Stops: []domain.Stop{{Stopped: at}}}, true},
		{"latest Stop Resumed", domain.Claim{Stops: []domain.Stop{{Stopped: at, Resumed: at}}}, false},
		{"stopped again after a Resume", domain.Claim{Stops: []domain.Stop{{Stopped: at, Resumed: at}, {Stopped: at}}}, true},
		{"ended while Stopped", domain.Claim{Ended: at, Stops: []domain.Stop{{Stopped: at}}}, false},
	}
	for _, c := range cases {
		if got := c.claim.Stopped(); got != c.want {
			t.Errorf("%s: Stopped() = %v, want %v", c.name, got, c.want)
		}
	}
}
