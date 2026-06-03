# Abstrax

Abstrax is a server management CLI that abstracts common Linux server administration tasks behind a consistent, friendly command interface.

## What is Abstrax?

Abstrax lets you manage Linux servers without remembering the exact syntax of `useradd`, `ufw`, `supervisorctl`, `certbot`, or nginx configuration files. Every command follows a consistent pattern, produces clean human output by default, and can emit structured JSON for scripting and automation.

```text
CLI command
  -> validates input
  -> builds typed options struct
  -> calls internal service/action layer
  -> service uses platform adapters
  -> returns structured result
  -> CLI prints human output or JSON
```

## Current Status

Abstrax is in active development. The local CLI is the primary focus. A future hosted agent will connect this CLI to a cloud management platform.

| Feature | Status |
|---|---|
| User management | Implemented |
| SSH key management | Implemented |
| SSH server config | Implemented |
| Package management (apt) | Implemented |
| Service management (systemd) | Implemented |
| Cron management | Implemented |
| Daemon management (Supervisor) | Implemented |
| Project management (nginx) | Implemented |
| Web server management | Implemented |
| SSL (Certbot) | Implemented |
| MySQL / MariaDB | Implemented |
| Cache (Redis, Memcached) | Implemented |
| Firewall (UFW) | Implemented |
| Server status | Implemented |
| Apache support | Stub (not yet implemented) |
| Hosted agent | Not yet implemented |

## Supported Platforms

**Fully supported:**
- Ubuntu 20.04+
- Debian 11+

**Planned:**
- RHEL / CentOS / Rocky Linux
- Other Debian derivatives

Abstrax detects unsupported platforms and returns a clear error rather than attempting unsafe operations.

## Installation

### From release binaries

