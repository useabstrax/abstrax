// Package debian provides Debian/Ubuntu specific helpers and constants.
package debian

const (
	// SudoGroup is the group that grants sudo access on Debian/Ubuntu.
	SudoGroup = "sudo"

	// CronDir is the directory for managed cron files.
	CronDir = "/etc/cron.d"

	// SSHConfigDir is the sshd_config include directory.
	SSHConfigDir = "/etc/ssh/sshd_config.d"

	// AbstraxSSHConfig is the managed sshd include file.
	AbstraxSSHConfig = "/etc/ssh/sshd_config.d/99-abstrax.conf"

	// SupervisorConfDir is the Supervisor conf.d directory.
	SupervisorConfDir = "/etc/supervisor/conf.d"

	// NginxSitesAvailable is nginx's sites-available dir.
	NginxSitesAvailable = "/etc/nginx/sites-available"

	// NginxSitesEnabled is nginx's sites-enabled dir.
	NginxSitesEnabled = "/etc/nginx/sites-enabled"

	// AbstraxStateDir is where Abstrax stores project state.
	AbstraxStateDir = "/var/lib/abstrax"

	// AbstraxProjectsDir is where project state JSON files live.
	AbstraxProjectsDir = "/var/lib/abstrax/projects"

	// AbstraxConfigDir is the main config directory.
	AbstraxConfigDir = "/etc/abstrax"

	// MySQLConfig stores Abstrax MySQL connection config.
	MySQLConfig = "/etc/abstrax/mysql.toml"

	// AbstraxLogDir is the log directory.
	AbstraxLogDir = "/var/log/abstrax"
)
