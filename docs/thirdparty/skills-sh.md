# skills.sh

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

[skills.sh](https://skills.sh) is a public directory of agent skills, run by Vercel. It lists skills from public GitHub repos and ranks them by how often people install them with [`npx skills`](./npx-skills).

## Search

```sh
skl search pdf
```

```
someone/agent-skills/pdf          2.1K installs
other/skills/pdf-tools             880 installs
someone/agent-skills/pdf-forms     310 installs
```

Each result is a [repo reference](../reference/repos), most installed first, up to 20. Paste one into `skl learn`:

```sh
skl learn someone/agent-skills/pdf
```

One word matches names loosely. Several words search by meaning:

```sh
skl search review pull requests
```

`--owner` limits the results to one GitHub user or organization:

```sh
skl search react --owner vercel
```

Results come from GitHub only, because skills.sh indexes nothing else. Skills that skills.sh lists from a website rather than a git repo are left out, since skl can't load them.

Search needs skills.sh. When it can't be reached, the command stops:

```
✗ skills.sh unreachable (no such host) · skl search pdf
```

No other command contacts skills.sh. skl sends it nothing besides your search words, so learning a skill with skl doesn't count as an install there.

skl uses the same search address as `npx skills find`. skills.sh doesn't document that address, so it may change; if it does, only `skl search` stops working.

## Links

A link copied from the skills.sh website works wherever a repo reference does:

```sh
skl learn https://skills.sh/someone/agent-skills/pdf
```

skl reads `https://skills.sh/<owner>/<repo>/<skill>` and `https://www.skills.sh/...` as `<owner>/<repo>/<skill>`.

## Configuration

In `~/.skl/config.toml`:

```toml
[skills_sh]
url = "https://skills.sh"
```

| Key | Default | Effect |
|---|---|---|
| `url` | `https://skills.sh` | Where `skl search` sends its requests. |

## Getting listed

You don't submit anything. A vault on public GitHub appears on skills.sh once people install from it with `npx skills`; see [Installing from your vault](./npx-skills#installing-from-your-vault).
