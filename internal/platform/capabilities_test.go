package platform_test

import (
	"context"
	"strings"
	"testing"

	"abstrax/internal/platform"
)

func TestRHELDatabaseProvider(t *testing.T) {
	p := platform.RHELDefaults()
	if p.DatabaseEngine() != platform.DatabaseMariaDB {
		t.Fatalf("engine = %q", p.DatabaseEngine())
	}
	if p.DatabasePackage("") != "mariadb-server" {
		t.Fatalf("package = %q", p.DatabasePackage(""))
	}
	if p.DatabaseServiceName() != "mariadb" {
		t.Fatalf("service = %q", p.DatabaseServiceName())
	}
	if !strings.Contains(p.DatabaseDisplayName(), "MariaDB") {
		t.Fatalf("display = %q", p.DatabaseDisplayName())
	}
	if p.DatabaseAuthPlugin() != platform.DatabaseAuthNativePassword {
		t.Fatalf("auth plugin = %q", p.DatabaseAuthPlugin())
	}
}

func TestDebianDatabaseProvider(t *testing.T) {
	p := platform.DebianDefaults()
	if p.DatabaseEngine() != platform.DatabaseMySQL {
		t.Fatalf("engine = %q", p.DatabaseEngine())
	}
	if p.DatabasePackage("") != "mysql-server" {
		t.Fatalf("package = %q", p.DatabasePackage(""))
	}
	if p.DatabasePackage("8.0") != "mysql-server-8.0" {
		t.Fatalf("versioned package = %q", p.DatabasePackage("8.0"))
	}
	if p.DatabaseServiceName() != "mysql" {
		t.Fatalf("service = %q", p.DatabaseServiceName())
	}
	if p.DatabaseAuthPlugin() != platform.DatabaseAuthCachingSHA2 {
		t.Fatalf("auth plugin = %q", p.DatabaseAuthPlugin())
	}
}

func TestCertbotProvider(t *testing.T) {
	debian := platform.DebianDefaults()
	pkgs := debian.CertbotPackages()
	if len(pkgs) != 2 || pkgs[0] != "certbot" || pkgs[1] != "python3-certbot-nginx" {
		t.Fatalf("debian certbot packages = %#v", pkgs)
	}
	if debian.RequiresEPELForCertbot() {
		t.Fatal("debian should not require EPEL")
	}

	rhel := platform.RHELDefaults()
	pkgs = rhel.CertbotPackages()
	if len(pkgs) != 2 || pkgs[0] != "certbot" || pkgs[1] != "python3-certbot-nginx" {
		t.Fatalf("rhel certbot packages = %#v", pkgs)
	}
	if !rhel.RequiresEPELForCertbot() {
		t.Fatal("rhel should require EPEL for certbot")
	}
}

func TestSupervisorProvider(t *testing.T) {
	debian := platform.DebianDefaults()
	if debian.SupervisorPackage() != "supervisor" {
		t.Fatalf("debian package = %q", debian.SupervisorPackage())
	}
	if debian.SupervisorServiceName() != "supervisor" {
		t.Fatalf("debian service = %q", debian.SupervisorServiceName())
	}
	if debian.SupervisorConfigDir() != "/etc/supervisor/conf.d" {
		t.Fatalf("debian conf dir = %q", debian.SupervisorConfigDir())
	}
	if debian.SupervisorConfigExtension() != ".conf" {
		t.Fatalf("debian ext = %q", debian.SupervisorConfigExtension())
	}

	rhel := platform.RHELDefaults()
	if rhel.SupervisorPackage() != "supervisor" {
		t.Fatalf("rhel package = %q", rhel.SupervisorPackage())
	}
	if rhel.SupervisorServiceName() != "supervisord" {
		t.Fatalf("rhel service = %q", rhel.SupervisorServiceName())
	}
	if rhel.SupervisorConfigDir() != "/etc/supervisord.d" {
		t.Fatalf("rhel conf dir = %q", rhel.SupervisorConfigDir())
	}
	if rhel.SupervisorConfigExtension() != ".ini" {
		t.Fatalf("rhel ext = %q", rhel.SupervisorConfigExtension())
	}
}

func TestNodeSourceSetupURL(t *testing.T) {
	debian := platform.DebianDefaults()
	url, err := debian.NodeSourceSetupURL("20")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://deb.nodesource.com/setup_20.x" {
		t.Fatalf("debian url = %q", url)
	}

	rhel := platform.RHELDefaults()
	url, err = rhel.NodeSourceSetupURL("20.11.0")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://rpm.nodesource.com/setup_20.x" {
		t.Fatalf("rhel url = %q", url)
	}

	if _, err := platform.NodeSourceSetupURL("debian", "evil;rm -rf /"); err == nil {
		t.Fatal("expected invalid version error")
	}
	if _, err := platform.ValidateNodeMajor("99"); err == nil {
		t.Fatal("expected unsupported major error")
	}
}

func TestRubyPackages(t *testing.T) {
	debian := platform.DebianDefaults()
	pkgs, err := debian.RubyPackages("3.2")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0] != "ruby3.2" {
		t.Fatalf("debian ruby packages = %#v", pkgs)
	}
	if !debian.RubySupportsExactVersion() {
		t.Fatal("debian should support exact ruby versions")
	}

	rhel := platform.RHELDefaults()
	pkgs, err = rhel.RubyPackages("3.2")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 || pkgs[0] != "ruby" || pkgs[1] != "ruby-devel" {
		t.Fatalf("rhel ruby packages = %#v", pkgs)
	}
	if rhel.RubySupportsExactVersion() {
		t.Fatal("rhel should not claim exact ruby version support")
	}
}

