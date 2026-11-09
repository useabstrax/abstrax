# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Plugin command names with spaces** - Plugin install rejected grouped commands such as `file list` and `process history`. A command `name` may now be one or more words separated by a single space; each word still follows the existing lowercase / digits / dashes rule.

## [3.2.0] - 2026-11-03

### Added

- **`firewall defaults`** - Shows the default firewall policy as `{backend, defaults}`: `defaults.incoming` and `defaults.outgoing` (`allow`, `deny`, or `reject`), `defaults.routed` (UFW only, for example `disabled`), and `defaults.ping` (`allow` or `block`; `mixed` if the IPv4 and IPv6 rules disagree, `unknown` if it cannot be read). On UFW it reads `ufw status verbose`, and `/etc/default/ufw` while UFW is inactive. On firewalld it reads the target of the default zone: `default` and `REJECT` are `reject`, `DROP` is `deny`, and `ACCEPT` is `allow`. firewalld reports `outgoing` as `allow`. Registered as `firewall.defaults`.
- **`firewall defaults set`** - `firewall defaults set --incoming deny|reject|allow [--outgoing allow|deny|reject]` runs `ufw default <policy> incoming|outgoing` on UFW, or sets the default zone target (`DROP`, `%%REJECT%%`, `ACCEPT`) and reloads on firewalld. `--outgoing` is refused on firewalld. `--incoming allow` warns and asks for confirmation (`--yes` skips it). `--incoming deny` or `reject` is refused while the firewall is active and no rule allows TCP to the SSH port, unless the policy already blocks incoming traffic. `--force` applies it anyway; `--yes` does not. Successful `--json` output includes `ssh_check` when the check ran. Registered as `firewall.defaults.set`.
- **`firewall ping`** - `firewall ping allow|block` controls inbound ping (ICMP echo requests). On UFW it switches the INPUT `echo-request` rules in `/etc/ufw/before.rules` and `before6.rules` between `ACCEPT` and `DROP`, writes a `.bak` of each changed file, reloads UFW, and restores the files if the reload fails. On firewalld it adds or removes the `echo-request` ICMP block in the default zone and reloads. Registered as `firewall.ping`.
- **`firewall status` defaults** - `firewall status --json` includes a `defaults` object with the same fields as `firewall defaults`. It is omitted if the defaults cannot be read.

- **`package auto-updates`** - `package auto-updates status|enable|disable` manages automatic security updates, registered as `package.auto-updates.status`, `package.auto-updates.enable`, and `package.auto-updates.disable`. Updates are limited to security origins, and the server is never rebooted automatically; `server reboot-required` reports when a reboot is needed. `enable` and `disable` require root. `status` changes nothing and does not require root. It returns `backend`, `installed`, `enabled`, `managed`, `config_path`, `last_run` (RFC3339 UTC, or `null`), and `last_result` (`success`, `failed`, `unknown`, or `never_run`).

  - **Debian and Ubuntu** - Installs `unattended-upgrades` when it is missing and writes `/etc/apt/apt.conf.d/52abstrax-auto-updates`. The file sets `APT::Periodic::Unattended-Upgrade "1"`, replaces the distro origin lists with security origins only, and sets `Automatic-Reboot "false"`. `disable` rewrites the same file with `APT::Periodic::Unattended-Upgrade "0"`. Distro files are not edited. `status` reads the effective setting from `apt-config`. The last run is the mtime of `/var/lib/apt/periodic/unattended-upgrades-stamp`, and the result comes from the last outcome line in `/var/log/unattended-upgrades/unattended-upgrades.log`.

  - **RHEL family** - Installs `dnf-automatic`, writes `/etc/abstrax/dnf-automatic.conf` (`upgrade_type = security`, `apply_updates = yes`, `reboot = never`), and adds a systemd drop-in `/etc/systemd/system/dnf-automatic-install.service.d/50-abstrax.conf` that runs `dnf-automatic` with that file. Then it runs `systemctl enable --now dnf-automatic-install.timer`. `disable` stops and disables the timer, removes both Abstrax files, and reloads systemd, which leaves `/etc/dnf/automatic.conf` untouched. The last run is the mtime of the timer stamp `/var/lib/systemd/timers/stamp-dnf-automatic-install.timer`. The result comes from `systemctl show dnf-automatic-install.service` and is only known for runs since boot; an earlier run reports `unknown`.


