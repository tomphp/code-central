// The Reported Models (Spec code-central-301): at startup the Factory
// asks its Harness which effort levels and model aliases it accepts,
// checks its own --effort against the levels, and reports both to Code
// Central with its flags.

package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/api"
)

// checkEffortFlag refuses an --effort outside the levels the Harness
// accepts.
func (f *factory) checkEffortFlag() error {
	if f.accepts.TakesEffort(f.o.effort) {
		return nil
	}
	return fmt.Errorf("--effort %q isn't an effort level %s accepts: use one of %s", f.o.effort, f.harness.Name(), strings.Join(f.accepts.Efforts, ", "))
}

// reportModels reports what the Harness accepts with the Factory's
// flags. It reports whether Code Central took it; if not, the next poll
// tries again.
func (f *factory) reportModels(ctx context.Context) bool {
	in := api.ReportModelsInput{EffortLevels: f.accepts.Efforts, Aliases: f.accepts.Aliases, ModelFlag: f.o.model, EffortFlag: f.o.effort}
	if _, err := f.client.ReportModels(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't report what %s accepts to Code Central: %v", f.harness.Name(), err)
		}
		return false
	}
	return true
}
