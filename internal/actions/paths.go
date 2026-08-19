package actions

// CommandPath returns the Cobra command path for a builtin action.
func CommandPath(action string) ([]string, bool) {
	path, ok := commandPaths[action]
	if !ok {
		return nil, false
	}
	out := make([]string, len(path))
	copy(out, path)
	return out, true
}

// All returns every builtin action constant. Tests use this to ensure the
// command-path map stays complete.
func All() []string {
	out := make([]string, 0, len(commandPaths))
	for action := range commandPaths {
		out = append(out, action)
	}
	return out
}

// commandPaths maps stable action names to Cobra command paths.
var commandPaths = map[string][]string{
	UserAdd:          {"user", "add"},
	UserRemove:       {"user", "remove"},
	UserGrantSudo:    {"user", "grant-sudo"},
	UserRevokeSudo:   {"user", "revoke-sudo"},
	UserSetGroups:    {"user", "set-groups"},
	UserAddGroups:    {"user", "add-groups"},
	UserRemoveGroups: {"user", "remove-groups"},
	UserSetShell:     {"user", "set-shell"},
	UserLock:         {"user", "lock"},
	UserUnlock:       {"user", "unlock"},
	UserInfo:         {"user", "info"},
	UserList:         {"user", "list"},

	SSHKeyAdd:    {"ssh-key", "add"},
	SSHKeyRemove: {"ssh-key", "remove"},
	SSHKeyList:   {"ssh-key", "list"},
	SSHKeyInfo:   {"ssh-key", "info"},

	SSHConfigShow:              {"ssh", "config", "show"},
	SSHConfigSetPort:           {"ssh", "config", "set-port"},
	SSHConfigSetTimeout:        {"ssh", "config", "set-timeout"},
	SSHConfigDisableRootLogin:  {"ssh", "config", "disable-root-login"},
	SSHConfigEnableRootLogin:   {"ssh", "config", "enable-root-login"},
	SSHConfigDisablePasswdAuth: {"ssh", "config", "disable-password-auth"},
	SSHConfigEnablePasswdAuth:  {"ssh", "config", "enable-password-auth"},
	SSHReload:                  {"ssh", "reload"},
	SSHRestart:                 {"ssh", "restart"},

	PackageInstall: {"package", "install"},
	PackageRemove:  {"package", "remove"},
	PackageUpdate:  {"package", "update"},
	PackageUpgrade: {"package", "upgrade"},
	PackageSearch:  {"package", "search"},
	PackageInfo:    {"package", "info"},
	PackageList:    {"package", "list"},

	ServiceStart:   {"service", "start"},
	ServiceStop:    {"service", "stop"},
	ServiceRestart: {"service", "restart"},
	ServiceReload:  {"service", "reload"},
	ServiceEnable:  {"service", "enable"},
	ServiceDisable: {"service", "disable"},
	ServiceStatus:  {"service", "status"},

	CronAdd:     {"cron", "add"},
	CronRemove:  {"cron", "remove"},
	CronModify:  {"cron", "modify"},
	CronList:    {"cron", "list"},
	CronInfo:    {"cron", "info"},
	CronEnable:  {"cron", "enable"},
	CronDisable: {"cron", "disable"},

	DaemonAdd:     {"daemon", "add"},
	DaemonRemove:  {"daemon", "remove"},
	DaemonModify:  {"daemon", "modify"},
	DaemonStart:   {"daemon", "start"},
	DaemonStop:    {"daemon", "stop"},
	DaemonRestart: {"daemon", "restart"},
	DaemonStatus:  {"daemon", "status"},
	DaemonList:    {"daemon", "list"},
	DaemonLogs:    {"daemon", "logs"},
	DaemonInstall: {"daemon", "install"},

	ProjectAdd:     {"project", "add"},
	ProjectRemove:  {"project", "remove"},
	ProjectModify:  {"project", "modify"},
	ProjectList:    {"project", "list"},
	ProjectInfo:    {"project", "info"},
	ProjectEnable:  {"project", "enable"},
	ProjectDisable: {"project", "disable"},
	ProjectReload:  {"project", "reload"},

	WebInstall: {"web", "install"},
	WebTest:    {"web", "test"},
	WebReload:  {"web", "reload"},
	WebRestart: {"web", "restart"},

	SSLInstall: {"ssl", "install"},
	SSLAdd:     {"ssl", "add"},
	SSLRemove:  {"ssl", "remove"},
	SSLRenew:   {"ssl", "renew"},
	SSLStatus:  {"ssl", "status"},

	MySQLConfigSet:         {"mysql", "config", "set"},
	MySQLConfigShow:        {"mysql", "config", "show"},
	MySQLTest:              {"mysql", "test"},
	MySQLInstall:           {"mysql", "install"},
	MySQLResetRootPassword: {"mysql", "reset-root-password"},
	MySQLDBAdd:             {"mysql", "database", "add"},
	MySQLDBRemove:          {"mysql", "database", "remove"},
	MySQLDBList:            {"mysql", "database", "list"},
	MySQLUserAdd:           {"mysql", "user", "add"},
	MySQLUserRemove:        {"mysql", "user", "remove"},
	MySQLUserList:          {"mysql", "user", "list"},
	MySQLUserInfo:          {"mysql", "user", "info"},
	MySQLGrant:             {"mysql", "grant"},
	MySQLRevoke:            {"mysql", "revoke"},

	CacheInstall: {"cache", "install"},
	CacheRemove:  {"cache", "remove"},
	CacheStart:   {"cache", "start"},
	CacheStop:    {"cache", "stop"},
	CacheRestart: {"cache", "restart"},
	CacheStatus:  {"cache", "status"},
	CacheConfig:  {"cache", "config"},

	FirewallStatus:        {"firewall", "status"},
	FirewallInstall:       {"firewall", "install"},
	FirewallEnable:        {"firewall", "enable"},
	FirewallDisable:       {"firewall", "disable"},
	FirewallAllow:         {"firewall", "allow"},
	FirewallDeny:          {"firewall", "deny"},
	FirewallAllowIP:       {"firewall", "allow-ip"},
	FirewallDenyIP:        {"firewall", "deny-ip"},
	FirewallRuleList:      {"firewall", "rule", "list"},
	FirewallRuleRm:        {"firewall", "rule", "remove"},
	FirewallRemoveService: {"firewall", "remove", "service"},
	FirewallRemovePort:    {"firewall", "remove", "port"},

	RepoEnable: {"repo", "enable"},

	ServerStatus:   {"server", "status"},
	ServerCPU:      {"server", "cpu"},
	ServerMemory:   {"server", "memory"},
	ServerDisk:     {"server", "disk"},
	ServerLoad:     {"server", "load"},
	ServerServices: {"server", "services"},

	DoctorCheck: {"doctor"},
	VersionShow: {"version"},
	SelfUpdate:  {"self", "update"},

	ConfigShow:   {"config", "show"},
	ConfigGet:    {"config", "get"},
	ConfigSet:    {"config", "set"},
	ConfigAdd:    {"config", "add"},
	ConfigRemove: {"config", "remove"},
	ConfigReset:  {"config", "reset"},

	PluginList:    {"plugin", "list"},
	PluginInfo:    {"plugin", "info"},
	PluginSearch:  {"plugin", "search"},
	PluginInstall: {"plugin", "install"},
	PluginUpdate:  {"plugin", "update"},
	PluginRemove:  {"plugin", "remove"},

	ProjectInspect:        {"project", "inspect"},
	ProjectServiceRestart: {"project", "service", "restart"},
	ProjectServiceReload:  {"project", "service", "reload"},
}