## [3.1.0] - 2026-11-03

### Added

- **Extended `project inspect`** - Still `api_version: v1`; the new fields are additive. The payload now includes `web_server`, `vhost` (`present`, `path`, `enabled`), `public_dir` and `proxy_port` (omitted when unset), `shared_logs`, `ssl` (`enabled`, plus `certificate_name`, `domains`, `redirect_http`, `staging`, and `expires_at` when known), and `created_at` / `updated_at` as RFC3339 UTC strings. `vhost.present` is true when the project records a vhost path. `vhost.enabled` is false when `vhost_disabled` is set, and otherwise is read from disk: on Debian-family systems the `sites-enabled` symlink must exist and resolve, and on RHEL-family systems the file in `conf.d` must exist (a `.disabled` file is disabled).
- **`project redirect`** - `project redirect list|add|modify|remove` manages exact-path redirects for a project (`project.redirect.list`, `.add`, `.modify`, `.remove`). `add <project> <source> <destination> [--code]` takes a source path starting with `/`, a destination that is a path or an absolute `http(s)` URL, and a status code of 301 (default), 302, 307, or 308. Each redirect has a short ID shown by `list`; `modify` and `remove` take that ID. Redirects are stored in the project state and written as `location = <source> { return <code> <destination>; }` blocks ahead of the runtime locations in the managed nginx vhost, sorted by ID, so `project modify` keeps them. The project needs a managed nginx vhost. Sources and destinations containing whitespace, quotes, `;`, braces, backslashes, or `$` are rejected. The vhost is written before state is saved, and a vhost that nginx rejects is rolled back to its previous contents.
- **`project vhost add` and `project vhost remove`** - `project vhost add <name>` creates an nginx vhost for a project that has none, such as one added with `--no-vhost`. `--domains`, `--port`, and `--web-root` override the stored values. `project vhost remove <name>` deletes the vhost (and its `.disabled` copy), clears `vhost_path` and `ssl_enabled`, reloads nginx, and keeps the project files and state. Registered as `project.vhost.add` and `project.vhost.remove`. The result is the updated project state.
- **SSL state on projects** - `project info` and the project state file return an `ssl` object with `certificate_name`, `domains`, `redirect_http`, `staging`, `issued_at`, `expires_at`, and `last_error`. `ssl_enabled` is still written. A project that only has `ssl_enabled` is adopted the first time Abstrax needs its certificate name: the name comes from the `ssl_certificate` path in the existing vhost, then from the first domain.
- **`ssl status` fields** - Each entry reports `project_name` (the real project name), `certificate_name`, `expiry` (RFC3339), `days_remaining`, `redirect_http`, `staging`, and `status` (`valid`, `expiring` within 30 days, `expired`, or `missing`). Status is read from project state and the certificate under `/etc/letsencrypt/live`. `certbot certificates` output is only a fallback, and also lists certificates no project owns.
- **`project runtime set`** - Changes a project between PHP, Node.js, Ruby, and static: `project runtime set <name> --php|--node|--ruby|--static`, registered as `project.runtime.set`. Exactly one runtime selector is required. `--php-version`, `--node-version`, `--ruby-version`, `--public-dir`, and `--proxy-port` apply to the matching runtime and are refused for others. The target runtime is installed first (with confirmation unless `--yes`). Fields from the old runtime are cleared: PHP keeps `php_version` and `public_dir`, Node.js and Ruby keep their version and `proxy_port` (the port carries over between them), and static keeps `public_dir` only. On a user-isolated project, leaving PHP removes its PHP-FPM pool, entering PHP creates one, and changing the PHP version creates the new pool, switches the vhost, and then removes the old pool. The nginx vhost is regenerated; if `nginx -t` rejects it, the previous vhost is restored and the project is left as it was. A disabled vhost stays disabled. `--dry-run` previews the new state without changing anything.

