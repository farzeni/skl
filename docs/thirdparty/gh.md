# gh

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Let `skl init` create your vault repo on GitHub, and fetch with the protocol you already use there. The part uses the [`gh` CLI](https://cli.github.com), GitHub's own command line tool, which must be installed and logged in (`gh auth login`).

## Create your vault repo

When the repo you give `skl init` doesn't exist and its host is GitHub, skl offers to create it as a private repo:

```
? Create you/skills, a private repo on GitHub?
  › yes   no
→ new vault you/skills (private, on GitHub)
```

skl runs `gh repo create you/skills --private`, then carries on as with any empty repo. See [Getting started](../guide/getting-started#make-your-vault).

Without `gh`, or on another host, create the repo yourself first. skl stops and says so:

```
✗ you/skills doesn't exist
  · create it, then skl init you/skills
```

## Pick SSH or HTTPS

When `protocol` isn't set in your [config](../reference/config), skl uses the protocol `gh` is configured with for that host (`gh config get git_protocol`). Without `gh`, the default is `https`.

## Without this part

Every other command works as before. Nothing else in skl runs `gh`.
