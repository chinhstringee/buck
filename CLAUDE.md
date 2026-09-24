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

# Run all tests (126 tests across 11 packages)
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
buck pr <branch-name> --repos repo-a --body-file description.md   # override commit-derived description
buck pr <branch-name> --dry-run

# PR management subcommands
buck pr view <branch> --repos repo-a               # inspect one PR (read-only)
buck pr view --id 42 --repos repo-a --state MERGED
buck pr merge <branch> --repos repo-a --strategy squash --yes
buck pr decline <branch> --group backend --yes
buck pr approve <branch> --repos repo-a,repo-b
buck pr reviewers <branch> --add "{uuid1},{uuid2}" --repos repo-a
buck pr list --state OPEN --mine --group backend

# PR status dashboard
buck status                # auto-detect from CWD
buck status --group backend
buck status --mine
buck status --author alice

# Branch status across repos (read-only: head, containment, PR state)
buck branch-status <branch> --repos repo-a,repo-b
buck branch-status <branch> --group backend --against release,master

# Branch cleanup
buck clean <branch> --repos repo-a,repo-b --yes
buck clean --merged --group backend --dry-run

# Common flags for create, pr, status, clean
#   -r, --repos       comma-separated repo slugs (supports fuzzy match)
#   -g, --group       repo group from .buck.yaml config
#   -i, --interactive  force interactive repo selection
#       --dry-run      preview without executing

# Other commands
buck list          # List workspace repos
buck login         # OAuth login flow
buck setup         # Interactive API token setup
buck completion bash|zsh|fish|powershell  # Generate shell completion script
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
  ├── pr.go           PR parent command (backward compat: `buck pr <branch>` = create)
  ├── pr_helpers.go   Shared PR subcommand context resolution + --body/--body-file reading
  ├── pr_view.go      View a single PR's details (read-only; by branch or --id)
  ├── pr_merge.go     Merge PRs by branch name across repos
  ├── pr_decline.go   Decline PRs by branch name across repos
  ├── pr_approve.go   Approve PRs by branch name across repos
  ├── pr_reviewers.go Add reviewers to PRs across repos
  ├── pr_list.go      List PRs across repos with filters
  ├── status.go       PR status dashboard across repos
  ├── branch_status.go Per-branch status across repos: head, containment, PR state (read-only)
  ├── clean.go        Branch cleanup (single or --merged)
  ├── setup.go        Interactive API token configuration
  └── completion.go   Shell completion generation + dynamic completers
  │
  internal/     (Private packages)
  ├── auth/         OAuth 2.0 + PKCE flow, token persistence (~/.buck/token.json)
  ├── bitbucket/    REST API client + types + AuthApplier (api.bitbucket.org/2.0)
  ├── cleanup/      Parallel branch deletion orchestrator with protected branches
  │                 (default: main, master, develop, staging, production, release)
  ├── config/       YAML config loading with env var expansion (${VAR_NAME})
  ├── creator/      Parallel branch creation orchestrator (goroutines + sync)
  ├── dashboard/    Concurrent PR fetcher + colored table display
  ├── gitutil/      Git context detection (current branch, Bitbucket remote parsing)
  ├── matcher/      Fuzzy repo slug matching
  └── pullrequest/  PR creation + management orchestrators (goroutines + sync)
```

**Key data flow for `create` command**: Config loading → Token retrieval (auto-refresh) → Repo resolution (flags/groups/interactive) → Concurrent branch creation → Colored result display.

**Key data flow for `pr` command**: Config loading → Token retrieval → Repo resolution → Per-repo: ListCommits (description, unless `--body`/`--body-file` overrides it) + CreatePullRequest → Colored result display with PR URLs.

**Key data flow for `pr view`**: Config/auth → Repo resolution → `--id` set: GetPullRequest (single repo); otherwise Per-repo: FindPRsByBranch (client-side state filter — Bitbucket ignores `state` when combined with `q`) → single match prints detail, multiple matches list ids and ask for `--id`.

**Key data flow for `pr merge/decline/approve`**: Config/auth → Repo resolution → Per-repo: FindPRByBranch → Action (merge/decline/approve) → Colored result display.

**Key data flow for `status` command**: Config/auth → Repo resolution → Per-repo: ListPullRequests → Filter (--mine/--author) → Colored dashboard table.

**Key data flow for `branch-status` command**: Config/auth → Repo resolution → Per-repo (concurrent): GetBranch (head; missing ⇒ "no branch", skips the rest for that repo) → per `--against` target: CommitsAhead(include=branch, exclude=target, pagelen=1) (empty ⇒ contained; missing target reported per-target) → FindAllPRsByBranch (single unfiltered call; Bitbucket returns every state) → Colored per-repo report. Read-only (GET only), no build-status column (2026-09-24 decision).

**Key data flow for `clean` command**: Config/auth → Repo resolution → Per-repo: DeleteBranch (or ListMergedPRBranches → DeleteBranch for --merged) → Colored result display.

**Repo resolution order**: `--interactive` flag > `--repos` flag > `--group` flag > interactive multi-select (charmbracelet/huh).

## Config

Config file: `.buck.yaml` (searched in cwd, then home dir). Real config is gitignored; `.buck.example.yaml` is the template. Supports `${ENV_VAR}` expansion for credential fields.

Auth methods: `api_token` (default, Basic auth) or `oauth` (Bearer token). OAuth token stored at `~/.buck/token.json` with 0600 permissions.

## Testing Patterns

- `httptest.Server` for Bitbucket API mocking
- `t.TempDir()` for file system isolation
- `t.Setenv()` for env var isolation
- Mock `AuthApplier func(req *http.Request) error` for auth injection
- `hostRewriteTransport` redirects real API URLs to test server (for orchestrator tests)
- Creator/dashboard/cleanup tests verify concurrency safety with stress tests (15-20 repos)

## Dependencies

- `spf13/cobra` — CLI framework
- `spf13/viper` — Config management
- `charmbracelet/huh` — Interactive TUI forms
- `fatih/color` — Colored terminal output
