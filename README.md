<p align="center">
  <img src="docs/abstrax-icon.svg" alt="Abstrax" width="80" />
</p>

<h1 align="center">Abstrax</h1>

<p align="center">
  A server management CLI for Linux.
</p>

<p align="center">
  <a href="LICENSE">Apache License 2.0</a> ·
  <a href="https://useabstrax.com/docs">Documentation</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

---

Abstrax is a command line tool for managing common Linux server tasks. It wraps everyday administration for users, packages, services, web projects, databases, firewalls, and more behind a single, consistent interface.

You should not need to remember the exact syntax of `useradd`, `ufw`, `supervisorctl`, `certbot`, or nginx configuration files to get routine server work done.

```bash
abstrax user add deploy --grant-sudo
abstrax package install nginx
abstrax firewall allow 443 --protocol=tcp
```

Every command validates its input, performs the requested change, and prints a clear result. Add `--json` for a single machine-readable result, `--json-stream` for NDJSON progress on long-running commands such as `project add`, `--dry-run` to preview changes, or `--yes` to skip confirmation prompts. See the [documentation](https://useabstrax.com/docs) for all global flags.

Abstrax is in active development. A hosted management platform and local agent are planned as an optional official plugin (`abstrax plugin install agent`), not as part of the core CLI package.

## Supported platforms

Abstrax runs on **Linux only**. Release builds are published for:

| Architecture | Notes |
|---|---|
| `linux/amd64` | x86_64 servers |
| `linux/arm64` | ARM / aarch64 servers |

**Fully supported** (official):

Debian/Ubuntu family:

- Ubuntu 20.04+
- Debian 11+
- Linux Mint
- Pop!\_OS
- Raspbian / Raspberry Pi OS

RHEL-compatible family:

- Rocky Linux 9+
- AlmaLinux 9+

**Experimental / compatible** (mutating commands allowed, best-effort):

- Other Debian/Ubuntu derivatives
- Red Hat Enterprise Linux 9+
- CentOS Stream 9+
- Oracle Linux 9+

Other families, and RHEL-family releases older than major version 9, are unsupported. Abstrax detects them and exits cleanly before making unsafe changes. Run `abstrax doctor` to see what was detected, including support level. See [supported platforms](https://useabstrax.com/docs/reference/supported-platforms) for family conventions (`apt`/`dnf`, UFW/firewalld, Remi PHP, and SELinux).

## Installation

Full installation instructions, checksum verification, and package options are in the [getting started guide](https://useabstrax.com/docs/getting-started).

### From a release archive

Download a release archive from the [releases page](https://github.com/useabstrax/abstrax/releases), verify the checksum, and install the binary:

```bash
VERSION="1.0.0"   # without the leading v
ARCH="amd64"      # or arm64 on ARM servers

wget "https://github.com/useabstrax/abstrax/releases/download/v${VERSION}/abstrax_${VERSION}_linux_${ARCH}.tar.gz"
wget "https://github.com/useabstrax/abstrax/releases/download/v${VERSION}/abstrax_${VERSION}_checksums.txt"
sha256sum -c "abstrax_${VERSION}_checksums.txt" 2>&1 | grep "abstrax_${VERSION}_linux_${ARCH}.tar.gz"
tar -xzf "abstrax_${VERSION}_linux_${ARCH}.tar.gz"
chmod +x abstrax
sudo mv abstrax /usr/local/bin/abstrax
```

See the [getting started guide](https://useabstrax.com/docs/getting-started) for checksum verification details and a command to resolve the latest version automatically.

### Using the install script

```bash
wget -qO- https://useabstrax.com/install.sh | sudo bash
```

The script downloads the latest release, verifies checksums, and installs to `/usr/local/bin/abstrax`.

### Build from source

Requires Go 1.22 or newer.

```bash
git clone https://github.com/useabstrax/abstrax
cd abstrax/cli
go mod download
go build -o abstrax ./cmd/abstrax
sudo mv abstrax /usr/local/bin/abstrax
```

See [building from source](https://useabstrax.com/docs/contributing/building-from-source) for version metadata and release builds.

## Quick start

After installing, confirm the CLI works and inspect your server:

```bash
abstrax --help
abstrax doctor
abstrax version

# Read-only
abstrax server status

# Preview a change (requires sudo for real execution)
sudo abstrax user add deploy --grant-sudo --dry-run
```

`abstrax doctor` does not require root and makes no changes - it is a safe first command. Most commands that change system state require `sudo`.

`server status` includes the server timezone (name, abbreviation, and current UTC offset). Cron schedules and log timestamps use that clock. The zone comes from `timedatectl` when it is available, otherwise from `/etc/localtime` or `/etc/timezone`.

Explore command groups with `abstrax <group> --help`, for example `abstrax user --help` or `abstrax firewall --help`.

### Server reboot

`server reboot` restarts the machine. It requires root and asks for confirmation unless you pass `--yes`. A positive `--delay` uses `systemd-run --on-active=<seconds>s -- systemctl reboot` when `systemd-run` is available, and otherwise rounds the wait up to whole minutes and runs `shutdown -r +N`. The command prints its result before that reboot runs, and `--dry-run` records the commands without rebooting.

```bash
sudo abstrax server reboot --yes
sudo abstrax server reboot --delay 90 --dry-run
```

### Package upgrades

`package upgrades` lists pending updates. Each row shows the package name, the installed version, the new version, and whether the update is a security update. `--security-only` shows security updates only. It does not require root.
`package upgrade --security-only` installs only those security updates. On Debian and Ubuntu that is the packages from the security pocket. On RHEL-family systems it runs `dnf upgrade -y --security`.
`server reboot-required` reports whether a reboot is needed. On Debian and Ubuntu it reads `/var/run/reboot-required`. On RHEL-family systems it runs `needs-restarting -r`. If that command is not installed, the result says so and does not fail.

### Firewall rule IDs

On UFW, `firewall rule list` gives each rule a stable `id` and a `number`. `id` is a short hash of the rule. `number` is its current position in `ufw status numbered` and can change when other rules are added or removed. Remove a rule with the stable id:

```bash
abstrax firewall rule list
sudo abstrax firewall rule remove a1b2c3d4
```

A numeric position still works (`firewall rule remove 3`) and prints a warning, because that number may point at a different rule than it did when you last listed them.

Removing the last rule that allows TCP to the SSH port is refused while the firewall is active. That includes `firewall deny` of the SSH port, `firewall remove port`, and `firewall remove service ssh`. An allow limited to one address still counts as access. Pass `--force` to apply the change anyway. `--yes` does not skip the check. When the firewall is inactive, the change is allowed and Abstrax warns that `firewall enable` will need `--allow-ssh`.

`firewall allow`, `deny`, `allow-ip`, and `deny-ip` allow your current client IP on the SSH port only in an interactive SSH session (`SSH_CONNECTION`, `SSH_CLIENT`, or `who`). From a script, that protection does nothing.

### Firewall defaults and ping

`firewall defaults` shows the default incoming and outgoing policy, the routed policy on UFW, and whether inbound ping is allowed. `firewall status --json` includes the same values in a `defaults` object.

```bash
abstrax firewall defaults --json
sudo abstrax firewall defaults set --incoming deny
sudo abstrax firewall defaults set --incoming reject --outgoing allow
sudo abstrax firewall ping block
```

`--incoming` takes `deny`, `reject`, or `allow`; `--outgoing` takes `allow`, `deny`, or `reject` and works on UFW only. On firewalld, the incoming policy is the target of the default zone (`DROP` is `deny`) and `--outgoing` is refused. `--incoming allow` asks for confirmation; pass `--yes` in scripts. `--incoming deny` or `reject` is refused while the firewall is active and no rule allows TCP to the SSH port, unless the policy already blocks incoming traffic. Pass `--force` to apply it anyway.

`firewall ping allow|block` switches the echo-request rules in `/etc/ufw/before.rules` and `before6.rules` on UFW (keeping a `.bak` of each changed file and reloading UFW), or the `echo-request` ICMP block in the default zone on firewalld.

### Cron jobs

`cron add` stores the command, working directory, and output redirect as separate fields. `cron info` and `cron list --json` return `working_dir`, `output`, `error_output`, `append_output`, and `discard_output` on their own, so `cron modify --command` does not drop or wrap them.

Schedules are checked field by field. Names (`JAN`, `MON`), steps, and lists are accepted, as are `@reboot`, `@yearly`, `@annually`, `@monthly`, `@weekly`, `@daily`, `@midnight`, and `@hourly`. Values that contain a line break are refused. A `%` in the command is stored as `\%` so cron treats it as a literal percent. Paths passed to `--working-dir`, `--output` and `--error-output` are shell-quoted.

### Run a cron job now

`cron run <id>` runs that job immediately as its user, with the job's environment and working directory. The environment also includes a cron-like base: `SHELL=/bin/sh`, `PATH=/usr/bin:/bin`, and `HOME`. Output is streamed and captured. It is not written to the job's log file unless you pass `--use-job-output`, which applies the same redirects cron would (append or overwrite, stdout, stderr, or discard).

`--timeout` defaults to 300 seconds. `0` means no limit. If the job exceeds the limit, Abstrax kills the process group and exits 124. Otherwise the exit code is the job's exit code. A disabled job still runs, and the result says that it is disabled.

```bash
sudo abstrax cron run backup
sudo abstrax cron run backup --timeout 60 --use-job-output
```

### Daemons

`daemon list` and `daemon status` group Supervisor processes under one daemon. `status` is the worst process state (`FATAL` before `BACKOFF`, `EXITED`, `STOPPING`, `STARTING`, `STOPPED`, then `RUNNING`). Each running process includes `pid`, `uptime`, and `uptime_seconds`. `daemon list` shows Abstrax-managed daemons only; add `--all` for every Supervisor program (`managed: false`, no `config_path`).

`daemon info <name>` prints the config Abstrax wrote, plus that live status. `daemon logs --lines N` returns N lines from the log file. When a daemon runs more than one process, `--process N` selects it (default `0`).

`daemon add` and `daemon modify` refuse values that contain a line break. A `%` in a value is stored as `%%` so Supervisor treats it as a literal percent. Environment values are quoted; `"` and `\` are not accepted in those values.

### Daemon modify

`daemon modify` changes only the flags you pass. Other settings stay as they are in the Supervisor config. It accepts the same settings as `daemon add`. `--environment` merges by key. `--clear-environment` drops every variable first, and `--unset-environment KEY` removes one. `--autorestart` must be `always`, `unexpected`, or `false`.

After a modify, Abstrax runs `supervisorctl update`, which restarts the program when its config changed.

### MySQL users

`mysql user list --grants` returns each user's grants in one call, including parsed database privileges. `mysql user info <name>` finds the user on any host, not only `localhost`. Pass `--host` to limit it to one host.

`mysql user password <name>` sets a new password and leaves grants unchanged. The password is generated unless you pass `--password`. With no `--host`, every host the user exists on is updated. `--host localhost` updates both `localhost` and `127.0.0.1`. `mysql user remove` drops both of those local accounts; pass another `--host` to drop only that one.

`mysql grant --privileges` and `mysql user add --privileges` only accept known privilege names. Hosts and database names are checked before they are placed in SQL.

### SSH key restrictions

`ssh-key add --expires 2026-11-07T23:59:00Z` writes `expiry-time` so sshd refuses the key after that UTC time. It needs OpenSSH 8.2 or later. `--from 140.82.112.0/20,143.55.64.0/20` limits the addresses that can use the key. `ssh-key list` and `ssh-key info` show `expires_at`, `from`, and whether the key has expired. Omit both flags and the key line is unchanged.

### MySQL status

`mysql status` reports whether a MySQL-compatible server is installed, which engine (`mysql` or `mariadb`) and version are present, and whether the service is active and enabled. It succeeds when MySQL is missing or stopped. When nothing is installed, `engine` is the server Abstrax would install on this platform.

### MySQL remote access

`mysql remote enable` writes a `zz-abstrax-remote.cnf` drop-in so the database listens beyond loopback (default `0.0.0.0`). Abstrax asks you to confirm, then restarts the service. If the database does not come back, the drop-in is removed and the service is restarted again. The firewall is left unchanged, so allow port 3306 yourself if remote clients should connect. `mysql remote disable` removes that file. `mysql remote status` reports the bind address the running server is using.

### User lock

`user lock` blocks password and SSH key logins by locking the password and expiring the account. Daemons and cron jobs already running as that user keep running. `user lock --password-only` locks the password and leaves key logins working. `user unlock` restores the expiry from before the lock.

`user info` and `user list` show when the account last logged in and the IP address or hostname it came from. Both values are null when that login is unknown. `user list` reads the login log once.

Locking does not end sessions that are already open. `user logout <name> --all` ends those interactive login sessions. It does not stop daemons, cron jobs, or the systemd user manager running as that user.

### Removing a user

`user remove` warns when an Abstrax daemon, an Abstrax cron job, or a running process still uses that account, then removes the user anyway. Daemons and cron jobs are not stopped. `user remove --if-unused` refuses and exits non-zero while anything is still using the account. `--json` includes `still_used_by`, which is an empty array when nothing is.

### SSH port

`ssh config set-port <port> --allow-firewall` opens the new port before reloading sshd. The old port stays open until you pass `--close-old-port`, so you can confirm a new connection first. Without `--allow-firewall` the firewall is left unchanged.

### Root SSH login

`ssh config enable-root-login` writes `PermitRootLogin yes`, which also allows password login when password authentication is on. `ssh config enable-root-login --keys-only` writes `prohibit-password`, so root can log in with an SSH key and not a password. `ssh config disable-root-login` writes `no` and blocks root for every method.

### Project logs

Each project nginx virtual host logs to `/var/log/nginx/<project>-access.log` and `/var/log/nginx/<project>-error.log`. `project info` and `project inspect` show those paths. `project add --shared-logs` and `project modify --shared-logs` keep the server-wide nginx logs instead. That choice is remembered, so a later `project modify` does not switch it unless you pass the flag again. Existing virtual hosts stay as they are until the next `project modify`. `project remove` keeps the log files; `project remove --delete-files` deletes them.

### Project runtime

`project runtime set <name> --php|--node|--ruby|--static` changes a project's runtime. Settings from the old runtime are cleared, the nginx vhost is regenerated, and a user-isolated project's PHP-FPM pool is created or removed to match. If `nginx -t` rejects the new vhost, the old one is restored. `--dry-run` previews the result.

### PHP projects

When multiple PHP versions are installed on a server, use the versioned CLI binary (`php8.5`, `php8.4`, and so on) for Artisan, Composer, cron jobs, and daemons - not the unversioned `php` command. Abstrax routes web requests to the correct PHP-FPM version per project, but CLI commands do not switch automatically. See [Projects](https://useabstrax.com/docs/commands/projects#php-on-the-command-line).

## Documentation

The README covers installation and a quick start. For the full command reference, configuration, guides, and troubleshooting, see the documentation site:

- [Documentation home](https://useabstrax.com/docs)
- [Getting started](https://useabstrax.com/docs/getting-started)
- [Command reference](https://useabstrax.com/docs/commands)
- [Configuration](https://useabstrax.com/docs/configuration)
- [Guides](https://useabstrax.com/docs/guides)

## Contributing

Contributions are welcome. Before opening a pull request:

1. Fork the repository and create a feature branch.
2. Make your change, with tests where appropriate.
3. Ensure `gofmt -l .` is clean.
4. Ensure `go vet ./...` passes.
5. Ensure `go test -race ./...` passes.
6. Open a pull request.

See the [contributing guide](https://useabstrax.com/docs/contributing) for repository layout, architecture, CI, and release details.

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for a history of notable changes.

Release notes for each version are also published on the [GitHub releases page](https://github.com/useabstrax/abstrax/releases).

## Licence

Abstrax is licensed under the [Apache License 2.0](LICENSE). The SPDX identifier is `Apache-2.0`.

The Abstrax name, logo, and branding are not licensed for unrestricted use. You may not use the branding to imply endorsement or to present modified versions as official Abstrax releases.
