// Package actions defines stable action name constants used across the CLI and,
// in the future, the hosted agent. Action names must be stable identifiers so
// the hosted platform can create structured jobs that reference them.
package actions

const (
	// User actions.
	UserAdd          = "user.add"
	UserRemove       = "user.remove"
	UserGrantSudo    = "user.grant_sudo"
	UserRevokeSudo   = "user.revoke_sudo"
	UserSetGroups    = "user.set_groups"
	UserAddGroups    = "user.add_groups"
	UserRemoveGroups = "user.remove_groups"
	UserSetShell     = "user.set_shell"
	UserLock         = "user.lock"
	UserUnlock       = "user.unlock"
	UserInfo         = "user.info"
	UserList         = "user.list"

	// SSH key actions.
	SSHKeyAdd    = "ssh_key.add"
	SSHKeyRemove = "ssh_key.remove"
	SSHKeyList   = "ssh_key.list"
	SSHKeyInfo   = "ssh_key.info"

	// SSH config actions.
	SSHConfigShow              = "ssh.config.show"
	SSHConfigSetPort           = "ssh.config.set_port"
	SSHConfigSetTimeout        = "ssh.config.set_timeout"
	SSHConfigDisableRootLogin  = "ssh.config.disable_root_login"
	SSHConfigEnableRootLogin   = "ssh.config.enable_root_login"
	SSHConfigDisablePasswdAuth = "ssh.config.disable_password_auth"
	SSHConfigEnablePasswdAuth  = "ssh.config.enable_password_auth"
	SSHReload                  = "ssh.reload"
	SSHRestart                 = "ssh.restart"

	// Package actions.
	PackageInstall = "package.install"
	PackageRemove  = "package.remove"
	PackageUpdate  = "package.update"
	PackageUpgrade = "package.upgrade"
	PackageSearch  = "package.search"
	PackageInfo    = "package.info"
	PackageList    = "package.list"

	// Service (systemd) actions.
	ServiceStart   = "service.start"
	ServiceStop    = "service.stop"
	ServiceRestart = "service.restart"
	ServiceReload  = "service.reload"
	ServiceEnable  = "service.enable"
	ServiceDisable = "service.disable"
	ServiceStatus  = "service.status"

	// Cron actions.
	CronAdd     = "cron.add"
	CronRemove  = "cron.remove"
	CronModify  = "cron.modify"
	CronList    = "cron.list"
	CronInfo    = "cron.info"
	CronEnable  = "cron.enable"
	CronDisable = "cron.disable"

	// Daemon actions.
	DaemonAdd     = "daemon.add"
	DaemonRemove  = "daemon.remove"
	DaemonModify  = "daemon.modify"
	DaemonStart   = "daemon.start"
	DaemonStop    = "daemon.stop"
	DaemonRestart = "daemon.restart"
	DaemonStatus  = "daemon.status"
	DaemonList    = "daemon.list"
	DaemonLogs    = "daemon.logs"

	// Project actions.
	ProjectAdd     = "project.add"
	ProjectRemove  = "project.remove"
	ProjectModify  = "project.modify"
	ProjectList    = "project.list"
	ProjectInfo    = "project.info"
	ProjectEnable  = "project.enable"
	ProjectDisable = "project.disable"
	ProjectReload  = "project.reload"

	// Web actions.
	WebInstall = "web.install"
	WebTest    = "web.test"
	WebReload  = "web.reload"
	WebRestart = "web.restart"

	// SSL actions.
	SSLAdd    = "ssl.add"
	SSLRemove = "ssl.remove"
	SSLRenew  = "ssl.renew"
	SSLStatus = "ssl.status"

	// MySQL actions.
	MySQLConfigSet  = "mysql.config_set"
	MySQLConfigShow = "mysql.config_show"
	MySQLTest       = "mysql.test"
	MySQLInstall    = "mysql.install"
	MySQLDBAdd      = "mysql.database.add"
	MySQLDBRemove   = "mysql.database.remove"
	MySQLDBList     = "mysql.database.list"
	MySQLUserAdd    = "mysql.user.add"
	MySQLUserRemove = "mysql.user.remove"
	MySQLUserList   = "mysql.user.list"
	MySQLUserInfo   = "mysql.user.info"
	MySQLGrant      = "mysql.grant"
	MySQLRevoke     = "mysql.revoke"

	// Cache actions.
	CacheInstall = "cache.install"
	CacheRemove  = "cache.remove"
	CacheStart   = "cache.start"
	CacheStop    = "cache.stop"
	CacheRestart = "cache.restart"
	CacheStatus  = "cache.status"
	CacheConfig  = "cache.config"

	// Firewall actions.
	FirewallStatus   = "firewall.status"
	FirewallEnable   = "firewall.enable"
	FirewallDisable  = "firewall.disable"
	FirewallAllow    = "firewall.allow"
	FirewallDeny     = "firewall.deny"
	FirewallAllowIP  = "firewall.allow_ip"
	FirewallDenyIP   = "firewall.deny_ip"
	FirewallRuleList = "firewall.rule.list"
	FirewallRuleRm   = "firewall.rule.remove"

	// Server info actions.
	ServerStatus   = "server.status"
	ServerCPU      = "server.cpu"
	ServerMemory   = "server.memory"
	ServerDisk     = "server.disk"
	ServerLoad     = "server.load"
	ServerServices = "server.services"

	// Agent actions (placeholder - not yet implemented).
	AgentConnect = "agent.connect"
	AgentStatus  = "agent.status"
	AgentRun     = "agent.run"
	AgentUpdate  = "agent.update"

	// Doctor / version.
	DoctorCheck = "doctor.check"
	VersionShow = "version.show"

	// Self-update.
	SelfUpdate = "self.update"

	// Config actions.
	ConfigShow   = "config.show"
	ConfigGet    = "config.get"
	ConfigSet    = "config.set"
	ConfigAdd    = "config.add"
	ConfigRemove = "config.remove"
	ConfigReset  = "config.reset"

	// Plugin actions.
	PluginList    = "plugin.list"
	PluginInfo    = "plugin.info"
	PluginSearch  = "plugin.search"
	PluginInstall = "plugin.install"
	PluginUpdate  = "plugin.update"
	PluginRemove  = "plugin.remove"

	// Project inspect and service actions.
	ProjectInspect        = "project.inspect"
	ProjectServiceRestart = "project.service.restart"
	ProjectServiceReload  = "project.service.reload"
)
