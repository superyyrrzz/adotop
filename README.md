# adotop

A terminal UI for Azure DevOps. Built for engineers who live in ADO all day and want to stay in the terminal.

Status: **Stage 0 — foundation.** See `docs/superpowers/specs/2026-04-25-adotop-roadmap.md` for the roadmap.

## Build

```sh
go build ./cmd/adotop
```

## Auth

Uses your existing `az` CLI login. Run `az login` once, then `adotop` will obtain ADO bearer tokens on demand.

## Config

`~/.adotop/config.toml` — same path on Windows, macOS, Linux. Example:

```toml
org = "ceapex"
project = "Engineering"
refresh_interval = "60s"
```

Logs go to `~/.adotop/logs/adotop.log`.

## Theming

adotop uses your terminal's own ANSI palette by default — whatever
scheme you've configured (Solarized, Gruvbox, Windows Terminal, etc.)
is what you'll see. Override with the `ADOTOP_THEME` environment
variable to opt into Catppuccin:

| Value           | Effect                                           |
| --------------- | ------------------------------------------------ |
| (unset) / `system` | Use terminal ANSI 4-bit palette (default)     |
| `auto`          | Catppuccin, picked from terminal background      |
| `dark`          | Force Catppuccin Mocha (dark)                    |
| `light`         | Force Catppuccin Latte (light)                   |

`auto` detection uses `termenv.HasDarkBackground()`, which queries the
terminal's background color via OSC 11. Some multiplexers (older tmux,
certain SSH setups) don't proxy this; if auto-detect picks the wrong
one, set `ADOTOP_THEME` to `dark` or `light` explicitly.
