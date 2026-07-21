# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2026-07-21

### Added

- **Platform profiles and detection** — Reads `/etc/os-release` and derives distro family, package and service managers, nginx layout, web user, project root, PHP-FPM strategy, firewall strategy, and support level (`official`, `compatible`, or `unsupported`).
- **Debian-family provider** — Centralises apt/systemd conventions: nginx `sites-available`/`sites-enabled`, `www-data`, `/var/www`, versioned PHP-FPM, and UFW.
- **RHEL-family provider** — Rocky Linux 9+ and AlmaLinux 9+ (official); experimental support for RHEL 9+, CentOS Stream 9+, and Oracle Linux 9+. Uses `dnf`, nginx `conf.d`, `nginx` web user, firewalld, and SELinux reporting.
- **DNF package backend** — Package commands use `dnf` on RHEL-family hosts and `apt` on Debian-family hosts.
- **firewalld backend** — Firewall on RHEL-family systems via `firewall-cmd` (permanent rules + reload); UFW behaviour preserved on Debian-family systems.
- **Firewalld rule removal** — `firewall rule list` assigns Abstrax IDs to services and ports; `firewall rule remove <id>` removes the matching entry. Also `firewall remove service` and `firewall remove port`.
- **Remi multi-version PHP** — RHEL-family PHP installs use Remi SCL packages (`php83-php-fpm`, paths under `/opt/remi` and `/etc/opt/remi`) with the same version-oriented project commands as Debian.
- **Repository helpers** — `abstrax repo enable <epel|crb|remi>` and global `--enable-required-repos` for explicit third-party repository consent (required for Remi; required for EPEL on RHEL/Oracle).
- **RHEL runtime and service installs** — `mysql install` uses `mariadb-server`; `ssl install` uses dnf and EPEL (automatic on Rocky/Alma/CentOS Stream; explicit on RHEL/Oracle); daemon auto-install uses `supervisord` and `/etc/supervisord.d`; project runtime install uses NodeSource RPM scripts and stock `ruby`/`ruby-devel` (exact Ruby version pinning on RHEL is a deliberate limitation).
- **SELinux warnings** — Enforcing mode is detected and surfaced in `doctor` and project/web flows; Abstrax never disables SELinux automatically.
- **`firewall install`** — Installs the platform firewall package (`ufw` on Debian-family, `firewalld` on RHEL-family) without enabling it.

### Changed

- **`abstrax doctor`** — Reports distro profile, nginx config directory, web user/group, SELinux status, firewalld presence, and support level alongside existing tool and manager detection.
- **Supported operating systems** — Fully supported distros are explicitly defined (Ubuntu 20.04+, Debian 11+, Linux Mint, Pop!_OS, Raspbian / Raspberry Pi OS; Rocky 9+, AlmaLinux 9+). Other Debian/Ubuntu derivatives are `compatible`; non-supported families are `unsupported`.
- **Mutating commands** — Commands that change system state verify platform support before running. Unsupported distributions receive a clear error without attempting destructive changes.
- **Nginx site enable/disable** — Provider-aware: Debian continues to symlink `sites-available` → `sites-enabled`; RHEL writes `/etc/nginx/conf.d/{site}.conf` and disables by renaming to `.disabled`.
- **Documentation** — Supported platforms documentation describes the functional parity model, Remi PHP, repository consent, firewalld rule removal, and remaining deliberate limitations.

### Fixed

- **Remi / CRB on EL10** — Remi release RPM URL and RHEL CodeReady Builder repo names now follow the host Enterprise Linux major version instead of hardcoding EL9.
- **PHP nginx virtual hosts on RHEL** — No longer include Debian-only `snippets/fastcgi-php.conf`; equivalent fastcgi directives are inlined on RHEL-family nginx.
- **Redis / Memcached on RHEL** — Uses provider-aware package, service, and config paths. Rocky/Alma 10+ enables the Remi Redis module stream (AppStream ships Valkey instead of Redis).
- **Firewall on RHEL** — `firewall enable` installs and starts `firewalld` before applying SSH protection rules; missing backends point users at `firewall install`.

## [1.1.1] - 2026-06-24

### Fixed

- **PHP nginx virtual hosts** — PHP location blocks now set `SCRIPT_FILENAME` and `DOCUMENT_ROOT` using `$realpath_root`, so OPcache picks up deployed code changes without reloading PHP-FPM after each release.

## [1.1.0] - 2026-06-23

### Removed

- **Automatic file backups** — Abstrax no longer creates timestamped `.abstrax-bak.<timestamp>` copies alongside managed files before overwriting them. This affected cron jobs in `/etc/cron.d`, SSH `authorized_keys`, the managed SSH include file, Supervisor configs, nginx virtual hosts, PHP-FPM pools, and `nginx.conf` patches. Backup files left in `/etc/cron.d` could be picked up by cron and appear as phantom jobs in `cron list`; a proper backup and restore design will replace this behaviour in a future release.

## [1.0.0] - 2026-06-23

First stable release of the Abstrax CLI — a single Go binary for managing common Linux server tasks through a consistent command interface.

### Added

- **Server administration** — users and groups, SSH keys and server config, packages, systemd services, cron jobs, and Supervisor daemons.
- **Web projects** — create and manage nginx-backed projects for static, PHP, Node.js, and Ruby apps, including SSL certificates via Let's Encrypt.
- **Databases and cache** — MySQL/MariaDB database and user management, plus Redis and Memcached setup.
- **Security and monitoring** — UFW firewall rules, server status and resource usage, and system inspection via `abstrax doctor`.
- **Plugin system** — install, update, and remove registry-backed CLI plugins with command delegation and metadata protocol v1.
- **Scripting support** — machine-readable `--json` output on all commands, including `abstrax project inspect --json` (v1) for plugins.
- **Project services** — `abstrax project service restart|reload` for project-owned supervisor services.
- **Reference plugin** — example plugin at `cli/cmd/abstrax-example`.

See the [documentation](https://useabstrax.com/docs) for the full list of commands, flags, and guides.

## [0.1.0 – 0.10.12] - Alpha releases

Versions v0.1.0 through v0.10.12 were alpha releases published during early development. They are superseded by v1.0.0.

See the [GitHub releases page](https://github.com/useabstrax/abstrax/releases) for changelogs and download links for those versions.
