package serverinfo

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

func (s *Service) timezone(ctx context.Context) TimezoneInfo {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	return timezoneInfoAt(s.timezoneName(ctx), now())
}

func (s *Service) timezoneName(ctx context.Context) string {
	if res, err := s.runner.RunSilent(ctx, "timedatectl", "show", "-p", "Timezone", "--value"); err == nil {
		if name := strings.TrimSpace(res.Stdout); validZoneName(name) {
			return name
		}
	}

	localtime := s.localtimePath
	if localtime == "" {
		localtime = "/etc/localtime"
	}
	if name := zoneFromLocaltime(localtime); name != "" {
		return name
	}

	tzFile := s.timezoneFilePath
	if tzFile == "" {
		tzFile = "/etc/timezone"
	}
	if data, err := os.ReadFile(tzFile); err == nil {
		if name := strings.TrimSpace(string(data)); validZoneName(name) {
			return name
		}
	}

	if _, err := os.Lstat(localtime); err != nil {
		return "UTC"
	}
	return ""
}

func timezoneInfoAt(name string, now time.Time) TimezoneInfo {
	loc, err := time.LoadLocation(name)
	if err != nil || name == "" {
		abbr, offset := now.Zone()
		zoneName := now.Location().String()
		if zoneName == "" || zoneName == "Local" {
			zoneName = "UTC"
			abbr, offset = now.UTC().Zone()
		}
		return TimezoneInfo{
			Name:             zoneName,
			Abbreviation:     abbr,
			UTCOffset:        formatUTCOffset(offset),
			UTCOffsetSeconds: offset,
		}
	}
	abbr, offset := now.In(loc).Zone()
	return TimezoneInfo{
		Name:             name,
		Abbreviation:     abbr,
		UTCOffset:        formatUTCOffset(offset),
		UTCOffsetSeconds: offset,
	}
}

func zoneFromLocaltime(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	const marker = "zoneinfo/"
	i := strings.LastIndex(target, marker)
	if i < 0 {
		return ""
	}
	name := strings.TrimSpace(target[i+len(marker):])
	if !validZoneName(name) {
		return ""
	}
	return name
}

func validZoneName(name string) bool {
	if name == "" || name == "n/a" {
		return false
	}
	if strings.ContainsAny(name, "\n\r\x00") || strings.Contains(name, "..") {
		return false
	}
	return true
}

func formatUTCOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, mins)
}
