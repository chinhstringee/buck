# buck Usage Guide

Create git branches across multiple Bitbucket Cloud repositories simultaneously.

## Quick Start

### 1. Install Prerequisites

- **Go 1.25 or later** — [Download Go](https://golang.org/dl/)
- **Bitbucket Cloud workspace** — [Create one](https://bitbucket.org)

### 2. Build and Configure

```bash
go build -o buck
cp .buck.example.yaml .buck.yaml
```

### 3. Authentication Setup

#### Option A: API Token (default, recommended)

1. Go to [Bitbucket > Personal settings > Security > API tokens](https://bitbucket.org/account/settings/api-tokens/)
2. Click "Create API token with scopes"
3. Select scopes: `read:repository:bitbucket`, `write:repository:bitbucket`
4. Copy the token (shown only once)

```yaml
workspace: your-workspace-slug

api_token:
  email: your-email@example.com
  token: YOUR_API_TOKEN
```

No `buck login` needed — works immediately.

#### Option B: OAuth 2.0 + PKCE

1. Go to Bitbucket workspace → Settings → API → OAuth consumers
2. Click "Add consumer"
3. Configure:
   - **Name**: `buck`
   - **Callback URL**: `http://localhost:9876/callback`
   - **Permissions**: Repositories (Read + Write)
4. Save client ID and secret

```yaml
workspace: your-workspace-slug

auth:
  method: oauth

oauth:
  client_id: YOUR_CLIENT_ID
  client_secret: YOUR_CLIENT_SECRET
```

Then run `buck login` to authenticate via browser.

### 4. Add Groups (optional)

```yaml
groups:
  backend:
    - api-repo
    - worker-repo
  frontend:
    - web-repo

defaults:
  source_branch: master
  branch_prefix: "feature/"
```

## Commands

### `buck login`

Authenticate with Bitbucket via OAuth 2.0 browser flow. Only needed when using `auth.method: oauth`.

```bash
buck login
```

**What it does:**
- Opens browser for OAuth authorization
- Stores token in `~/.buck/token.json`
- Token reused for all subsequent commands

**Note**: Not needed for API token auth. Run when token expires or you need to switch accounts.

---

### `buck list`

List all repositories in your workspace with metadata.

```bash
buck list
```

**Output example:**

```
Fetching repos from workspace "my-workspace"...

REPO                           DEFAULT BRANCH     UPDATED
─────────────────────────────────────────────────────────────
api-repo                       main               2025-02-20
web-repo                       master             2025-02-18
worker-repo                    main               2025-02-15
mobile-repo                    develop            2025-02-10

Total: 4 repositories
```

**Use cases:**
- Verify workspace access
- Find exact repo slugs for `--repos` flag
- Check default branches

---

### `buck create <branch-name>`

Create a branch across selected repositories.

```bash
buck create feature/new-feature
```

**By default**, prompts interactive multi-select of repos. Navigate with arrow keys, toggle with space, confirm with enter.

#### Options

| Flag | Short | Description |
|------|-------|-------------|
| `--group` | `-g` | Use predefined repo group from config |
| `--repos` | `-r` | Comma-separated repo slugs |
| `--from` | `-f` | Source branch name or full commit hash (overrides config default) |
| `--dry-run` | | Preview without executing |
| `--interactive` | `-i` | Force interactive selection |
| `--config` | | Custom config file path |

#### Examples

**Interactive mode (default):**

```bash
buck create release/v1.2.3
```

**Using a group from config:**

```bash
buck create feature/auth --group backend
```

Groups must be defined in `.buck.yaml`:

```yaml
groups:
  backend:
    - api-repo
    - worker-repo
```

**Specific repositories:**

```bash
buck create bugfix/cors --repos api-repo,web-repo
```

**From non-default branch:**

```bash
buck create release/v2.0 --from develop
```

**From a specific commit (full SHA):**

```bash
buck create test/hotfix-check --from 4c0ae8c66faf786f59aa7b5b2ef567c43d890277 --repos api-repo
```

`--from` is passed as-is to Bitbucket's branch target, which resolves both
branch names and full commit hashes. Verified live 2026-09-24.

**Preview without creating:**

```bash
buck create feature/test --dry-run
```

Output:

```
Dry run: would create branch "feature/test" from "master" in:
  - api-repo
  - web-repo
```

**Custom config file:**

```bash
buck create feature/x --config /path/to/.buck.yaml
```

---

### `buck pr [branch-name]`

Create pull requests from a branch to `master` (or a custom destination). Branch name is optional — when omitted, auto-detects from git context.

**Auto-detection mode** (no arguments):
```bash
buck pr
```
Auto-detects:
- Current branch from git HEAD
- Repository from `origin` remote URL (Bitbucket format)
- Creates PR in that repo only, without prompts

**With branch name** (explicit mode):
```bash
buck pr feature/auth
```
**By default**, prompts interactive multi-select of repos. Navigate with arrow keys, toggle with space, confirm with enter.

#### Options

| Flag | Short | Description |
|------|-------|-------------|
| `--group` | `-g` | Use predefined repo group from config |
| `--repos` | `-r` | Comma-separated repo slugs |
| `--source` | `-s` | Source branch (defaults to target branch name) |
| `--destination` | `-d` | Destination branch (defaults to `master`) |
| `--body` | `-b` | PR description text (default: derived from commits) |
| `--body-file` | `-F` | Read PR description from a file; use `-` for standard input. Mutually exclusive with `--body` |
| `--dry-run` | | Preview without creating |
| `--interactive` | `-i` | Force interactive selection |
| `--config` | | Custom config file path |

#### Examples

**Auto-detect from git context (fastest):**

```bash
# Current branch is "feature/auth", origin points to api-repo
buck pr
```

Creates a single PR from current branch to `master` in the detected repo. No interactive selection.

**Interactive mode (explicit branch):**

```bash
buck pr feature/auth
```

Creates PRs from `feature/auth` to `master` in selected repos. Prompts interactive multi-select.

**Using a group from config:**

```bash
buck pr feature/auth --group backend
```

**Specific repositories:**

```bash
buck pr feature/cors --repos api-repo,web-repo
```

**Custom destination branch:**

```bash
buck pr feature/hotfix --destination develop --repos api-repo,worker-repo
```

**Preview without creating:**

```bash
buck pr feature/test --dry-run
```

Output:

```
Dry run: would create PRs from "feature/test" to "master" in:
  - api-repo
  - web-repo
  - worker-repo
```

**Force interactive selection:**

```bash
buck pr feature/auth --interactive
```

**Custom description (overrides the commit-derived default):**

```bash
buck pr feature/auth --repos api-repo --body "Reviewed summary of the change"
buck pr feature/auth --repos api-repo --body-file description.md
buck pr feature/auth --repos api-repo --body-file - < description.md
```

`--dry-run` prints the description source (`--body`/`--body-file`) and its byte
length instead of making a network call.

---

### `buck pr view [branch-name]`

Show one pull request's full details — state, source/destination, source head
commit, author, reviewers with approval, timestamps, URL, and description.
Read-only (GET only); replaces ad-hoc REST reads for inspecting a single PR.

Resolve by branch (default state `OPEN`) or by `--id` (requires exactly one
resolved repository). If several PRs in a repo match the branch and state,
`pr view` lists their ids instead of guessing and asks you to rerun with `--id`.

#### Options

| Flag | Short | Description |
|------|-------|-------------|
| `--id` | | Pull request id; requires exactly one resolved repository |
| `--state` | | Filter when resolving by branch: `OPEN` (default), `MERGED`, `DECLINED`, `SUPERSEDED` |
| `--full` | | Show the untruncated source commit hash (default: short, 7 chars) |
| `--repos` | `-r` | Comma-separated repo slugs |
| `--group` | `-g` | Use predefined repo group from config |
| `--interactive` | `-i` | Force interactive selection |

```bash
buck pr view feature/auth --repos api-repo
buck pr view --id 42 --repos api-repo
buck pr view feature/auth --repos api-repo --state MERGED --full
```

---

### `buck pr edit [<number> | <branch>]`

Edit the title or body of one pull request. When the selector is omitted, Buck finds the open pull request for the current branch and repository.

Exactly one repository must be selected.

#### Options

| Flag | Short | Description |
|------|-------|-------------|
| `--title` | `-t` | Set the new title |
| `--body` | `-b` | Set the new body |
| `--body-file` | `-F` | Read the body from a file; use `-` for standard input |
| `--repos` | `-r` | Select one repository |
| `--dry-run` | | Preview the target and edited fields |

`--body` and `--body-file` cannot be used together.

```bash
buck pr edit 23 --repos api-repo --title "Updated title"
buck pr edit 23 --repos api-repo --body-file description.md
buck pr edit 23 --repos api-repo --body-file - < description.md
buck pr edit --body "Updated from the current branch"
```

---

## Configuration

### File Locations

buck looks for `.buck.yaml` in this order:

1. Current directory
2. User home directory (`~/.buck.yaml`)
3. Custom path via `--config` flag

### Schema

```yaml
workspace: my-workspace              # Required: Bitbucket workspace slug

# Auth method: "api_token" (default) or "oauth"
auth:
  method: api_token                  # Optional: defaults to api_token

# For API token auth
api_token:
  email: user@example.com            # Atlassian account email
  token: YOUR_API_TOKEN              # API token with repo scopes

# For OAuth auth
oauth:
  client_id: YOUR_CLIENT_ID
  client_secret: YOUR_CLIENT_SECRET

groups:                               # Optional: Named repo groups
  backend:
    - api-repo
    - worker-repo

defaults:
  source_branch: master               # Optional: Default source branch
  branch_prefix: "feature/"           # Optional: Not used by create command
```

### Environment Variables

All credential fields support `${ENV_VAR}` expansion:

```bash
export BITBUCKET_EMAIL=user@example.com
export BITBUCKET_API_TOKEN=your-token
buck list
```

---

## Common Workflows

### Workflow 1: Create feature branch across backend services

```bash
# One-time: add group to config
# Then run:
buck create feature/auth --group backend

# Or interactively:
buck create feature/auth
```

### Workflow 2: Create release branch from develop

```bash
buck create release/v1.5.0 --from develop --repos api-repo,web-repo
```

### Workflow 3: Preview before creating

```bash
# Preview
buck create feature/risky-change --dry-run

# If satisfied:
buck create feature/risky-change
```

### Workflow 4: Auto-create PR from current branch

```bash
# After working on a branch locally
git checkout feature/auth
buck pr  # Auto-detects branch and repo, creates PR instantly
```

No config needed — just works if origin points to a Bitbucket repo.

### Workflow 5: Create pull requests across repos

```bash
# Create branches first
buck create feature/auth --group backend

# Then create PRs from those branches
buck pr feature/auth --group backend
```

Or in one group:

```bash
buck pr feature/auth --repos api-repo,web-repo,worker-repo
```

### Workflow 6: Create PR with custom destination

```bash
# PR from feature branch to develop (not master)
buck pr feature/newfeature --destination develop --repos api-repo,web-repo
```

### Workflow 7: Verify workspace setup

```bash
buck list
```

---

## Troubleshooting

### "Workspace not configured"

**Problem**: Error when running commands.

**Solution**: Add `workspace:` to `.buck.yaml`:

```yaml
workspace: your-workspace-slug
```

Find your workspace slug in Bitbucket URL: `bitbucket.org/{workspace-slug}`

---

### "api_token credentials not configured"

**Problem**: Commands fail with credentials error.

**Solution**: Set API token in `.buck.yaml`:

```yaml
api_token:
  email: your-email@example.com
  token: YOUR_API_TOKEN
```

Create an API token at: Bitbucket > Personal settings > Security > API tokens.
Required scopes: `read:repository:bitbucket`, `write:repository:bitbucket`.

---

### "No repositories found"

**Problem**: `buck list` returns empty or create fails.

**Solutions**:
1. Verify workspace slug: `buck list`
2. Check API token scopes: needs `read:repository:bitbucket` + `write:repository:bitbucket`
3. If using OAuth: re-authenticate with `buck login`

---

### "Selection cancelled"

**Problem**: Interactive select closed without choosing repos.

**Solutions**:
- Try again and use arrow keys + space + enter
- Use `--repos` flag instead: `buck create feature/x --repos api-repo,web-repo`
- Use `--group` flag: `buck create feature/x --group backend`

---

### "Port 9876 in use"

**Problem**: OAuth login fails because callback port is taken.

**Solution**: Kill process using port 9876 or restart your system.

---

## Global Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `.buck.yaml` in current dir or home) |
| `--help` | Show command help |
| `--version` | Show tool version |

---

## Security Notes

- **Token storage**: `~/.buck/token.json` (OAuth only) is readable by your user account only
- **Credentials**: Never commit `.buck.yaml` with real credentials to git
- **Environment**: Use `${ENV_VAR}` expansion or environment variables in CI/CD pipelines
- **API token scope**: Use minimal scopes — only `read:repository` + `write:repository`

---

## Examples Reference

| Task | Command |
|------|---------|
| Authenticate | `buck login` |
| List repos | `buck list` |
| Create (interactive) | `buck create feature/auth` |
| Create (group) | `buck create feature/auth --group backend` |
| Create (specific repos) | `buck create feature/auth --repos api-repo,web-repo` |
| Create from different branch | `buck create release/v1 --from develop` |
| Create (preview) | `buck create feature/test --dry-run` |
| PR (auto-detect) | `buck pr` |
| PR (interactive) | `buck pr feature/auth` |
| PR (group) | `buck pr feature/auth --group backend` |
| PR (specific repos) | `buck pr feature/auth --repos api-repo,web-repo` |
| PR (custom destination) | `buck pr feature/auth --destination develop` |
| PR (preview) | `buck pr feature/test --dry-run` |
| Custom config | `buck list --config ~/.buck-prod.yaml` |
