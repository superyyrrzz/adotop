# adotop

A terminal UI for [Azure DevOps](https://dev.azure.com) pull requests. Triage
your queue, read diffs, leave comments, and approve — without leaving the
terminal.

```
┌── Recents (12) ── Assigned (3) ── Created (1) ── Reviewing (8) ─────────────┐
│  ID       State    Title                          Author     Source         │
│  #1151413 OPEN     Fix BuildId collision under …  Alice A.   fix/build-id   │
│▸ #1145087 READY    🛩️ April release prep           Bob B.     master         │
│  #1140193 MERGED   Bump dependency                Dependabot security/      │
│                                                                              │
│  / filter   # goto-pr   enter open   o browser   ? help                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

## What it does

- **Browse PRs** across Recents, Assigned-to-me, Created-by-me, and
  Reviewing tabs. Background refresh keeps the list current without you
  re-entering each PR.
- **Read diffs** with syntax highlighting. Local clone? `git diff` is used
  directly so you get the exact output you'd see in your editor. No
  clone? Falls back to a REST diff that handles binary files, unicode,
  and large files gracefully.
- **Walk threads inline.** Comments render under their target diff line,
  not in a footer, with a cursor that highlights the active thread and
  expand/collapse for long discussions.
- **Leave comments without leaving the TUI.** A textarea overlay lets you
  type multi-line comments while the diff stays visible behind it.
  `ctrl+e` drops to `$EDITOR` if you'd rather use vim.
- **Stale-approval detection.** When the author pushes new commits after
  your approval, the My Vote line shows `· stale, re-approve needed` —
  derived from ADO's own VoteUpdate event stream, no local cache.
- **Per-commit diffs.** Press `M` to view a single commit's changes
  instead of the accumulated PR diff.
- **One-key approve / vote menu / abandon.** `a` to approve, `v` for the
  full vote menu, `X` to abandon (with confirmation).
- **Catppuccin theme support** with auto-detection from your terminal
  background, or pure-ANSI mode that respects your terminal palette.

## Install

### From source

```sh
go install github.com/superyyrrzz/adotop/cmd/adotop@latest
```

Requires Go 1.26+.

### Prebuilt binary

Releases are coming. For now, build from source.

## Quick start

```sh
# 1. One-time: log in to Azure DevOps via the az CLI.
az login

# 2. Tell adotop which org/project to default to.
mkdir -p ~/.adotop
cat > ~/.adotop/config.toml <<'EOF'
org     = "your-org"
project = "your-project"
EOF

# 3. Run.
adotop
```

You can also jump straight to a PR:

```sh
adotop 1145743
adotop https://dev.azure.com/your-org/your-project/_git/your-repo/pullrequest/1145743
```

## Keys

Press `?` from any screen for the full reference. The most common ones:

| Key | What |
|---|---|
| `j` `k` | move cursor (wraps at edges) |
| `enter` | open / drill in |
| `space` | expand thread under cursor |
| `tab` | switch Files ↔ Diff focus |
| `[` `]` | prev / next thread |
| `c` `C` | new comment / reply |
| `a` `v` | approve / open vote menu |
| `M` | view a single commit's diff |
| `o` | open in browser |
| `?` | toggle help |
| `esc` | back / close modal |

The statusline only shows hints relevant to your current state, so the
list grows as you discover features instead of dumping 30 keys at you.

## Config

`~/.adotop/config.toml` (same path on Windows, macOS, Linux):

```toml
org              = "your-org"           # default ADO organization
project          = "your-project"       # default project
refresh_interval = "60s"                # background list refresh cadence
repo_roots       = ["~/code", "~/work"] # where adotop looks for local clones
```

Logs land in `~/.adotop/logs/adotop.log`.

## Theming

adotop uses your terminal's own ANSI palette by default — whatever scheme
you've configured (Solarized, Gruvbox, Windows Terminal, etc.) is what you'll
see. To opt into Catppuccin, set `ADOTOP_THEME`:

| Value             | Effect                                                  |
| ----------------- | ------------------------------------------------------- |
| (unset) / `system`| Use terminal ANSI 4-bit palette (default)               |
| `auto`            | Catppuccin, picked from terminal background             |
| `dark`            | Force Catppuccin Mocha (dark)                           |
| `light`           | Force Catppuccin Latte (light)                          |

`auto` queries the terminal's background color via OSC 11. Some multiplexers
(older tmux, certain SSH setups) don't proxy it; if auto-detect picks the
wrong one, set `ADOTOP_THEME` to `dark` or `light` explicitly.

## Compatibility

- **OS**: Linux, macOS, Windows 10+. CI runs all three.
- **Terminal**: any 256-color (or better) terminal. Emoji glyphs (used in
  status pills and the Discussion entry) need a font with emoji coverage —
  most defaults do. If you see boxes instead of `🔒`/`💬`, install a
  fallback emoji font (Cascadia Code PL, Segoe UI Emoji, etc.).
- **`az` CLI**: required for auth. `az login` once, adotop reuses the
  cached token.

## Build from source

```sh
git clone https://github.com/superyyrrzz/adotop
cd adotop
make build      # produces ./adotop.exe
make test       # unit tests, all OSes
make test-live  # hits a real ADO PR — requires az login
```

## Contributing

Issues and PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the short
version: open an issue first for big changes, tests required, `make test`
must pass.

## License

[MIT](LICENSE)
