# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**buck** — Multi-repo orchestration tool for Bitbucket Cloud. Create branches and pull requests across multiple repositories simultaneously. Supports API token (default) and OAuth 2.0 with PKCE authentication.

- **Module**: `github.com/chinhstringee/buck`
- **Go version**: 1.25.0

## Commands

```bash
# Build
go build -o buck

# Run all tests (101 tests across 9 packages)
go test ./...

# Run single test
go test -run TestFunctionName ./internal/auth/

# Run with verbose output
go test -v ./...

# Run directly without building
go run main.go <subcommand>
```

No Makefile or linter configuration exists. Standard `go vet` and `gofmt` apply.

## CLI Usage

```bash
# Create branches across repos
buck create <branch-name> --repos repo-a,repo-b --from main
buck create <branch-name> --group backend
buck create <branch-name> --dry-run

# Create pull requests across repos
buck pr                    # auto-detect branch and repo from git context
buck pr <branch-name> --repos repo-a,repo-b
buck pr <branch-name> --group backend --destination develop
buck pr <branch-name> --dry-run

# Common flags for both create and pr
#   -r, --repos       comma-separated repo slugs (supports fuzzy match)
#   -g, --group       repo group from .buck.yaml config
#   -i, --interactive  force interactive repo selection
#       --dry-run      preview without executing

# Other commands
buck list          # List workspace repos
buck login         # OAuth login flow
buck setup         # Interactive API token setup
```

## Release

Tag-based via GoReleaser. To release a new version:
```bash
git tag v0.X.0 && git push origin v0.X.0
# GitHub Actions runs GoReleaser → builds binaries + updates Homebrew tap
# Users upgrade: brew upgrade buck
```

## Architecture

```
main.go → cmd.Execute()
  │
  cmd/          (Cobra CLI commands)
  ├── root.go         Viper config init (.buck.yaml)
  ├── auth_helper.go  Builds AuthApplier from config (API token or OAuth)
  ├── resolve.go      Shared repo resolution (--repos/--group/interactive)
  ├── login.go        OAuth login flow
  ├── list.go         List workspace repos
  ├── create.go       Create branches across repos
  ├── pr.go           Create pull requests across repos
  └── setup.go        Interactive API token configuration
  │
  internal/     (Private packages)
  ├── auth/         OAuth 2.0 + PKCE flow, token persistence (~/.buck/token.json)
  ├── bitbucket/    REST API client + types + AuthApplier (api.bitbucket.org/2.0)
  ├── config/       YAML config loading with env var expansion (${VAR_NAME})
  ├── creator/      Parallel branch creation orchestrator (goroutines + sync)
  ├── gitutil/      Git context detection (current branch, Bitbucket remote parsing)
  ├── matcher/      Fuzzy repo slug matching
  └── pullrequest/  Parallel PR creation orchestrator (goroutines + sync)
```

**Key data flow for `create` command**: Config loading → Token retrieval (auto-refresh) → Repo resolution (flags/groups/interactive) → Concurrent branch creation → Colored result display.

**Key data flow for `pr` command**: Config loading → Token retrieval → Repo resolution → Per-repo: ListCommits (description) + CreatePullRequest → Colored result display with PR URLs.

**Repo resolution order**: `--interactive` flag > `--repos` flag > `--group` flag > interactive multi-select (charmbracelet/huh).

## Config

Config file: `.buck.yaml` (searched in cwd, then home dir). Real config is gitignored; `.buck.example.yaml` is the template. Supports `${ENV_VAR}` expansion for credential fields.

Auth methods: `api_token` (default, Basic auth) or `oauth` (Bearer token). OAuth token stored at `~/.buck/token.json` with 0600 permissions.

## Testing Patterns

- `httptest.Server` for Bitbucket API mocking
- `t.TempDir()` for file system isolation
- `t.Setenv()` for env var isolation
- Mock `AuthApplier func(req *http.Request) error` for auth injection
- Creator tests verify concurrency safety with stress tests (20 repos)

## Dependencies

- `spf13/cobra` — CLI framework
- `spf13/viper` — Config management
- `charmbracelet/huh` — Interactive TUI forms
- `fatih/color` — Colored terminal output
