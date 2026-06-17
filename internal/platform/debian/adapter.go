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

	// AbstraxStateDir is where Abstrax stores runtime state (plugins, caches).
	AbstraxStateDir = "/var/lib/abstrax"

	// AbstraxConfigDir is the main config directory.
	AbstraxConfigDir = "/etc/abstrax"

	// AbstraxConfig stores general Abstrax settings.
	AbstraxConfig = "/etc/abstrax/config.json"

	// AbstraxProjectsDir is where project state JSON files live.
	AbstraxProjectsDir = "/etc/abstrax/projects"

	// AbstraxProjectsDirLegacy is the pre-consolidation project state directory.
	AbstraxProjectsDirLegacy = "/var/lib/abstrax/projects"

	// MySQLConfig stores Abstrax MySQL connection config.
	MySQLConfig = "/etc/abstrax/mysql.json"

	// MySQLConfigLegacy is the pre-consolidation MySQL config file.
	MySQLConfigLegacy = "/etc/abstrax/mysql.toml"

	// AbstraxLogDir is the log directory.
	AbstraxLogDir = "/var/log/abstrax"

	// AbstraxPluginsDir is the preferred system plugin installation directory.
	AbstraxPluginsDir = "/usr/local/lib/abstrax/plugins"

	// AbstraxPluginsDirAlt is the secondary system plugin search directory.
	AbstraxPluginsDirAlt = "/usr/lib/abstrax/plugins"

	// AbstraxPluginStateDir stores plugin installation records and caches.
	AbstraxPluginStateDir = "/var/lib/abstrax/plugins"

	// AbstraxPluginCacheDir stores plugin metadata and registry caches.
	AbstraxPluginCacheDir = "/var/lib/abstrax/plugins/cache"

	// AbstraxPluginRegistryCacheDir stores cached registry HTTP responses.
	AbstraxPluginRegistryCacheDir = "/var/lib/abstrax/plugins/cache/registry"
)
