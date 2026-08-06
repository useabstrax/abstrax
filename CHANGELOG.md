# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`daemon install`** - Installs Supervisor and enables/starts its service, matching `mysql install` / `firewall install`.

### Fixed

- **PHP install on Debian-family** - PHP installs now enable Ondřej Surý’s repository first (`packages.sury.org` on Debian, `ppa:ondrej/php` on Ubuntu), fixing missing packages like `php8.5-fpm`.
- **MySQL install on Debian** - `mysql install` now installs `mariadb-server` on Debian and Raspberry Pi OS, where `mysql-server` is not available in default apt repos.
- **Firewall strategy on Debian-family** - Doctor and firewall commands now treat UFW as the intended backend even when it is not installed yet, and point users at `firewall install`.
- **Daemon commands without Supervisor** - Missing `supervisorctl` now hints at `sudo abstrax daemon install` instead of a raw executable-not-found error.

## [2.0.1] - 2026-07-29

### Fixed

- **Passwordless sudo for managed users** - `user add --grant-sudo` and `user grant-sudo` now install an `/etc/sudoers.d/abstrax-<user>` drop-in with `NOPASSWD`, so sudo works without a password prompt.

## [2.0.0] - 2026-07-21

### Added

- **Platform profiles and detection** - Detects distro family, package/service managers, nginx layout, web user, PHP-FPM strategy, firewall, and support level from `/etc/os-release`.
- **Debian-family provider** - Centralises apt/systemd conventions: nginx sites layout, `www-data`, `/var/www`, versioned PHP-FPM, and UFW.
- **RHEL-family provider** - Official support for Rocky/Alma 9+; experimental RHEL, CentOS Stream, and Oracle Linux 9+ with dnf, `conf.d`, firewalld, and SELinux reporting.
- **DNF package backend** - Package commands use `dnf` on RHEL-family hosts and `apt` on Debian-family hosts.
- **firewalld backend** - RHEL-family firewall uses `firewall-cmd`; UFW remains on Debian-family systems.
- **Firewalld rule removal** - `firewall rule list` assigns Abstrax IDs; remove by ID, service, or port.
- **Remi multi-version PHP** - RHEL-family PHP uses Remi SCL packages with the same version-oriented project commands as Debian.
- **Repository helpers** - `abstrax repo enable <epel|crb|remi>` and `--enable-required-repos` for third-party repo consent.
- **RHEL runtime and service installs** - MariaDB, Certbot/EPEL, Supervisor, NodeSource, and stock Ruby installs adapted for RHEL-family hosts.
- **SELinux warnings** - Enforcing mode is reported in `doctor` and project/web flows; Abstrax never disables SELinux.
- **`firewall install`** - Installs the platform firewall package without enabling it.

### Changed

- **`abstrax doctor`** - Also reports distro profile, nginx config dir, web user/group, SELinux, firewalld, and support level.
- **Supported operating systems** - Official support is explicitly listed; other Debian/Ubuntu derivatives are `compatible`, others `unsupported`.
- **Mutating commands** - Unsupported distributions get a clear error before any destructive changes.
- **Nginx site enable/disable** - Debian keeps sites-available/enabled symlinks; RHEL writes `conf.d` and disables via `.disabled`.
- **Documentation** - Supported platforms docs cover parity, Remi PHP, repo consent, firewalld, and known limitations.

### Fixed

- **Remi / CRB on EL10** - Remi and CodeReady Builder repo names follow the host EL major version instead of hardcoding EL9.
- **PHP nginx virtual hosts on RHEL** - Inline fastcgi directives instead of Debian-only `snippets/fastcgi-php.conf`.
- **Redis / Memcached on RHEL** - Provider-aware paths; Rocky/Alma 10+ installs Redis from Remi (AppStream ships Valkey).
- **Firewall on RHEL** - `firewall enable` installs and starts firewalld first; missing backends point to `firewall install`.

## [1.1.1] - 2026-06-24

### Fixed

- **PHP nginx virtual hosts** - PHP locations use `$realpath_root` for `SCRIPT_FILENAME`/`DOCUMENT_ROOT` so OPcache sees new deploys without reloading FPM.

## [1.1.0] - 2026-06-23

### Removed

- **Automatic file backups** - No longer writes `.abstrax-bak.<timestamp>` copies before overwriting managed files; leftover cron backups could appear as phantom jobs.

## [1.0.0] - 2026-06-23

First stable release of the Abstrax CLI - a single Go binary for managing common Linux server tasks through a consistent command interface.

### Added

- **Server administration** - Users and groups, SSH keys and server config, packages, systemd services, cron jobs, and Supervisor daemons.
- **Web projects** - Nginx-backed projects for static, PHP, Node.js, and Ruby apps, including Let's Encrypt SSL.
- **Databases and cache** - MySQL/MariaDB management, plus Redis and Memcached setup.
- **Security and monitoring** - UFW firewall rules, server status, and `abstrax doctor`.
- **Plugin system** - Install, update, and remove registry-backed CLI plugins.
- **Scripting support** - Machine-readable `--json` output on all commands, including `project inspect`.
- **Project services** - `abstrax project service restart|reload` for project-owned supervisor services.
- **Reference plugin** - Example plugin at `cli/cmd/abstrax-example`.

See the [documentation](https://useabstrax.com/docs) for the full list of commands, flags, and guides.

## [0.1.0 - 0.10.12] - Alpha releases

Versions v0.1.0 through v0.10.12 were alpha releases published during early development. They are superseded by v1.0.0.

See the [GitHub releases page](https://github.com/useabstrax/abstrax/releases) for changelogs and download links for those versions.
