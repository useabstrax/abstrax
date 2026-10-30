package user

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLockStatePath  = "/var/lib/abstrax/user-locks.json"
	accountLockExpireDays = "1"
)

type lockRecord struct {
	PasswordOnly bool   `json:"password_only,omitempty"`
	Expiry       string `json:"expiry,omitempty"`
}

type lockFile struct {
	Users map[string]lockRecord `json:"users"`
}

func (s *Service) lockPath() string {
	if s.lockStatePath != "" {
		return s.lockStatePath
	}
	return defaultLockStatePath
}

func (s *Service) shadowExpire(ctx context.Context, username string) (string, error) {
	res, err := s.runner.RunSilent(ctx, "getent", "shadow", username)
	if err != nil {
		return "", nil
	}
	return shadowExpireField(res.Stdout), nil
}

func shadowExpireField(line string) string {
	parts := strings.Split(strings.TrimSpace(line), ":")
	if len(parts) < 8 {
		return ""
	}
	return parts[7]
}

func daysToDate(days string) (string, error) {
	n, err := strconv.Atoi(days)
	if err != nil {
		return "", err
	}
	return time.Unix(0, 0).UTC().AddDate(0, 0, n).Format("2006-01-02"), nil
}

func accountExpired(expireField string, now time.Time) bool {
	if expireField == "" {
		return false
	}
	n, err := strconv.Atoi(expireField)
	if err != nil {
		return false
	}
	expiry := time.Unix(0, 0).UTC().AddDate(0, 0, n)
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return !expiry.After(today)
}

func (s *Service) applyAccountState(ctx context.Context, info *UserInfo) {
	if res, err := s.runner.RunSilent(ctx, "passwd", "-S", info.Username); err == nil {
		parts := strings.Fields(res.Stdout)
		if len(parts) >= 2 && parts[1] == "L" {
			info.Locked = true
		}
	}
	if res, err := s.runner.RunSilent(ctx, "getent", "shadow", info.Username); err == nil {
		info.Expired = accountExpired(shadowExpireField(res.Stdout), time.Now())
	}
	info.LoginBlocked = info.Locked && info.Expired
}

func (s *Service) loadLocks() (lockFile, error) {
	data, err := os.ReadFile(s.lockPath())
	if os.IsNotExist(err) {
		return lockFile{Users: map[string]lockRecord{}}, nil
	}
	if err != nil {
		return lockFile{}, err
	}
	var file lockFile
	if err := json.Unmarshal(data, &file); err != nil {
		return lockFile{}, fmt.Errorf("reading %s: %w", s.lockPath(), err)
	}
	if file.Users == nil {
		file.Users = map[string]lockRecord{}
	}
	return file, nil
}

func (s *Service) writeLocks(file lockFile) error {
	if file.Users == nil {
		file.Users = map[string]lockRecord{}
	}
	path := s.lockPath()
	if len(file.Users) == 0 {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func (s *Service) savedLock(username string) (lockRecord, bool, error) {
	file, err := s.loadLocks()
	if err != nil {
		return lockRecord{}, false, err
	}
	record, ok := file.Users[username]
	return record, ok, nil
}

func (s *Service) saveLock(username string, record lockRecord) error {
	file, err := s.loadLocks()
	if err != nil {
		return err
	}
	file.Users[username] = record
	return s.writeLocks(file)
}

func (s *Service) deleteLock(username string) error {
	file, err := s.loadLocks()
	if err != nil {
		return err
	}
	delete(file.Users, username)
	return s.writeLocks(file)
}
