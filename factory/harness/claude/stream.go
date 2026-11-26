package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory/harness"
)

// streamMessage is the part of a stream-json message the Harness reads.
type streamMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	// Error is an assistant message's API error, or an api_retry's.
	Error string `json:"error"`
	// ParentToolUseID is the Agent call a subagent's message runs under;
	// empty for the main session's.
	ParentToolUseID string `json:"parent_tool_use_id"`
	// RateLimitInfo is a rate_limit_event's, as readPlanLimits reads
	// it; kept raw so a shape it doesn't expect never loses the line.
	RateLimitInfo json.RawMessage `json:"rate_limit_info"`
	Message       struct {
		// ID, Model and Usage are an assistant message's model
		// response's.
		ID      string       `json:"id"`
		Model   string       `json:"model"`
		Usage   *streamUsage `json:"usage"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			// ToolUseID, Content and IsError are a tool result's.
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`
}

// readStream reads t's stream-json to its end, telling t's callbacks of
// each Session Event, model response and Plan Limits it maps to, in
// order; now times each.
func readStream(stdout io.Reader, t harness.Turn, now func() time.Time) end {
	var e end
	var retryErr string
	usage := newUsageReader(t.UsageKind, now)
	tools := map[string]string{}
	hookRefused := map[string]bool{}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg streamMessage
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		if r, ok := usage.read(msg); ok {
			t.OnUsage(r)
		}
		if pl, ok := readPlanLimits(msg, now()); ok {
			t.OnPlanLimits(pl)
		}
		for _, ev := range streamEvents(msg, now(), tools, t.Dir) {
			if ev.Kind == api.EventText {
				e.lastText = ev.Text
			}
			t.OnEvent(ev)
		}
		switch msg.Type {
		case "user":
			for _, c := range msg.Message.Content {
				if c.Type == "tool_result" && c.IsError && isHookRefusal(c.Content) {
					hookRefused[c.ToolUseID] = true
				}
			}
		case "system":
			if msg.Subtype == "api_retry" && msg.Error != "" {
				retryErr = msg.Error
			}
		case "assistant":
			if msg.Error != "" {
				e.apiError = msg.Error
			}
			for _, c := range msg.Message.Content {
				if c.Type == "tool_use" && calledReview(c.Name, c.Input) {
					e.reviewCalled = true
				}
			}
		case "result":
			var r result
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				// A PreToolUse hook's refusal is listed as a denial
				// too, but no --allowedTools rule lifts it.
				r.PermissionDenials = slices.DeleteFunc(r.PermissionDenials, func(d denial) bool { return hookRefused[d.ID] })
				e.result = &r
			}
		}
	}
	if e.lastText == "" && e.result != nil {
		e.lastText = e.result.Result
	}
	// A retry that later succeeded is no error; one the Turn never got
	// past is.
	if e.apiError == "" && (e.result == nil || e.result.IsError) {
		e.apiError = retryErr
	}
	return e
}

// isHookRefusal reports whether a tool result's content, a string or a
// list of text blocks, is the error a PreToolUse hook's refusal gives:
// text starting "PreToolUse:".
func isHookRefusal(content json.RawMessage) bool {
	var text string
	if json.Unmarshal(content, &text) != nil {
		var blocks []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(content, &blocks) != nil || len(blocks) == 0 {
			return false
		}
		text = blocks[0].Text
	}
	return strings.HasPrefix(strings.TrimSpace(text), "PreToolUse:")
}
