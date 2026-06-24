# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
