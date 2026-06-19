# Changelog

All notable changes to Abstrax are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Default PHP extensions now include `intl`, `redis`, and `sqlite3` for Laravel-oriented installations. `pcntl` and `posix` are provided by `php*-cli` and are no longer installed as separate packages.

### Added

- Plugin system with standalone executable binaries, registry-backed install/update/remove, command delegation, metadata protocol v1, and `abstrax plugin` management commands.
- Machine-readable `abstrax project inspect --json` API (v1) for plugins.
- `abstrax project service restart|reload` for project-owned supervisor services.
- Reference plugin at `cli/cmd/abstrax-example`.
- Initial open-source release of the Abstrax CLI.
