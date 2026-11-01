package cli

import (
	"strings"
	"testing"

	"abstrax/internal/services/cron"
)

func TestCronRunSummaryMentionsDisabled(t *testing.T) {
	code := 0
	summary := cronRunSummary(&cron.RunResult{
		ID:         "backup",
		Disabled:   true,
		ExitCode:   &code,
		DurationMS: 1500,
	})
	if !strings.Contains(summary, "disabled") {
		t.Fatalf("summary = %q", summary)
	}
	if !strings.Contains(summary, "exit code 0") {
		t.Fatalf("summary = %q", summary)
	}
}
