# Security Policy

## Reporting a vulnerability

adotop is a local-only tool — it reads your Azure DevOps PRs via your
own `az login` credentials, caches data in `~/.adotop/`, and never
sends anything to a third-party server. Vulnerabilities that affect
**other users** are unlikely but not impossible (e.g., a path-handling
bug that could overwrite arbitrary files, a parser bug that hangs the
terminal on malicious input).

If you find one, please **don't open a public issue.** Instead:

- Open a [private security advisory](https://github.com/superyyrrzz/adotop/security/advisories/new)
  on this repo. GitHub will route it to the maintainers and keep the
  details out of the public timeline until a fix ships.

I aim to acknowledge reports within a week, and to ship a fix or a
mitigation within two weeks of acknowledgement. If a report needs
more urgency than that, say so in the advisory and I'll prioritize.

## What counts

- **In scope:** anything in this repo's source. Path traversal,
  command injection, terminal-escape-sequence injection, panics that
  drop the user into an unrecoverable state, credential leakage
  through the log file or cache.
- **Out of scope:** Azure DevOps itself, the `az` CLI, or your
  terminal emulator. Report those upstream.

## Supported versions

Only the latest tagged release receives security patches. Older
versions are unsupported.