### Changed

- **`project modify` clears fields explicitly** - A flag passed with an empty value now clears the stored field instead of being ignored: `--domains=""`, `--public-dir=""`, `--proxy-port=0`, and an empty `--php-version`, `--node-version`, or `--ruby-version`. Flags that are not passed leave the field unchanged. An empty version is cleared, but the active runtime keeps its default version. `--clear-domains` removes every domain and cannot be combined with `--domains`. Removing every domain (with `--domains`, `--clear-domains`, or `--remove-domain`) is refused when the project has an nginx vhost, because the vhost would be written with `server_name _` and answer for every host. `--add-domain` and `--domains` validate each domain. `--proxy-port` is validated as a port, and `--public-dir` must stay inside the project path. `--path` must not be empty and is checked as `project add` checks it. It updates the stored path only and does not move files on disk.
- **`ssl add`** - Runs `certbot certonly --nginx --cert-name abstrax-<project>` and then writes HTTPS into the project's nginx vhost itself, validating with `nginx -t` before reloading. Certbot no longer edits the vhost. The certificate name is stored and reused for later adds, renewals, status, and removal. A deploy hook reloads nginx after each renewal. If nginx rejects the HTTPS vhost, the previous vhost is restored, `ssl_enabled` stays off, and `ssl.last_error` records why.
- **`ssl remove`** - Rewrites the vhost without HTTPS and runs `nginx -t` before `certbot delete`, then clears the stored SSL state. If nginx rejects the vhost, the certificate is left alone. If `certbot delete` fails, HTTPS is put back.
- **`ssl renew`** - Uses the stored certificate name, reloads nginx, and refreshes `ssl.expires_at`.
- **Project vhosts** - The nginx vhost renders an HTTPS server (`listen 443 ssl`) for projects with SSL enabled, with an HTTP server that redirects when `redirect_http` is true. A failed vhost rewrite restores the previous file.
- **`project enable` and `project disable`** - Store the choice as `vhost_disabled` in the project state. `project modify` keeps a disabled vhost disabled when it rewrites the file, and previously turned it back on.
- **Vhost rewrites** - `project modify` and `project vhost add` restore the previous vhost file when `nginx -t` rejects the new one. On Debian-family systems the new file is now enabled before `nginx -t` runs, so the test checks it.
- **Project state** - `project add` stores `port` and `web_root`, so a later vhost rewrite keeps them.

### Fixed

- **`--action` / `--payload` booleans** - A JSON `false` value now encodes as `--key=false` instead of being dropped. Agents can turn off default-true flags such as `shared-logs` and `redirect-http`. Boolean `true` still emits a bare `--key`.
- **`project modify` side effects** - The runtime install check runs only when the active runtime's version or the runtime changes. The nginx vhost is rewritten and reloaded only when a value that appears in it changed. Previously every `project modify` did both.
- **`project modify` removing HTTPS** - Rewriting the vhost dropped the certbot-added HTTPS server, so a modified project silently lost SSL. The vhost is now rendered from stored SSL state, so HTTPS stays in place.

## [3.0.0] - 2026-11-02

### Added

