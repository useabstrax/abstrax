<p align="center">
  <img src="docs/abstrax-icon.svg" alt="Abstrax" width="80" />
</p>

<h1 align="center">Abstrax</h1>

<p align="center">
  A server management CLI for Linux.
</p>

<p align="center">
  <a href="https://github.com/useabstrax/abstrax/blob/main/LICENSE">MIT License</a> ·
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

Every command validates its input, performs the requested change, and prints a clear result. Add `--json` for machine-readable output, `--dry-run` to preview changes, or `--yes` to skip confirmation prompts. See the [documentation](https://useabstrax.com/docs) for all global flags.

Abstrax is in active development. A hosted management platform and local agent are planned but not yet available.

## Supported platforms

Abstrax runs on **Linux only**. Release builds are published for:

| Architecture | Notes |
|---|---|
| `linux/amd64` | x86_64 servers |
| `linux/arm64` | ARM / aarch64 servers |

**Fully supported distributions** (Debian/Ubuntu based):

- Ubuntu 20.04+
- Debian 11+
- Linux Mint
- Pop!\_OS
- Raspbian

**Planned:** RHEL / CentOS / Rocky Linux and other Debian derivatives.

Abstrax detects unsupported platforms and returns a clear error rather than attempting unsafe operations. Run `abstrax doctor` to see what was detected on your server. See [supported platforms](https://useabstrax.com/docs/reference/supported-platforms) for details.

## Installation

Full installation instructions, checksum verification, and package options are in the [getting started guide](https://useabstrax.com/docs/getting-started).

### From a release binary

Download the latest binary from the [releases page](https://github.com/useabstrax/abstrax/releases):

```bash
curl -Lo abstrax https://github.com/useabstrax/abstrax/releases/latest/download/abstrax_linux_amd64
chmod +x abstrax
sudo mv abstrax /usr/local/bin/abstrax
```

Replace `abstrax_linux_amd64` with `abstrax_linux_arm64` on ARM servers.

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

`abstrax doctor` does not require root and makes no changes — it is a safe first command. Most commands that change system state require `sudo`.

Explore command groups with `abstrax <group> --help`, for example `abstrax user --help` or `abstrax firewall --help`.

## Documentation

The README covers installation and a quick start. For the full command reference, configuration, guides, and troubleshooting, see the documentation site:

- [Documentation home](https://useabstrax.com/docs)
- [Getting started](https://useabstrax.com/docs/getting-started)
- [Command reference](https://useabstrax.com/docs/commands)
- [Configuration](https://useabstrax.com/docs/configuration)
- [Guides](https://useabstrax.com/docs/guides/creating-a-user)

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

## License

Abstrax is released under the [MIT License](https://github.com/useabstrax/abstrax/blob/main/LICENSE).
