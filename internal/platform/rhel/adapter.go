// Package rhel provides RHEL-compatible specific helpers and constants.
package rhel

const (
	// SudoGroup is the group that grants sudo access on RHEL-family systems.
	SudoGroup = "wheel"

	// CronDir is the directory for managed cron files.
	CronDir = "/etc/cron.d"

	// SSHConfigDir is the sshd_config include directory.
	SSHConfigDir = "/etc/ssh/sshd_config.d"

	// AbstraxSSHConfig is the managed sshd include file.
	AbstraxSSHConfig = "/etc/ssh/sshd_config.d/99-abstrax.conf"

	// SupervisorConfDir is the Supervisor conf.d directory.
	SupervisorConfDir = "/etc/supervisord.d"

	// NginxConfigDir is nginx's conf.d directory on RHEL-family systems.
	NginxConfigDir = "/etc/nginx/conf.d"

	// NginxConfPath is the main nginx configuration file.
	NginxConfPath = "/etc/nginx/nginx.conf"

	// NginxConfDInclude is the include directive for conf.d site configs.
	NginxConfDInclude = "include /etc/nginx/conf.d/*.conf;"

	// PHPSocketDir is where PHP-FPM Unix sockets are created on RHEL-family systems.
	PHPSocketDir = "/run/php-fpm"

	// PHPConfigRoot is unused for versioned layouts on stock RHEL PHP packages.
	PHPConfigRoot = "/etc"

	// PHPFPMPoolDir is the default PHP-FPM pool configuration directory.
	PHPFPMPoolDir = "/etc/php-fpm.d"

	// PHPFPMDefaultPoolConfig is the default www pool configuration file.
	PHPFPMDefaultPoolConfig = "/etc/php-fpm.d/www.conf"

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
