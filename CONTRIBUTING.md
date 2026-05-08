# Contributing

Thanks for considering a contribution.

## Before opening a big PR

For non-trivial changes (new keybindings, new screens, new ADO endpoints,
or anything that touches the layout/geometry math) please open an issue
first and let's agree on the shape. Saves us both rework.

Small fixes — typos, obvious bugs, missing tests for existing behavior —
just send the PR.

## Local development

```sh
git clone https://github.com/superyyrrzz/adotop
cd adotop
make build      # produces ./adotop.exe
make test       # full unit-test suite, must pass before merge
make test-live  # exercises real ADO endpoints; requires `az login`
```

`make test-live` is opt-in via the `live` build tag — it doesn't run in
CI. It hits a specific public-ish PR and checks rendered output across a
matrix of pane sizes. Run it before declaring any "PR view" or "diff
rendering" change done.

## Code style

- Go's `gofmt` defaults; CI runs `go vet`.
- Follow the existing patterns in the file you're editing rather than
  importing a new convention.
- Comments explain *why*, not *what*. If a future reader can derive the
  reasoning from the code itself, the comment is noise. If there's a
  hidden constraint, surprising behavior, or a reason a more obvious
  alternative was rejected — write the comment.

## TUI changes

The TUI is the bulk of the project and has its own gotchas. Read
[CLAUDE.md](CLAUDE.md) before touching the rendering layer — short
version:

- Test through the real layout (`renderDetailInLayout` helper), not
  bare `m.View()`. The pane geometry differs.
- Iterate over `forEachPaneSize` for any view-related test. Bugs hide
  at specific widths.
- Measure final composed strings with `lipgloss.Height/Width`, never
  intermediate fragments.

## Commits

Conventional-commits-ish prefixes are nice but not enforced:

- `feat:` / `fix:` / `chore:` / `docs:` / `test:`
- One change per commit when reasonable.
- Body explains the *why*, since the diff already shows the what.

## Pull requests

- Small PRs land faster. If your change touches more than 5 files,
  consider whether it should be two PRs.
- Tests for new behavior, regression tests for fixes.
- Update `?` help, statusline hints, and README if you add a key or a
  user-facing feature.
- CI must be green.

## Reporting bugs

Use the issue templates under `.github/ISSUE_TEMPLATE/`. Include:

- adotop version (`adotop --version` once that's wired) or commit SHA.
- OS + terminal (Windows Terminal, iTerm2, kitty, etc.).
- The exact steps; a screen recording or screenshot if it's a render
  bug.
- Output of `~/.adotop/logs/adotop.log` for the failing session if it's
  a network or auth issue.

## Questions

Open an issue with the `question` label.