func TestFirewallNumberedRulesCapability(t *testing.T) {
	if !platform.DebianDefaults().FirewallSupportsNumberedRules() {
		t.Fatal("debian/ufw should support numbered rules")
	}
	if platform.RHELDefaults().FirewallSupportsNumberedRules() {
		t.Fatal("rhel/firewalld should not claim UFW-style numbered rules (uses Abstrax list IDs)")
	}
}

func TestNginxPHPFastCGIInclude(t *testing.T) {
	debian := platform.DebianDefaults()
	if debian.NginxPHPFastCGIInclude() != "snippets/fastcgi-php.conf" {
		t.Fatalf("debian include = %q", debian.NginxPHPFastCGIInclude())
	}
	rhel := platform.RHELDefaults()
	if rhel.NginxPHPFastCGIInclude() != "" {
		t.Fatalf("rhel should not use Debian snippets; got %q", rhel.NginxPHPFastCGIInclude())
	}
}

func TestCacheProviderNames(t *testing.T) {
	debian := platform.DebianDefaults()
	if debian.RedisPackage() != "redis-server" || debian.RedisServiceName() != "redis-server" {
		t.Fatalf("debian redis = %s/%s", debian.RedisPackage(), debian.RedisServiceName())
	}
	if debian.RequiresExternalRepoForRedis() {
		t.Fatal("debian redis should not require Remi")
	}

	rocky9 := rhelProviderForVersion(t, "rocky", "Rocky Linux 9.5", "9.5", platform.SupportOfficial)
	if rocky9.RedisPackage() != "redis" || rocky9.RedisServiceName() != "redis" {
		t.Fatalf("rocky9 redis = %s/%s", rocky9.RedisPackage(), rocky9.RedisServiceName())
	}
	if rocky9.RequiresExternalRepoForRedis() {
		t.Fatal("rocky 9 redis should use AppStream without Remi")
	}

	rocky10 := rhelProviderForVersion(t, "rocky", "Rocky Linux 10.1", "10.1", platform.SupportOfficial)
	if !rocky10.RequiresExternalRepoForRedis() {
		t.Fatal("rocky 10 redis should require Remi (AppStream ships Valkey)")
	}
	if rocky10.RedisModuleStream() != "redis:remi-7.2" {
		t.Fatalf("module stream = %q", rocky10.RedisModuleStream())
	}
}

func TestEnsureRedisRepositoryRocky10(t *testing.T) {
	provider := rhelProviderForVersion(t, "rocky", "Rocky Linux 10.1", "10.1", platform.SupportOfficial)
	var installed []string
	var ran []string
	err := platform.EnsureRedisRepository(context.Background(), provider, platform.RepoOptions{EnableRequiredRepos: true}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			installed = append(installed, name)
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			ran = append(ran, name+" "+strings.Join(args, " "))
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	foundRemi := false
	for _, name := range installed {
		if strings.Contains(name, "remi-release-10.rpm") {
			foundRemi = true
		}
	}
	if !foundRemi {
		t.Fatalf("expected remi-release-10; installed=%#v", installed)
	}
	foundModule := false
	for _, cmd := range ran {
		if strings.Contains(cmd, "module enable redis:remi-7.2") {
			foundModule = true
		}
	}
	if !foundModule {
		t.Fatalf("expected module enable; ran=%#v", ran)
	}
}

func TestEnsureRedisRepositoryRocky9Noop(t *testing.T) {
	provider := rhelProviderForVersion(t, "rocky", "Rocky Linux 9.5", "9.5", platform.SupportOfficial)
	err := platform.EnsureRedisRepository(context.Background(), provider, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			t.Fatalf("unexpected install %s", name)
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			t.Fatalf("unexpected run %s", name)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnsureEPELUnsupportedDistro(t *testing.T) {
	info := &platform.Info{
		Family: "rhel",
		Profile: platform.Profile{
			Family:       "rhel",
			DistroID:     "rhel",
			DistroName:   "Red Hat Enterprise Linux 9.5",
			SupportLevel: platform.SupportCompatible,
			WebUser:      "nginx",
			WebGroup:     "nginx",
		},
	}
	provider, err := platform.For(info)
	if err != nil {
		t.Fatal(err)
	}
	err = platform.EnsureEPEL(context.Background(), provider,
		func(ctx context.Context, name string) error { return nil },
		func(ctx context.Context, name string, args ...string) error { return nil },
	)
	if err == nil {
		t.Fatal("expected EPEL manual instruction error for RHEL")
	}
	if !strings.Contains(err.Error(), "EPEL") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "enable-required-repos") && !strings.Contains(err.Error(), "repo enable") {
		t.Fatalf("error should mention how to enable EPEL: %v", err)
	}
}

func TestEnsureEPELRocky(t *testing.T) {
	info := &platform.Info{
		Family: "rhel",
		Profile: platform.Profile{
			Family:       "rhel",
			DistroID:     "rocky",
			DistroName:   "Rocky Linux 9.5",
			SupportLevel: platform.SupportOfficial,
			WebUser:      "nginx",
			WebGroup:     "nginx",
		},
	}
	provider, err := platform.For(info)
	if err != nil {
		t.Fatal(err)
	}
	var installed []string
	err = platform.EnsureEPEL(context.Background(), provider,
		func(ctx context.Context, name string) error {
			installed = append(installed, name)
			return nil
		},
		func(ctx context.Context, name string, args ...string) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0] != "epel-release" {
		t.Fatalf("installed = %#v", installed)
	}
}
