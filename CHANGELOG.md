# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- **Cron files** - `cron add` and `cron modify` reject line breaks, carriage returns and null bytes in every value, and reject invalid users and environment names. `%` in a command is written as `\%` and read back as `%`, so it is a literal percent instead of cron's stdin separator. Working directory and output paths are shell-quoted.

### Added

- **`daemon info <name>`** - Prints the settings from the Supervisor config (command, directory, user, numprocs, restart policy, and log paths) plus live status. Registered as the `daemon.info` action.
- **`mysql user list --grants`** - Includes each user's raw grants and parsed per-database privileges in one call. `--exclude-system` hides built-in accounts. Without `--grants`, the list is unchanged.

### Changed

- **Daemon list and status JSON** - `daemon list` and `daemon status` group processes under one daemon. `status` is the worst process state. Each process includes `pid`, `uptime`, and `uptime_seconds` while it is running. `daemon list` shows Abstrax-managed daemons by default; `--all` includes other Supervisor programs with `managed: false` and no `config_path`.

### Fixed

- **UFW rule identity** - `firewall rule list` returns a stable `id` (a short hash of the rule) and keeps the current UFW position as `number`. `firewall rule remove <id>` deletes that rule by its specification, so a later removal cannot hit a rule that shifted into an old position. Numeric positions still work and print a warning. Listed rules now include protocol, direction, source, destination, comment, and IPv6. `firewall allow-ip` accepts `--protocol` and `--comment`; `firewall deny-ip` accepts `--comment`.
- **firewalld source-limited allows** - `firewall allow <port> --from <ip>` on firewalld now adds a rich rule for that source. It no longer opens the port to every address, including for ports 80 and 443. Rich rules are listed and can be removed by id. `firewall deny <port>` adds IPv4 and IPv6 reject rules. IPv6 sources use `family="ipv6"`. Firewalld rule ids are a stable hash, with the list position kept as `number`.
- **Cron job fields** - Working directory and output redirects are stored separately from the command, so `cron info` returns `working_dir`, `output`, `error_output`, `append_output`, and `discard_output`, and `cron modify` no longer nests `cd` or redirects. Environment values with spaces round-trip. Schedules are validated per field, and `@reboot`, `@hourly`, `@daily`, `@weekly`, `@monthly`, `@yearly`, `@annually`, and `@midnight` are accepted. `cron modify` can set `--append-output`, `--discard-output`, and `--no-output`.
- **Multi-process daemons** - `start`, `stop`, and `restart` address the Supervisor group as `<name>:*`. `daemon logs --lines N` returns N lines from the log file. `--stderr` works, and `--process N` selects one process when `numprocs` is greater than 1.
- **`daemon modify`** - Only flags that were passed are written. Omitted settings are read back from the existing Supervisor config instead of being reset. Modify accepts the same settings as `daemon add`, including restart policy and log paths. `--environment` merges by key; `--clear-environment` and `--unset-environment` remove keys. `--autorestart` must be `always`, `unexpected`, or `false`. `supervisorctl update` still runs afterwards and restarts the program when the config changed.
- **`mysql user info`** - Looks up every host the user exists on, including a specific IP, instead of only `localhost`, `127.0.0.1`, and `%`. `--host` limits the result to one host. JSON output is a list, one entry per host.

## [2.3.1] - 2026-08-19

### Fixed

- **Plugin install from registry archives** - `plugin install` extracts `abstrax-<name>` from gzip-compressed tar archives after checksum verification. Registry plugins publish `.tar.gz` files; the installer previously tried to execute the archive, which failed with `exec format error`.

## [2.3.0] - 2026-08-19

### Added

- **`--action` / `--payload`** - Agents can run any builtin or plugin command as `abstrax --json-stream --yes --action user.add --payload '{"args":["alice"]}'`. Plugin jobs use `plugin.<name>.<command>` (for example `plugin.deploy.now`). `--payload -` reads JSON from stdin.

## [2.2.0] - 2026-08-14

### Added

- **Local plugin install** - `plugin install --path <binary>` (or a filesystem path as the argument) links a non-registry plugin into the install directory. Shows an unofficial/unverified warning and confirmation. `plugin remove` deletes only the symlink, not the original binary. Local plugins cannot be updated via `plugin update`; reinstall with `--path` instead.

## [2.1.0] - 2026-08-08

### Added

- **`--json-stream`** - NDJSON progress events plus a final `type=result` line for machine consumers; mutually exclusive with `--json`. `project add` emits staged progress (`validate`, `directories`, `nginx_vhost`, and related steps).
- **`daemon install`** - Installs Supervisor and enables/starts its service, matching `mysql install` / `firewall install`.

### Removed

- **Built-in agent stubs** - Removed the placeholder `abstrax agent` command, unused `Agent*` action constants, and the packaged disabled `abstrax-agent.service` unit so an official `agent` plugin can own that name later.

### Fixed

- **PHP install on Debian-family** - PHP installs now enable Ondřej Surý’s repository first (`packages.sury.org` on Debian, `ppa:ondrej/php` on Ubuntu), fixing missing packages like `php8.5-fpm`.
- **MySQL install on Debian** - `mysql install` now installs `mariadb-server` on Debian and Raspberry Pi OS, where `mysql-server` is not available in default apt repos.
- **Firewall strategy on Debian-family** - Doctor and firewall commands now treat UFW as the intended backend even when it is not installed yet, and point users at `firewall install`.
- **Daemon commands without Supervisor** - Missing `supervisorctl` now hints at `sudo abstrax daemon install` instead of a raw executable-not-found error.
- **Redis install Remi messaging** - Enabling Remi for Redis on Rocky/Alma 10+ no longer says it is required for multi-version PHP.

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
