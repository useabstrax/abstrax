# Abstrax Development Guide

## Cursor Cloud specific instructions

Abstrax is a single-binary Go CLI tool for Linux server administration. There is no web frontend, no database, and no services to start - development consists entirely of building and testing a Go binary.

### Quick reference

| Task | Command |
|---|---|
| Install deps | `go mod download` |
| Lint (format) | `gofmt -l .` (no output = clean) |
| Lint (vet) | `go vet ./...` |
| Test | `go test -v -race ./...` |
| Build | `go build -o abstrax ./cmd/abstrax` |
| Run | `./abstrax --help`, `./abstrax doctor` |

### Gotchas

- Most CLI commands (user, firewall, package, service, etc.) require **root** - run them with `sudo ./abstrax ...`.
- Use `--dry-run` to preview what a command would do without making changes (safe for testing without root for some commands, but many still require root to read system state).
- The `abstrax doctor` command works without root and is a good smoke test after building.
- The built binary is placed in the repo root as `./abstrax` - it is `.gitignore`d.
- CI runs `gofmt -l .`, `go vet ./...`, `go test -v -race ./...`, and `go build` - see `.github/workflows/test.yml`.
