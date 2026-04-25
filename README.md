# adotop

A terminal UI for Azure DevOps. Built for engineers who live in ADO all day and want to stay in the terminal.

Status: **Stage 0 — foundation.** See `docs/superpowers/specs/2026-04-25-adotop-roadmap.md` for the roadmap.

## Build

```sh
go build ./cmd/adotop
```

## Auth

Uses your existing `az` CLI login. Run `az login` once, then `adotop` will obtain ADO bearer tokens on demand.