- **Server timezone** - `server status` includes the zone name, abbreviation, and current UTC offset. Cron schedules and timestamps use this clock.
- **`ssh config enable-root-login --keys-only`** - Writes `PermitRootLogin prohibit-password`, so root can log in with an SSH key and not a password. Without the flag, the command still writes `yes`. `ssh config disable-root-login` still writes `no` and blocks root for every method.
- **SSH key restrictions** - `ssh-key add --expires` writes OpenSSH `expiry-time` (UTC) and `--from` writes a `from` allow-list. `ssh-key list` and `ssh-key info` return `expires_at`, `from`, and `expired`. `--expires` is refused on OpenSSH older than 8.2. Keys added without the flags are written as before.
- **`mysql status`** - Reports whether a MySQL-compatible server is installed, which engine (`mysql` or `mariadb`) and version are present, and whether the service is active and enabled. The command succeeds when MySQL is missing or stopped. When nothing is installed, `engine` is the server Abstrax would install on this platform and `version` is null.
- **`mysql remote`** - `mysql remote enable` writes an Abstrax-managed `zz-abstrax-remote.cnf` drop-in so MySQL or MariaDB listens beyond loopback, restarts the database, and waits until it accepts connections. If the service does not come back, the drop-in is removed and the service is restarted again. `mysql remote disable` removes that drop-in, with the same rollback (the file is put back if the server does not return). `mysql remote status` reports the bind address the running server is using. `--bind` chooses the address (default `0.0.0.0`). Confirmation is required unless `--yes` is set. The firewall is not opened; port 3306 must be allowed separately. On MySQL the drop-in also sets `mysqlx-bind-address`.
- **`mysql user password <name>`** - Changes the password for an existing MySQL or MariaDB user without altering grants or the auth plugin. A password is generated unless `--password` is passed, which prompts for one. With no `--host`, every host the user exists on is updated. `--host localhost` updates `localhost` and `127.0.0.1`. Any other `--host` updates only that account.
- **`cron run <id>`** - Runs a managed cron job immediately as its user, with the job's environment and working directory. `--timeout` defaults to 300 seconds (`0` means no limit) and the command exits 124 when the process group is killed. `--use-job-output` applies the job's log redirects. A disabled job still runs and the result reports it as disabled.
- **`server reboot`** - Reboots the machine after confirmation. `--yes` skips the prompt. `--delay` waits that many seconds; `0` runs `systemctl reboot` immediately. A positive delay uses `systemd-run --on-active=<seconds>s` when `systemd-run` is available, otherwise `shutdown -r` with the delay rounded up to whole minutes. The result is printed before the reboot command runs.
- **`server reboot-required`** - Reports whether a reboot is required and which packages requested it. On Debian and Ubuntu it reads `/var/run/reboot-required`. On RHEL-family systems it runs `needs-restarting -r`, and reports a note instead of failing when that command is not installed. Does not require root. Registered as `server.reboot-required`.
- **`package upgrades`** - Lists pending package upgrades with the installed version, the new version, and whether each update is a security update. `--security-only` limits the list to security updates. Does not require root. Registered as `package.upgrades`.
- **`user info` and `user list`** - Include the last login time and where it came from. `last_login_at` is an RFC3339 timestamp or null. `last_login_from` is an IP address or hostname, or null. `user list` reads the login log once. A missing log leaves the fields null and the command still succeeds.
- **`user sessions` and `user logout`** - `user sessions <name>` lists interactive login sessions (session id, TTY, source IP, login time, and idle time). `user logout <name> --session <id>` ends one session. `user logout <name> --all` ends each of that user's login sessions. Daemons, cron jobs, and systemd user-manager sessions running as the user are left running.
- **`user remove --if-unused`** - Refuses to remove a user, and exits non-zero, when an Abstrax daemon, an Abstrax cron job, or a running process still uses that account.
- **Per-project nginx logs** - A managed vhost logs to `/var/log/nginx/<project>-access.log` and `/var/log/nginx/<project>-error.log`. `project info` and `project inspect` return those paths as `access_log_path` and `error_log_path`. `project add --shared-logs` and `project modify --shared-logs` omit the directives and keep the server-wide nginx logs. The choice is stored as `shared_logs`, so a later `project modify` that does not pass the flag leaves it unchanged. `--shared-logs=false` turns per-project logs back on. Existing vhosts are not rewritten on upgrade. They pick up per-project logs the next time `project modify` rewrites the vhost. `--shared-logs` keeps the previous server-wide behaviour. `project remove` keeps the log files. `project remove --delete-files` deletes the project files and those two log files. Passing `--keep-files` as well still keeps both.

### Changed

- **`package upgrade --security-only`** - On Debian and Ubuntu, installs only the security updates listed by `package upgrades`. If none are pending, the command succeeds without installing anything. On yum, runs `yum update -y --security`. A full `package upgrade` is unchanged.
- **`user remove`** - Warns when Abstrax daemons, cron jobs, or running processes still use the user. The user is still removed unless `--if-unused` is set. Daemons and cron jobs are not stopped. JSON results include `still_used_by`.

### Fixed

