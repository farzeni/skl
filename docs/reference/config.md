# Configuration

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Each machine can have its own config file, `~/.skl/config.toml`. Create and edit it by hand when you want to change a default; without it, skl uses the defaults below. It's never synced, so settings that differ between machines go here. Settings that should follow you everywhere, like skill sets, live in your vault.

Your vault itself needs no setting: it's the git clone at `~/.skl/vault`, and its remote is that clone's `origin`. `skl init` sets both up.

```toml
default_host = "github.com"
protocol     = "ssh"
scope        = "user"
claude       = "claude"
license      = "MIT"
```

| Key | Default | Effect |
|---|---|---|
| `default_host` | `github.com` | The host that `owner/repo` refers to. Set it to your company's GitLab, for example. |
| `protocol` | from `gh`, else `https` | `ssh` or `https`, for repos written as `owner/repo` or `host/path`. Without a setting, skl uses the protocol the `gh` CLI is configured with for that host, or `https` if `gh` isn't installed. |
| `scope` | `user` | The default for `--scope`. |
| `claude` | `claude` | The Claude Code binary that a [lesson](../guide/learn#how-a-lesson-works) runs. |
| `license` | `MIT` | The license `skl init` writes into a new vault: an SPDX id such as `Apache-2.0`, or `none`. An existing vault's `LICENSE` file is never changed. See [Licenses](../guide/learn#licenses). |

## Your own git servers

[`skl open`](./repos#web-pages) knows the web pages of GitHub, GitLab, Bitbucket, Gitea and Forgejo (such as Codeberg), sourcehut and Azure Repos. For a server of your own, say what it runs:

```toml
[web]
"git.example.com" = "gitlab"
```

The value is one of `github`, `gitlab`, `bitbucket`, `gitea` (also Forgejo), `sourcehut` or `azure`. A server without an entry gets its repo's front page.

## Environment variables

Every key above the `[web]` table can be set with an environment variable: `SKL_DEFAULT_HOST`, `SKL_PROTOCOL`, `SKL_SCOPE`, `SKL_CLAUDE`, `SKL_LICENSE`.

skl also reads:

| Variable | Effect |
|---|---|
| `SKL_HOME` | Moves the whole `~/.skl` folder, config file included. Useful in CI or for keeping separate setups. |
| `SKL_DEBUG` | Set to `1` to print each git and `claude` command skl runs, on standard error. |
| `NO_COLOR` | Set to anything to turn color off. `TERM=dumb` does the same. See [Output](./commands#output). |
| `BROWSER` | The browser [`skl open`](./repos#web-pages) starts, instead of the system's default. |

## Precedence

A command-line flag wins over an environment variable, which wins over the config file, which wins over the default.
