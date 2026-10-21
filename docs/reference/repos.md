# Repo references

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

skl works with any git remote. Every command that takes a repo (`init`, remote selectors in `use`, `add` and `drop`, `learn`, `star`) accepts the same forms.

## Repos

| Form | Means |
|---|---|
| `owner/repo` | A repo on the [default host](./config), GitHub unless you change it |
| `gitlab.com/team/skills` | A repo on another host. A first part containing a dot is a host. |
| `https://git.example.com/team/skills.git` | Any HTTPS URL, used as given |
| `git@git.example.com:team/skills.git` | Any SSH URL, used as given |
| `~/code/skills` | A repo on this machine |

For `owner/repo` and `host/path` forms, skl picks SSH or HTTPS from the [`protocol`](./config) setting.

## Skills inside a repo

Add the path to the skill folder. In GitHub shorthand, the first two parts are the repo and the rest is the path:

```sh
skl learn someone/agent-skills/pdf
```

For every other form, separate the repo from the path with `//`, since a URL alone doesn't say where the repo ends:

```sh
skl learn gitlab.com/team/sub/skills//review/pr-review
skl learn git@git.example.com:team/skills.git//review/pr-review
```

A [skill set](../guide/skill-sets) in a repo is its name after `@`, as the last part of the path:

```sh
skl use someone/agent-skills/@backend
skl use gitlab.com/team/sub/skills//@backend
```

## Pinning a version

Add `#` and a tag, branch or commit:

```sh
skl use someone/agent-skills/pdf#v1.2
```

Without a pin, skl uses the default branch, records the commit it fetched, and keeps using that commit until you run `skl upstream`.

## Authentication

skl uses your normal git setup: SSH keys, the SSH agent and git credential helpers. It never asks for or stores credentials. If git can clone a repo on this machine, skl can use it.

When access fails, the command stops with git's error instead of waiting for a password.

## Host-specific features

Everything works on any host except search, which uses skills.sh and GitHub. Creating a missing vault repo during `skl init` needs the `gh` CLI, so it works only on GitHub. On other hosts, create the repo yourself first.