- **`mysql user remove`** - A local user is dropped from both `localhost` and `127.0.0.1`. Previously only `'user'@'localhost'` was removed, so the `127.0.0.1` account created by `mysql user add` was left behind.
- **`project remove --delete-files`** - Deletes the project files. The default `--keep-files` previously stayed on, so `--delete-files` alone had no effect unless `--keep-files=false` was also passed.

### Security

- **Firewall SSH access** - `firewall deny`, `firewall rule remove`, `firewall remove service`, and `firewall remove port` are refused when the firewall is active and the change would leave no rule allowing TCP to the SSH port. An allow limited to one address still counts. `--force` applies the change anyway. `--yes` does not. When the firewall is inactive, the change is allowed and Abstrax warns that `firewall enable` will need `--allow-ssh`. Successful `--json` output includes `ssh_check`. Automatic client-IP protection on `allow`, `deny`, `allow-ip`, and `deny-ip` runs only for an interactive SSH session (`SSH_CONNECTION`, `SSH_CLIENT`, or `who`). From a script, it does nothing.

## [2.4.0] - 2026-10-30

### Security

- **Cron files** - `cron add` and `cron modify` reject line breaks, carriage returns and null bytes in every value, and reject invalid users and environment names. `%` in a command is written as `\%` and read back as `%`, so it is a literal percent instead of cron's stdin separator. Working directory and output paths are shell-quoted.
- **Daemon configs** - `daemon add` and `daemon modify` reject line breaks, carriage returns and null bytes, and reject invalid users, paths, stop signals, exit codes and environment names. `%` is written as `%%` so Supervisor treats it as a literal percent. Environment values are double-quoted. Values containing `"` or `\` are rejected because Supervisor 4.2 parses `environment` with non-POSIX `shlex`, which does not treat `\` as an escape.
- **MySQL SQL** - Privileges are parsed against an allowlist and the GRANT statement is built from that list. Hosts must match a safe character set and are escaped, and database names are checked on `user add --grant-db` as well as `grant`. Column-level privileges and hosts outside that character set are rejected.

### Added

- **`daemon info <name>`** - Prints the settings from the Supervisor config (command, directory, user, numprocs, restart policy, and log paths) plus live status. Registered as the `daemon.info` action.
- **`mysql user list --grants`** - Includes each user's raw grants and parsed per-database privileges in one call. `--exclude-system` hides built-in accounts. Without `--grants`, the list is unchanged.

### Changed

- **Daemon list and status JSON** - `daemon list` and `daemon status` group processes under one daemon. `status` is the worst process state. Each process includes `pid`, `uptime`, and `uptime_seconds` while it is running. `daemon list` shows Abstrax-managed daemons by default; `--all` includes other Supervisor programs with `managed: false` and no `config_path`.
- **`user lock`** - Also expires the account, so SSH key logins are refused as well as passwords. `user unlock` restores the previous expiry. `--password-only` keeps the old password-only lock. `user info` and `user list` add `expired` and `login_blocked`. `locked` still means the password is locked.
- **`ssh config set-port`** - With `--allow-firewall`, the new port is opened before sshd is reloaded. The previous port stays open until `--close-old-port`. The change is rolled back if the new port does not answer. SELinux port labels are applied only when SELinux is enforcing, and socket-activated sshd gets a drop-in that listens on both ports.

### Fixed

- **SSH config writes** - `ssh config` writes `/etc/ssh/sshd_config.d/00-abstrax.conf`, which sshd reads before drop-ins such as `50-cloud-init.conf`. The next write moves an existing `99-abstrax.conf` into that file, so settings that were being overridden start to apply. If `sshd_config` does not include the directory, or sets the same keyword before `Include`, that keyword is updated in `sshd_config`. After reload, `sshd -T` is checked and the command fails, naming the file that still wins, when the value did not take effect. `--dry-run` writes nothing.
- **`ssh config show`** - Reports the settings `sshd -T` is using, including defaults, instead of only the Abstrax drop-in. JSON keeps the same keys and adds a source for each value. If `sshd -T` is unavailable, the command says so and reads the config files instead.
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
