package serverinfo

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	executil "abstrax/internal/exec"
)

func TestRebootNoDelayUsesSystemctl(t *testing.T) {
	runner := dryRunRunner()
	svc := &Service{runner: runner}

	var emitted *RebootResult
	res, err := svc.Reboot(context.Background(), RebootOptions{
		Exists: func(string) bool { return false },
	}, func(result *RebootResult) error {
		emitted = result
		if got := runner.Commands(); len(got) != 1 || got[0][0] != "logger" {
			t.Fatalf("journal not recorded before result: %#v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if emitted == nil || !res.Scheduled || res.DelaySeconds != 0 || res.Command != "systemctl reboot" {
		t.Fatalf("result = %+v emitted = %+v", res, emitted)
	}

	want := [][]string{
		{"logger", "-t", "abstrax", "Restart requested through Abstrax"},
		{"systemctl", "reboot"},
	}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestRebootNoDelayPrefersSystemdCat(t *testing.T) {
	runner := dryRunRunner()
	svc := &Service{runner: runner}

	_, err := svc.Reboot(context.Background(), RebootOptions{
		Exists: func(name string) bool { return name == "systemd-cat" },
	}, func(*RebootResult) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"systemd-cat", "-t", "abstrax", "--", "echo", "Restart requested through Abstrax"},
		{"systemctl", "reboot"},
	}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestRebootDelayUsesSystemdRun(t *testing.T) {
	runner := dryRunRunner()
	svc := &Service{runner: runner}

	res, err := svc.Reboot(context.Background(), RebootOptions{
		DelaySeconds: 90,
		Exists:       func(name string) bool { return name == "systemd-run" },
	}, func(result *RebootResult) error {
		cmds := runner.Commands()
		if len(cmds) != 1 {
			t.Fatalf("reboot recorded before result: %#v", cmds)
		}
		if result.Command != "systemd-run --on-active=90s -- systemctl reboot" {
			t.Fatalf("command = %q", result.Command)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scheduled || res.DelaySeconds != 90 {
		t.Fatalf("result = %+v", res)
	}

	want := [][]string{
		{"logger", "-t", "abstrax", "Restart requested through Abstrax in 90 seconds"},
		{"systemd-run", "--on-active=90s", "--", "systemctl", "reboot"},
	}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestRebootDelayZeroMatchesImmediate(t *testing.T) {
	// --delay 0 is the same command as omitting the delay, even when
	// systemd-run is available.
	plan, err := rebootPlan(0, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if plan.display != "systemctl reboot" || !reflect.DeepEqual(plan.reboot, []string{"systemctl", "reboot"}) {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestRebootShutdownMinutesRoundedUp(t *testing.T) {
	cases := []struct {
		seconds int
		minutes int
	}{
		{1, 1},
		{59, 1},
		{60, 1},
		{61, 2},
		{90, 2},
		{120, 2},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%ds", tc.seconds), func(t *testing.T) {
			plan, err := rebootPlan(tc.seconds, func(string) bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			wantDisplay := fmt.Sprintf("shutdown -r +%d", tc.minutes)
			want := []string{"shutdown", "-r", fmt.Sprintf("+%d", tc.minutes)}
			if plan.display != wantDisplay || !reflect.DeepEqual(plan.reboot, want) {
				t.Fatalf("plan display %q argv %#v", plan.display, plan.reboot)
			}
		})
	}
}

func TestRebootSystemdRunPreferredForWholeMinutes(t *testing.T) {
	plan, err := rebootPlan(60, func(name string) bool { return name == "systemd-run" })
	if err != nil {
		t.Fatal(err)
	}
	if plan.display != "systemd-run --on-active=60s -- systemctl reboot" {
		t.Fatalf("display = %q", plan.display)
	}
}

func TestRebootNegativeDelay(t *testing.T) {
	runner := dryRunRunner()
	svc := &Service{runner: runner}
	_, err := svc.Reboot(context.Background(), RebootOptions{DelaySeconds: -1}, func(*RebootResult) error {
		t.Fatal("emitted a result for a negative delay")
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if cmds := runner.Commands(); len(cmds) != 0 {
		t.Fatalf("recorded commands for a negative delay: %#v", cmds)
	}
}

func TestRebootDoesNotRunWhenResultFails(t *testing.T) {
	runner := dryRunRunner()
	svc := &Service{runner: runner}
	_, err := svc.Reboot(context.Background(), RebootOptions{
		Exists: func(string) bool { return false },
	}, func(*RebootResult) error {
		return fmt.Errorf("print failed")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	got := runner.Commands()
	if len(got) != 1 || got[0][0] != "logger" {
		t.Fatalf("commands = %#v", got)
	}
}

func TestRebootDryRunRecordsWithoutExecuting(t *testing.T) {
	runner := dryRunRunner()
	svc := &Service{runner: runner}

	_, err := svc.Reboot(context.Background(), RebootOptions{
		DelaySeconds: 90,
		Exists:       func(string) bool { return false },
	}, func(*RebootResult) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	got := runner.Commands()
	if len(got) != 2 || got[0][0] != "logger" || got[1][0] != "shutdown" || got[1][2] != "+2" {
		t.Fatalf("commands = %#v", got)
	}
}

func dryRunRunner() *executil.Runner {
	return executil.New(true, false)
}
