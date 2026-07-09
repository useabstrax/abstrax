# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **RHEL-family provider** — Initial support for Rocky Linux 9+ and AlmaLinux 9+ (official), plus experimental compatibility for RHEL 9+, CentOS Stream 9+, and Oracle Linux 9+. Detection classifies `rocky`, `almalinux`, `rhel`, `centos`, `ol`, and `oracle` as the `rhel` family with `dnf`, systemd, nginx `conf.d`, `nginx`/`nginx` web user, Remi multi-version PHP, firewalld, and SELinux status reporting.
- **DNF package backend** — Package commands select `dnf` on RHEL-family hosts and `apt` on Debian-family hosts.
- **firewalld backend** — Firewall commands use firewalld on RHEL-family systems (permanent rules + reload) while preserving UFW behaviour on Debian-family systems.
- **Firewalld rule removal** — `firewall rule list` assigns Abstrax IDs to firewalld services/ports; `firewall rule remove <id>` removes the matching entry. Also `firewall remove service` and `firewall remove port`.
- **Remi multi-version PHP** — RHEL-family PHP installs use Remi SCL packages (`php83-php-fpm`, paths under `/opt/remi` and `/etc/opt/remi`) with the same version-oriented project commands as Debian.
- **Repository helpers** — `abstrax repo enable <epel|crb|remi>` and global `--enable-required-repos` for explicit third-party repo consent (required for Remi; required for EPEL on RHEL/Oracle).
- **SELinux warnings** — Enforcing mode is detected and surfaced in `doctor` and project/web flows; Abstrax never disables SELinux automatically.
- **RHEL MariaDB install** — `mysql install` installs `mariadb-server` on RHEL-family systems as the MySQL-compatible database server.
- **RHEL Certbot install** — `ssl install` installs Certbot via dnf and enables EPEL on Rocky/Alma/CentOS Stream when required (RHEL/Oracle need `--enable-required-repos` or `repo enable epel`).
- **RHEL Supervisor install** — Daemon auto-install uses `supervisord`, `/etc/supervisord.d`, and `.ini` program configs on RHEL-family systems.
- **RHEL Node.js / Ruby install** — Project runtime install uses NodeSource RPM setup scripts and stock `ruby`/`ruby-devel` packages on RHEL-family systems. Exact Ruby version pinning on RHEL remains a deliberate product limitation.

### Changed

- **`abstrax doctor`** — Reports nginx config directory, web user/group, SELinux status, and firewalld tool presence for RHEL-family systems.
- **Nginx site enable/disable** — Provider-aware: Debian continues to symlink `sites-available` → `sites-enabled`; RHEL writes `/etc/nginx/conf.d/{site}.conf` and disables by renaming to `.disabled`.
- **Functional parity docs** — Supported platforms documentation now describes the parity model, Remi PHP, repository consent, and remaining deliberate limitations.


### Added

- **Platform profiles** — Abstrax now reads `/etc/os-release` (including `ID_LIKE` and `VERSION_ID`) and derives a platform profile covering distro family, package and service managers, nginx layout, web user, default project root, PHP-FPM naming, firewall strategy, and support level (`official`, `compatible`, or `unsupported`).
- **Debian-family provider** — Paths and naming conventions for apt/systemd, nginx `sites-available`/`sites-enabled`, `www-data`, `/var/www`, PHP-FPM services and sockets, and UFW are centralised in a single provider rather than scattered through command code.
- **Enhanced `abstrax doctor`** — Reports distro ID, family, nginx layout, web user, project root, PHP-FPM strategy, firewall strategy, and support level alongside the existing tool and manager detection.

### Changed

- **Supported operating systems** — Fully supported distros are now explicitly defined as Ubuntu 20.04+, Debian 11+, Linux Mint, Pop!_OS, and Raspbian / Raspberry Pi OS. Other Debian/Ubuntu-based systems are marked `compatible` (best-effort). Non-Debian-family distributions are `unsupported`.
- **Mutating commands** — Commands that change system state now verify platform support before running. Unsupported distributions receive a clear error explaining what was detected and which distros are supported, without attempting destructive changes.

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
