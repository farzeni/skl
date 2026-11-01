# Selectors and references

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Every command that loads skills takes the same selectors, and every command that takes a repo accepts the same reference forms. Selectors name things in your vault; references name things in any git remote.

## Selectors

`use`, `add` and `drop` take selectors. They're applied left to right, and on a [name clash](../guide/skill-sets#names-and-clashes) the later one wins.

| Selector | Loads |
|---|---|
| `review` | Every skill in one of your groups |
| `review/pr-review` | One skill from one of your groups |
| `@backend` | A named skill set from your vault |
| `-review/test-gaps` | Removes a skill that an earlier selector added |

```sh
skl use @backend -review/test-gaps someone/pdf
```

skl's [flags](./commands#flags) start with `--` and can go anywhere on the line. Every other word is a selector, so `-review/test-gaps` is never read as a flag.

A selector has one or two parts. A word with three or more parts, a host, a `#` pin, a URL or a path is a [reference](#repos) to someone else's repo: `use`, `add` and `drop` stop and tell you to [learn it](../guide/learn#get-a-skill) first.

### Grammar

```
selector  = [ "-" ] ( group | group "/" skill | "@" set )
group     = name        skill = name        set = name
name      = letters, digits, "-" and "_", starting with a
            letter or a digit
```

- **One part** is a group, **two parts** are a skill in a group, and **`@name`** is a named skill set in `sets/`. A skill is found by its folder name or its frontmatter `name`.
- **Letters** can be from any script, with their combining marks: `résumé` works whether `é` is one character or `e` plus an accent.
- **`-`** removes what the rest of the selector names from what the earlier selectors added: `-review/test-gaps`, `-review` or `-@base`. [Clashes](../guide/skill-sets#names-and-clashes) are settled after that.
- **A reference** is a word with three or more parts, a dot in the first part, a `#` (`review#v1`), a URL (`https://…`, `git@host:…`), or one that starts with `~`, `/` or `.`. Every name is free of dots, so a group can't be mistaken for a host.
- **Any other word is invalid,** and the command stops with a [usage error](./commands#exit-codes): `review/pr.review`, a word with a space, `review/@base`, `@a.b`. A word that starts with `@` and has fewer than three parts is a skill set's name, never a host or a pin.
- **Reserved names.** A group can't be named `sets`, `lessons` or `h`, in any case: on a case-insensitive disk, `Sets` is the folder `sets`. When a new skill's group would be the owner's name and that name is reserved or isn't a valid name, skl uses `<owner>-skills` instead. If that isn't valid either, as with the owner `my.org`, there's no default group, and `skl learn` asks you to name one:

  ```
  ✗ the owner my.org gives no valid group name
    · skl learn gitlab.com/my.org/skills//pdf <group>/
  ```

- **A selector that matches nothing** is a warning. If the word names a skill but no group, the hint repeats your command with only that word replaced: `skl use @a pdf` warns `! pdf   matches no group · skl use @a someone/pdf`, and `-pdf` becomes `-someone/pdf`. Anything else hints at dropping it: `! nope   matches no group · skl drop nope`. A line in a named skill set hints at [its file](../guide/skill-sets#whats-loaded).
- **`skl drop` of a word that matches nothing** removes it without a warning if it's a line in your active skill set. If it isn't, skl warns: `! nope   isn't in the active skill set · skl status`.
- **`-h` and `--`** follow the [flag rules](./commands#flags): `-h` is help, never a removal, and every word after `--` is an argument.

## Repos

Every command that takes a repo (`init`, `learn`, `open`) accepts the same forms.

| Form | Means |
|---|---|
| `owner/repo` | A repo on the [default host](./config), GitHub unless you change it |
| `gitlab.com/team/skills` | A repo on another host. A first part containing a dot is a host. |
| `https://git.example.com/team/skills.git` | Any HTTPS URL, used as given |
| `git@git.example.com:team/skills.git` | Any SSH URL, used as given |
| `~/code/skills` | A repo on this machine |

For `owner/repo` and `host/path` forms, skl picks SSH or HTTPS from the [`protocol`](./config) setting.

### How a reference is stored

skl records the sources and peers you learn from as full URLs, so they mean the same thing on every machine. `owner/repo` and `host/path` forms are stored as HTTPS, whatever `protocol` you fetch with: `someone/agent-skills/pdf` becomes `https://github.com/someone/agent-skills//skills/pdf`, with the path skl found after `//`. A URL you typed is stored as you typed it. A trailing `.git` is dropped, so two references to the same repo match with or without it. A pin is stored apart, as `ref` in [`.skl-origin.toml`](./files#skl-origin-toml).

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
skl learn someone/agent-skills/pdf    # the folder is skills/pdf/
```

If two skills in the repo have that name, skl stops and lists both paths, so you can pick one:

```
✗ two skills named pdf: skills/pdf, legacy/pdf
  · skl learn someone/agent-skills/skills/pdf
```

skl records the path it found, so a renamed or added skill elsewhere in the repo doesn't change what you learned.

If the whole repo is one skill, end the reference with `//`: `skl learn someone/pdf-skill//`. The copy takes the `name` in its `SKILL.md`, else the repo's name.

A [skill set](../guide/skill-sets) in a repo is its name after `@`, as the last part of the path:

```sh
skl learn someone/agent-skills/@backend
skl learn gitlab.com/team/sub/skills//@backend
```

## Pinning a version

Add `#` and a tag, branch or commit:

```sh
skl learn someone/agent-skills/pdf#v1.2
```

skl copies that version and records the pin with the copy. A tag or a commit stays where it is: the skill never shows an update, and you move it by learning a newer one (`…/pdf#v1.3`). A branch, or no pin at all, follows that branch: [`skl sync`](../guide/sync) finds its new commits and [`skl learn`](../guide/learn#keep-up-with-its-author) takes them.

## Web pages

`skl open` shows a skill, a skill set or a repo in your browser:

| You type | It opens |
|---|---|
| `skl open` | Your vault's repo page |
| `skl open review/pr-review` | The skill's folder in your vault repo |
| `skl open @backend` | `sets/backend.toml` in your vault repo |
| `skl open someone/agent-skills/pdf` | The skill's folder in their repo |
| `skl open someone/agent-skills/@backend` | Their `sets/backend.toml` |
| `skl open someone/agent-skills` | Their repo page |

Here two parts mean a skill in your vault if one exists, such as `someone/pdf`, and otherwise a repo on the default host, such as `someone/agent-skills`.

It always prints the URL:

```
$ skl open review/pr-review
https://github.com/you/skills/tree/HEAD/review/pr-review
```

A page in someone else's repo shows the version you pinned with `#`, else the default branch. A page in your vault shows its default branch, as you last pushed it: if the skill has commits you haven't pushed yet, skl says so, and [`skl sync`](../guide/sync) pushes them.

For a skill you copied from someone else, skl opens your copy and prints where it came from below it:

```
$ skl open someone/pdf
https://github.com/you/skills/tree/HEAD/someone/pdf
source:
  https://github.com/someone/agent-skills/tree/3f9a1c2/skills/pdf
```

skl starts your browser only when there's a screen to show it on. It uses `$BROWSER` if set, else the system's default. Over SSH, without a display, or when the output goes to a pipe, it only prints the URL, so `skl open review/pr-review | pbcopy` copies it. If the browser doesn't start, skl says so after the URL and still exits 0.

### Hosts

skl knows the page layout of these hosts:

| Host | Folder page | Checked |
|---|---|---|
| GitHub | `https://github.com/<repo>/tree/<ref>/<path>` | yes |
| GitLab | `https://gitlab.com/<repo>/-/tree/<ref>/<path>` | yes |
| Bitbucket | `https://bitbucket.org/<repo>/src/<ref>/<path>` | yes |
| Codeberg, Gitea, Forgejo | `https://codeberg.org/<repo>/src/<ref>/<path>` | not yet |
| sourcehut | `https://git.sr.ht/<repo>/tree/<ref>/item/<path>` | not yet |
| Azure Repos | `https://dev.azure.com/<org>/<project>/_git/<repo>?path=/<path>` | not yet |

A skill set is a file, so GitHub and GitLab show it under `blob/` instead of `tree/`.

For your own server, say which of these it runs in the [`[web]`](./config#your-own-git-servers) table of your config. Without an entry, skl opens the repo's front page, `https://<host>/<repo>`, which works on every host, and tells you how to add one.

A repo on this machine (`~/code/skills`) has no web page: skl prints the folder's path and opens nothing.

## Authentication

skl uses your normal git setup: SSH keys, the SSH agent and git credential helpers. It never asks for or stores credentials. If git can clone a repo on this machine, skl can use it.

When access fails, the command stops with git's error instead of waiting for a password.

## Host-specific features

Everything works on any host. The optional [skills.sh part](../thirdparty/skills-sh) searches GitHub only.