Download the latest binary from the [releases page](https://github.com/useabstrax/abstrax/releases):

```bash
# amd64
curl -Lo abstrax https://github.com/useabstrax/abstrax/releases/latest/download/abstrax_linux_amd64
chmod +x abstrax
sudo mv abstrax /usr/local/bin/abstrax
```

### From .deb package

```bash
# Download and install the .deb package
dpkg -i abstrax_<version>_amd64.deb
```

### Build from source

Requirements: Go 1.22+

```bash
git clone https://github.com/useabstrax/abstrax
cd abstrax/cli
go mod download
go test ./...
go build -o abstrax ./cmd/abstrax
sudo mv abstrax /usr/local/bin/abstrax
abstrax --help
```

### Build with GoReleaser

```bash
goreleaser release --snapshot --clean
```

## Running tests

```bash
go test ./...
go test -v -race ./...
```

## CLI usage

### Global flags

All commands support the following global flags:

```text
--json        Output machine-readable JSON
--dry-run     Show what would happen without making changes
--yes         Skip confirmation prompts for destructive commands
--quiet       Reduce output
--verbose     Increase output verbosity
--no-color    Disable colour output
```

### Root commands

```bash
abstrax --help
abstrax version
abstrax doctor
abstrax doctor --json
```

`abstrax doctor` inspects the current system and reports:

```text
OS, version, architecture
Kernel version
Package manager (apt, dnf, etc.)
Service manager (systemd, etc.)
Firewall backend (ufw, firewalld, etc.)
Whether running as root
Available tools (nginx, certbot, mysql, supervisor, redis, etc.)
```

---

## Commands

### User management

```bash
abstrax user add <name>
abstrax user add <name> --grant-sudo --groups=www-data,deploy --shell=/bin/bash
abstrax user add <name> --create-home --comment="Deploy user"
abstrax user add <name> --system --no-create-home --disabled-password

abstrax user remove <name>
abstrax user remove <name> --delete-home --kill-processes

abstrax user grant-sudo <name>
abstrax user revoke-sudo <name>

abstrax user set-groups <name> www-data,deploy
abstrax user add-groups <name> docker
abstrax user remove-groups <name> docker

abstrax user set-shell <name> /bin/bash

abstrax user lock <name>
abstrax user unlock <name>

abstrax user info <name>
abstrax user list
abstrax user list --sudo
abstrax user list --regular
abstrax user list --system
```

**`user add` flags:**

| Flag | Description |
|---|---|
| `--create-home` | Create home directory (default) |
| `--no-create-home` | Do not create home directory |
| `--grant-sudo` | Add to sudo group |
| `--groups=<groups>` | Comma-separated additional groups |
| `--shell=<shell>` | Login shell |
| `--uid=<uid>` | Custom UID |
| `--system` | Create a system user |
| `--password` | Prompt for password securely |
| `--disabled-password` | Create user without a password |
| `--comment=<comment>` | GECOS comment field |

---

### SSH key management

```bash
abstrax ssh-key add <user> "<key>"
abstrax ssh-key add <user> "<key>" --name=github-deploy --comment="CI key"
abstrax ssh-key add <user> /path/to/key.pub --from-file

abstrax ssh-key remove <user> <key-id>
abstrax ssh-key remove <user> <key-id> --fingerprint=SHA256:xxxx

abstrax ssh-key list <user>
abstrax ssh-key list <user> --managed-only

abstrax ssh-key info <user> <key-id>
```

SSH keys managed by Abstrax include a managed marker comment:

```text
# abstrax:key id=github-deploy name="GitHub deploy key"
ssh-ed25519 AAAAC3... user@example
```

---

### SSH server configuration

```bash
abstrax ssh config show
abstrax ssh config set-port 2222 --allow-firewall
abstrax ssh config set-timeout 300
abstrax ssh config disable-root-login
abstrax ssh config enable-root-login
abstrax ssh config disable-password-auth
abstrax ssh config enable-password-auth

abstrax ssh reload
abstrax ssh restart
```

Abstrax writes SSH configuration to `/etc/ssh/sshd_config.d/99-abstrax.conf` rather than modifying the main config directly. Configuration is validated with `sshd -t` before reloading.

---

### Package management

```bash
abstrax package install <name>
abstrax package install <name> --version=1.2.3
abstrax package remove <name>
abstrax package remove <name> --purge
abstrax package update
abstrax package upgrade
abstrax package upgrade --security-only
abstrax package search <query>
abstrax package info <name>
abstrax package list
```

---

### Service management

```bash
abstrax service start <name>
abstrax service stop <name>
abstrax service restart <name>
abstrax service reload <name>
abstrax service enable <name>
abstrax service disable <name>
abstrax service status <name>
```

---

### Cron management

Abstrax manages cron jobs as files in `/etc/cron.d/abstrax-<id>`.

```bash
abstrax cron add backup --command="/usr/local/bin/backup.sh" --daily --user=root
abstrax cron add report --command="php artisan report" --schedule="0 8 * * 1" --user=www-data
abstrax cron add queue --command="php artisan queue:work" --every-minute

abstrax cron remove <id>
abstrax cron modify <id> --schedule="0 3 * * *"
abstrax cron list
abstrax cron info <id>
abstrax cron enable <id>
abstrax cron disable <id>
```

**Frequency flags:**

```text
--every-minute
--every-five-minutes
--every-ten-minutes
--every-fifteen-minutes
--every-thirty-minutes
--hourly
--daily
--weekly
--monthly
--yearly
--schedule="<cron expression>"
```

---

### Daemon management

Abstrax manages daemons using Supervisor, writing config files to `/etc/supervisor/conf.d/abstrax-<name>.conf`.

```bash
abstrax daemon add queue-worker \
  --command="php artisan queue:work" \
  --directory=/var/www/myapp \
  --user=www-data \
  --processes=2 \
  --autostart \
  --autorestart=unexpected

abstrax daemon add queue-worker --command="..." --install-supervisor

abstrax daemon remove <name> --stop
abstrax daemon modify <name> --processes=4
abstrax daemon start <name>
abstrax daemon stop <name>
abstrax daemon restart <name>
abstrax daemon status <name>
abstrax daemon list
abstrax daemon logs <name>
abstrax daemon logs <name> --lines=100 --follow
```

---

### Project management

Abstrax manages web application projects with nginx virtual hosts. Project state is stored in `/var/lib/abstrax/projects/<name>.json`.

```bash
# Static site
abstrax project add myapp --path=/var/www/myapp --domains=myapp.com,www.myapp.com --static

# PHP application
abstrax project add myapp --path=/var/www/myapp --domains=myapp.com --php --php-version=8.2 --public-dir=public

# Node.js application (proxy)
abstrax project add myapp --path=/var/www/myapp --domains=myapp.com --node --proxy-port=3000

# Ruby application
abstrax project add myapp --path=/var/www/myapp --domains=myapp.com --ruby --proxy-port=3000

abstrax project remove <name> --remove-vhost
abstrax project remove <name> --delete-files --force
abstrax project modify <name> --add-domain=www.myapp.com
abstrax project list
abstrax project info <name>
abstrax project enable <name>
abstrax project disable <name>
abstrax project reload <name>
```

---

### Web server management

```bash
abstrax web test
abstrax web reload
abstrax web restart
```

---

### SSL management

```bash
abstrax ssl add <project> --email=admin@example.com --redirect-http
abstrax ssl add <project> --email=admin@example.com --staging
abstrax ssl remove <project>
abstrax ssl renew
abstrax ssl renew --project=<name>
abstrax ssl status
abstrax ssl status <project>
```

---

### MySQL management

```bash
# Configure connection
abstrax mysql config set --host=127.0.0.1 --user=root --password
abstrax mysql config show

# Test connection
abstrax mysql test

# Install
abstrax mysql install
abstrax mysql install --secure

# Databases
abstrax mysql database add myapp_db
abstrax mysql database add myapp_db --charset=utf8mb4 --if-not-exists
abstrax mysql database remove myapp_db
abstrax mysql database list

# Users
abstrax mysql user add appuser --password --grant-db=myapp_db --preset=app
abstrax mysql user remove appuser
abstrax mysql user list
abstrax mysql user info appuser

# Grants
abstrax mysql grant appuser myapp_db --preset=app
abstrax mysql revoke appuser myapp_db
```

**Privilege presets:**

| Preset | Privileges |
|---|---|
| `readonly` | SELECT |
| `app` | SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP |
| `admin` | ALL PRIVILEGES |

---

### Cache management

```bash
abstrax cache install redis
abstrax cache install memcached
abstrax cache install redis --bind=127.0.0.1 --memory=256mb

abstrax cache remove redis
abstrax cache start redis
abstrax cache stop redis
abstrax cache restart redis
abstrax cache status
abstrax cache status redis
abstrax cache config redis
```

---

### Firewall management

```bash
abstrax firewall status
abstrax firewall enable --allow-ssh
abstrax firewall enable --allow-ssh --ssh-port=2222
abstrax firewall disable

abstrax firewall allow 80
abstrax firewall allow 443/tcp
abstrax firewall deny 23

abstrax firewall allow-ip 192.168.1.0/24
abstrax firewall deny-ip 10.0.0.5

abstrax firewall rule list
abstrax firewall rule remove <id>
```

---

### Server status

```bash
abstrax server status
abstrax server cpu
abstrax server memory
abstrax server disk
abstrax server disk --path=/var/www
abstrax server load
abstrax server services
abstrax server services --failed
```

`server status` output includes: hostname, uptime, load average, CPU cores, memory usage, swap usage, disk usage, OS information, kernel version, and private IP addresses.

---

### Local logs

```bash
abstrax log
abstrax log --follow
abstrax log --lines=100
```

---

## JSON output examples

All commands support `--json` for machine-readable output:

```bash
abstrax doctor --json
```

```json
{
  "status": "success",
  "action": "doctor.check",
  "summary": "System inspection complete.",
  "data": {
    "os": "ubuntu",
    "version": "22.04",
    "architecture": "amd64",
    "package_manager": "apt",
    "service_manager": "systemd",
    "firewall_backend": "ufw",
    "is_root": true,
    "supported": true,
    "tools": {
      "nginx": true,
      "certbot": true,
      "mysql": true
    }
  }
}
```

```bash
abstrax user add mike --grant-sudo --json
```

```json
{
  "status": "success",
  "action": "user.add",
  "summary": "User mike created.",
  "data": {
    "username": "mike",
    "uid": "1001",
    "home": "/home/mike",
    "shell": "/bin/bash",
    "groups": ["mike", "sudo"],
    "sudo": true,
    "created": true
  }
}
```

Error output:

```json
{
  "status": "error",
  "action": "user.add",
  "error_code": "command_error",
  "message": "user mike already exists"
}
```

---

## Dry-run examples

```bash
# Show what would happen without making changes
sudo abstrax user add deploybot --grant-sudo --dry-run
sudo abstrax firewall enable --allow-ssh --dry-run
sudo abstrax package upgrade --dry-run
```

---

## Safety notes

- **Destructive commands** prompt for confirmation unless `--yes` is passed.
- **SSH configuration** changes are validated with `sshd -t` before reloading. Existing configs are backed up with a timestamped suffix (e.g., `sshd_config.abstrax-bak.20240101T120000`).
- **Firewall enable** warns if `--allow-ssh` is not set.
- **File modifications** create backups before writing where practical.
- **MySQL password** is never passed as a command-line flag; use `--password` to be prompted securely.
- **Redis** warns before binding to `0.0.0.0`.
- **Root requirement** – destructive system commands check for root and return a clear error if not running as root.

---

## Packaging notes

### Directory layout installed by packages

```text
/usr/bin/abstrax          – CLI binary
/etc/abstrax/             – configuration (e.g., mysql.toml)
/var/lib/abstrax/         – runtime state (e.g., project JSON files)
/var/log/abstrax/         – logs
/etc/systemd/system/abstrax-agent.service  – future agent service unit
```

### Build release packages locally

```bash
goreleaser release --snapshot --clean
```

Output is in `dist/`.

---

## Future agent

> **This section describes planned functionality that is not yet implemented.**

The Abstrax hosted management platform will not store raw shell commands. Instead it will create structured jobs:

```text
user.add
cron.add
project.add
...
```

The local **Abstrax Agent** will:

1. Connect outbound to the hosted API (no inbound SSH required).
2. Poll for pending jobs assigned to this server.
3. Claim and execute each job locally through the same internal action layer used by the CLI.
4. Report structured results back to the API.

The agent will share all service and validation logic with the CLI. Adding the agent will not require rewriting the core command execution code.

Agent placeholder commands (not yet functional):

```bash
abstrax agent connect
abstrax agent status
abstrax agent run
abstrax agent update
```

These commands currently print:

```
Agent mode is not yet implemented.
```

---

## Contributing

1. Fork the repository.
2. Create a feature branch.
3. Ensure `go test ./...` and `go vet ./...` pass.
4. Ensure `gofmt` formatting is clean.
5. Open a pull request.

## License

MIT
