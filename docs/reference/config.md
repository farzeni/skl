# Configuration

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Each machine has its own config file, `~/.skl/config.toml`. `skl init` creates it; after that, edit it by hand. It's never synced, so settings that differ between machines go here. Settings that should follow you everywhere, like skill sets, live in your vault.

```toml
vault        = "git@github.com:you/skills.git"
default_host = "github.com"
protocol     = "ssh"
scope        = "user"
claude       = "claude"
```

| Key | Default | Effect |
|---|---|---|
| `vault` | set by `skl init` | Your vault's remote. |
| `default_host` | `github.com` | The host that `owner/repo` refers to. Set it to your company's GitLab, for example. |
| `protocol` | from `gh`, else `https` | `ssh` or `https`, for repos written as `owner/repo` or `host/path`. Without a setting, skl uses the protocol the `gh` CLI is configured with for that host, or `https` if `gh` isn't installed. |
| `scope` | `user` | The default for `--scope`. |
| `claude` | `claude` | The Claude Code binary that a [lesson](../guide/learn#how-a-lesson-works) runs. |

## Your own git servers

[`skl open`](./repos#web-pages) knows the web pages of GitHub, GitLab, Bitbucket, Codeberg, sourcehut and Azure Repos. For a server of your own, say what it runs:

```toml
[web]
"git.example.com" = "gitlab"
```

The value is one of `github`, `gitlab`, `bitbucket`, `gitea` (also Forgejo), `sourcehut` or `azure`. A server without an entry gets its repo's front page.

## Environment variables

Every key above the `[web]` table can be set with an environment variable: `SKL_VAULT`, `SKL_DEFAULT_HOST`, `SKL_PROTOCOL`, `SKL_SCOPE`, `SKL_CLAUDE`.

`SKL_HOME` moves the whole `~/.skl` folder, which is useful in CI or for keeping separate setups.

## Precedence

A command-line flag wins over an environment variable, which wins over the config file, which wins over the default.
