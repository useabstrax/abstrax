package serverinfo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	executil "abstrax/internal/exec"
)

func TestZoneFromLocaltime(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink("../usr/share/zoneinfo/Europe/London", link); err != nil {
		t.Fatal(err)
	}
	if got := zoneFromLocaltime(link); got != "Europe/London" {
		t.Fatalf("zone = %q", got)
	}

	nested := filepath.Join(dir, "localtime-us")
	if err := os.Symlink("/usr/share/zoneinfo/America/New_York", nested); err != nil {
		t.Fatal(err)
	}
	if got := zoneFromLocaltime(nested); got != "America/New_York" {
		t.Fatalf("zone = %q", got)
	}
}

func TestTimezoneFileFallback(t *testing.T) {
	dir := t.TempDir()
	localtime := filepath.Join(dir, "localtime")
	tzFile := filepath.Join(dir, "timezone")
	if err := os.WriteFile(tzFile, []byte("Europe/London\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := testTimezoneService(t, localtime, tzFile, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	info := svc.timezone(context.Background())
	if info.Name != "Europe/London" {
		t.Fatalf("name = %q", info.Name)
	}
	if info.Abbreviation != "GMT" || info.UTCOffset != "+00:00" || info.UTCOffsetSeconds != 0 {
		t.Fatalf("winter offset = %+v", info)
	}
}

func TestTimezoneMissingFilesAreUTC(t *testing.T) {
	dir := t.TempDir()
	svc := testTimezoneService(t, filepath.Join(dir, "localtime"), filepath.Join(dir, "timezone"), time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	info := svc.timezone(context.Background())
	if info.Name != "UTC" || info.UTCOffset != "+00:00" || info.UTCOffsetSeconds != 0 {
		t.Fatalf("info = %+v", info)
	}
}

func TestTimezoneLondonDST(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink("/usr/share/zoneinfo/Europe/London", link); err != nil {
		t.Fatal(err)
	}

	summer := testTimezoneService(t, link, filepath.Join(dir, "timezone"), time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	info := summer.timezone(context.Background())
	if info.Name != "Europe/London" || info.Abbreviation != "BST" || info.UTCOffset != "+01:00" || info.UTCOffsetSeconds != 3600 {
		t.Fatalf("summer = %+v", info)
	}

	winter := testTimezoneService(t, link, filepath.Join(dir, "timezone"), time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	info = winter.timezone(context.Background())
	if info.Abbreviation != "GMT" || info.UTCOffset != "+00:00" || info.UTCOffsetSeconds != 0 {
		t.Fatalf("winter = %+v", info)
	}
}

func TestTimedatectlWinsOverLocaltime(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink("/usr/share/zoneinfo/America/New_York", link); err != nil {
		t.Fatal(err)
	}
	svc := testTimezoneService(t, link, filepath.Join(dir, "timezone"), time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	svc.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "timedatectl" {
			return executil.Result{Stdout: "Europe/London\n"}, true, nil
		}
		return executil.Result{}, false, nil
	})
	info := svc.timezone(context.Background())
	if info.Name != "Europe/London" {
		t.Fatalf("name = %q", info)
	}
}

func testTimezoneService(t *testing.T, localtime, tzFile string, now time.Time) *Service {
	t.Helper()
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(_ context.Context, name string, _ []string) (executil.Result, bool, error) {
		if name == "timedatectl" {
			return executil.Result{}, true, fmt.Errorf("timedatectl not found")
		}
		return executil.Result{}, false, nil
	})
	return &Service{
		runner:           runner,
		localtimePath:    localtime,
		timezoneFilePath: tzFile,
		now:              func() time.Time { return now },
	}
}
