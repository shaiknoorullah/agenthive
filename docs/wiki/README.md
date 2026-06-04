# Wiki content (source)

This directory is the source of truth for the [agenthive GitHub Wiki](https://github.com/shaiknoorullah/agenthive/wiki).

Why it lives in the main repo:
- It can be browsed directly on github.com (`docs/wiki/Home.md` etc.)
- It can be edited via normal PRs, with review and CI
- It survives wiki accidents (wiki history is separate from the main repo)

Why it also lives in the wiki:
- The wiki has nicer navigation (sidebar, footer, page-name links)
- `[[Page Name]]` wikilinks resolve inside the wiki
- It's where most users will look first

## Layout

| File | Role |
|---|---|
| `Home.md` | Wiki landing page |
| `_Sidebar.md` | Left navigation (wiki-only) |
| `_Footer.md` | Bottom of every page (wiki-only) |
| `Installation.md`, `Quick-Start.md`, `Configuration.md` | Getting started |
| `Claude-Code-Integration.md`, `tmux-Plugin.md` | Integration guides |
| `CLI-Reference.md`, `Routing.md`, `Security-Model.md` | Reference |
| `Architecture.md`, `NAT-Traversal.md`, `CRDT-State-Sync.md`, `Action-Gate.md` | Internals |
| `Troubleshooting.md`, `FAQ.md`, `Glossary.md` | Help |
| `Design-Decisions.md`, `Roadmap.md`, `Release-Notes.md` | Project |

## Browsing on github.com vs the wiki

Two conventions don't render the same way:

1. `[[Page Name]]` — works on the wiki, displays as literal text on github.com.
2. `_Sidebar.md` / `_Footer.md` — wiki auto-renders these. github.com treats them like any other file.

When you read these files at `docs/wiki/` on github.com, you'll see `[[Page Name]]` as text. That's fine — links to `Page-Name.md` work too if you need to click through.

## Syncing to the wiki

The wiki is a separate git repository at `git@github.com:shaiknoorullah/agenthive.wiki.git`. The first time, GitHub requires you to create at least one page through the web UI to materialize the repo. After that, content syncs via plain git.

To push the contents of this directory to the wiki:

```bash
./scripts/sync-wiki.sh
```

The script clones the wiki, copies every `*.md` here over the wiki's working tree, commits with a stable subject, and pushes. Run it after any merge to `main` that touches `docs/wiki/`.

## Editing

Edit any file with a normal pull request. Two things to keep in mind:

1. Cross-page links should use `[[Page Name]]` (the wiki syntax). Browsing on github.com will show it as literal — that's the trade. The vast majority of readers will land on the wiki, not the source files.
2. Image references should be absolute URLs (the wiki's relative path resolution differs from github.com's).

## License

MIT, same as the rest of the repo.
