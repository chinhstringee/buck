# buck

Multi-repo orchestration tool for Bitbucket Cloud. Create branches and pull requests across multiple repositories simultaneously.

## Features

- **Branch creation** — Create the same branch across many repos in parallel
- **Pull requests** — Open PRs across repos, or auto-detect from git context
- **Repository groups** — Define named groups in config for quick targeting
- **Fuzzy matching** — Target repos by partial name (`--repos "api,web"`)
- **Interactive selection** — TUI multi-select when no flags given
- **Dry run** — Preview actions without executing
- **Auth flexibility** — API token (default) or OAuth 2.0 with PKCE
- **Shell completion** — Tab completion for bash, zsh, fish, and powershell

## Install

**Homebrew** (macOS / Linux):

```bash
brew tap chinhstringee/tap
brew install buck
```

**Go** (requires Go 1.25+):

```bash
go install github.com/chinhstringee/buck@latest
```

**From source**:

```bash
git clone https://github.com/chinhstringee/buck.git
cd buck && go build -o buck
```

## Quick Start

```bash
# 1. Configure credentials
buck setup                    # interactive API token setup

# 2. List repos in your workspace
buck list

# 3. Create a branch across repos
buck create feature/auth --group backend

# 4. Create PRs from that branch
buck pr feature/auth --group backend

# Or auto-detect from current git context
buck pr
```

## Usage

```bash
# Branches
buck create <branch> --repos repo-a,repo-b --from main
buck create <branch> --group backend
buck create <branch> --dry-run

# Pull requests
buck pr                       # auto-detect branch and repo from CWD
buck pr <branch> --repos repo-a,repo-b
buck pr <branch> --group backend --destination develop
buck pr <branch> --repos repo-a --title "Add login feature"
buck pr <branch> --dry-run

# Other
buck list                     # list workspace repos
buck login                    # OAuth browser flow
buck setup                    # interactive API token setup
buck completion zsh           # generate shell completion script
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--repos` | `-r` | Comma-separated patterns (fuzzy match) |
| `--group` | `-g` | Use a predefined repo group from config |
| `--from` | `-f` | Source branch (overrides config default) |
| `--destination` | `-d` | PR destination branch (default: master) |
| `--title` | `-t` | PR title (default: derived from branch name) |
| `--dry-run` | | Preview without executing |
| `--interactive` | `-i` | Force interactive selection |
| `--config` | | Custom config file path |

## Configuration

Config file: `.buck.yaml` (searched in CWD, then home dir).

```bash
cp .buck.example.yaml .buck.yaml
```

```yaml
workspace: my-workspace

# API token auth (default, no login needed)
api_token:
  email: ${BITBUCKET_EMAIL}
  token: ${BITBUCKET_API_TOKEN}

# Or OAuth 2.0 (requires 'buck login')
# auth:
#   method: oauth
# oauth:
#   client_id: ${BITBUCKET_OAUTH_CLIENT_ID}
#   client_secret: ${BITBUCKET_OAUTH_CLIENT_SECRET}

groups:
  backend:
    - repo-api
    - repo-worker
  frontend:
    - repo-web
    - repo-mobile

defaults:
  source_branch: master
```

All credential fields support `${ENV_VAR}` expansion.

## Shell Completion

Enable tab completion for commands, flags, and dynamic values (repo names, groups).

**Bash:**

```bash
source <(buck completion bash)

# Persist (Linux):
buck completion bash > /etc/bash_completion.d/buck

# Persist (macOS):
buck completion bash > $(brew --prefix)/etc/bash_completion.d/buck
```

**Zsh:**

```bash
buck completion zsh > "${fpath[1]}/_buck"
exec zsh
```

**Fish:**

```bash
buck completion fish | source

# Persist:
buck completion fish > ~/.config/fish/completions/buck.fish
```

**PowerShell:**

```powershell
buck completion powershell | Out-String | Invoke-Expression
```

Completions include: commands, `--group` (group names), `--repos` (repo slugs), `--from`/`--destination` (common branches), `--strategy` (merge strategies), `--state` (PR states).

## License

MIT
