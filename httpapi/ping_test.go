package httpapi_test

import "testing"

func TestPingAlwaysAnswers204(t *testing.T) {
	h := newHandler(t)
	for _, body := range []string{`{"session_id":"s1"}`, `{"session_id":"unknown"}`, `{}`, `{`, `{"other":1}`} {
		if rec := do(h, "POST", "/api/ping", body); rec.Code != 204 || rec.Body.Len() != 0 {
			t.Errorf("ping %s: %d %q, want 204 with no body", body, rec.Code, rec.Body)
		}
	}
	if rec := do(h, "GET", "/api/ping", ""); rec.Code == 204 {
		t.Error("GET /api/ping should not ping: GET never writes")
	}
}
