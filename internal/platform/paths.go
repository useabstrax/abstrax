package platform

// Paths holds filesystem paths used by Abstrax on a given platform family.
type Paths struct {
	SudoGroup                  string
	CronDir                    string
	SSHConfigDir               string
	AbstraxSSHConfig           string
	SupervisorConfDir          string
	NginxSitesAvailable        string
	NginxSitesEnabled          string
	NginxConfPath              string
	NginxSitesEnabledInclude   string
	NginxConfigDir             string
	AbstraxStateDir            string
	AbstraxConfigDir           string
	AbstraxConfig              string
	AbstraxProjectsDir         string
	AbstraxProjectsDirLegacy   string
	MySQLConfig                string
	MySQLConfigLegacy          string
	AbstraxLogDir              string
	AbstraxPluginsDir          string
	AbstraxPluginsDirAlt       string
	AbstraxPluginStateDir      string
	AbstraxPluginCacheDir      string
	AbstraxPluginRegistryCache string
	PHPSocketDir               string
	PHPConfigRoot              string
	PHPFPMPoolDir              string
	PHPFPMDefaultPoolConfig    string
}
