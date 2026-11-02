package mysql

import (
	"context"
	"fmt"
	"strings"

	"abstrax/internal/validate"
)

const redactedSQLPassword = "[redacted]"

// UserPassword sets a new password on each matching account.
// Grants and the auth plugin are left unchanged. MySQL and MariaDB both use
// ALTER USER ... IDENTIFIED BY, which keeps the account's current plugin.
func (s *Service) UserPassword(ctx context.Context, opts UserPasswordOptions) (*UserPasswordResult, error) {
	hosts, err := s.resolveAccountHosts(ctx, opts.Name, opts.Host)
	if err != nil {
		return nil, err
	}

	password, generated, err := resolveOrGeneratePassword(opts.Password)
	if err != nil {
		return nil, err
	}

	for _, h := range hosts {
		account, err := sqlAccount(opts.Name, h)
		if err != nil {
			return nil, err
		}
		sql := fmt.Sprintf("ALTER USER %s IDENTIFIED BY '%s'", account, escapeSQLString(password))
		if err := s.execIdentifiedBy(ctx, sql, password, s.dryRun || opts.DryRun); err != nil {
			return nil, err
		}
	}

	result := &UserPasswordResult{
		Name:      opts.Name,
		Password:  password,
		Generated: generated,
	}
	if len(hosts) == 1 {
		result.Host = hosts[0]
	} else {
		result.Hosts = hosts
	}
	return result, nil
}

// resolveAccountHosts returns the accounts a password change or drop should touch.
// An empty host means every host the user exists on. localhost means the local
// app pair (localhost and 127.0.0.1). Any other host is that account only.
// If the user does not exist on any matching host, no statement is run.
func (s *Service) resolveAccountHosts(ctx context.Context, name, host string) ([]string, error) {
	if host != "" {
		if err := validate.MySQLHost(host); err != nil {
			return nil, err
		}
	}

	existing, err := s.lookupUserHosts(ctx, name, s.queryRead)
	if err != nil {
		return nil, err
	}
	if host == "" {
		return existing, nil
	}

	want := []string{host}
	if usesLocalAppHosts(host) {
		want = localAppHosts()
	}

	have := make(map[string]bool, len(existing))
	for _, h := range existing {
		have[h] = true
	}
	var matched []string
	for _, h := range want {
		if have[h] {
			matched = append(matched, h)
		}
	}
	if len(matched) == 0 {
		if usesLocalAppHosts(host) {
			return nil, fmt.Errorf("mysql user %q has no localhost or 127.0.0.1 account", name)
		}
		return nil, fmt.Errorf("mysql user %q@%q not found", name, host)
	}
	return matched, nil
}

func redactSQLPassword(sql, password string) string {
	needle := "IDENTIFIED BY '" + escapeSQLString(password) + "'"
	return strings.ReplaceAll(sql, needle, "IDENTIFIED BY '"+redactedSQLPassword+"'")
}

// execIdentifiedBy runs ALTER USER. Dry-run and verbose output replace the
// password; the password is never written to the command log.
func (s *Service) execIdentifiedBy(ctx context.Context, sql, password string, dryRun bool) error {
	statement := sql
	if dryRun {
		statement = redactSQLPassword(sql, password)
	}
	if s.captureSQL {
		s.sqlLog = append(s.sqlLog, statement)
		return nil
	}
	if dryRun {
		if s.runner != nil && s.dryRun {
			return s.exec(ctx, statement)
		}
		fmt.Printf("[dry-run] would run: mysql -e %s\n", statement)
		return nil
	}
	if s.verbose {
		fmt.Printf("[exec] mysql -e %s\n", redactSQLPassword(sql, password))
	}
	return s.runMySQL(ctx, s.connectionConfig(), sql, true)
}
