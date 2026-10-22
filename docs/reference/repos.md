# Selectors and references

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Every command that loads skills takes the same selectors, and every command that takes a repo accepts the same reference forms. skl works with any git remote.

## Selectors

`use`, `add` and `drop` take selectors. They're applied left to right, and on a [name clash](../guide/skill-sets#names-and-clashes) the later one wins.

| Selector | Loads |
|---|---|
| `review` | Every skill in one of your groups |
| `review/pr-review` | One skill from one of your groups |
| `@backend` | A named skill set from your vault |
| `owner/repo/skill` | A skill from someone else's repo, used read-only and pinned to a commit. Any reference form below works. |
| `owner/repo/@backend` | A skill set from someone else's repo, used read-only. Its selectors are read inside that repo, so `review` there means their `review` group. |
| `-review/test-gaps` | Removes a skill that an earlier selector added |

```sh
skl use @backend -review/test-gaps anthropics/skills/pdf
```

skl's flags start with `--` and can go anywhere on the line. Every other word is a selector, so `-review/test-gaps` is never read as a flag.

## Repos

Every command that takes a repo (`init`, remote selectors in `use`, `add` and `drop`, `learn`, `star`) accepts the same forms.

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

You can also name the skill instead of giving its path. If there's no `SKILL.md` at the path and the path is one word, skl looks for the skill in the repo whose folder or frontmatter `name` is that word, wherever it is:

```sh
skl use anthropics/skills/pdf     # the skill lives in skills/pdf/
```

If two skills in the repo have that name, skl stops and lists both paths, so you can pick one:

```
anthropics/skills has two skills named pdf: skills/pdf, legacy/pdf
  use the path, e.g. anthropics/skills/skills/pdf
```

skl records the path it found, so a renamed or added skill elsewhere in the repo doesn't change what you loaded.

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

Everything works on any host. The optional [skills.sh part](../thirdparty/skills-sh) searches GitHub only. Creating a missing vault repo during `skl init` needs the `gh` CLI, so it works only on GitHub. On other hosts, create the repo yourself first.
