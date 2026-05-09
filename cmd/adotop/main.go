package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/applog"
	"github.com/superyyrrzz/adotop/internal/config"
	"github.com/superyyrrzz/adotop/internal/ui"
)

// Version metadata, populated by GoReleaser via -X ldflags at
// release-build time:
//
//	go build -ldflags="-X main.version=v0.1.0 -X main.commit=abc1234 -X main.date=2026-05-09"
//
// Default values are what `go build` (with no ldflags) and `go install`
// produce — useful for local dev so `--version` doesn't print empty
// strings, but not what release binaries should ever show.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// errSilent is returned by run() when an error has already been
// printed to stderr in a friendly form. main() exits non-zero
// without prepending its usual "adotop:" prefix so the user sees
// the bespoke message we wrote, not a doubled-up wrapper line.
var errSilent = errors.New("silent")

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errSilent) {
			fmt.Fprintln(os.Stderr, "adotop:", err)
		}
		os.Exit(1)
	}
}

func run() error {
	// Top-level flags. We hand-roll instead of using flag.Parse so the
	// existing positional-arg path (`adotop 1234` / `adotop <url>`) and
	// the `init` subcommand keep working without rewriting their
	// dispatch shape.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("adotop %s (%s, %s)\n", version, commit, date)
			return nil
		case "--help", "-h", "help":
			printUsage(os.Stdout)
			return nil
		}
	}

	// Subcommand dispatch. `adotop init` runs the config-writer flow
	// and exits — no TUI, no log file, no token fetch. Anything else
	// falls through to the normal TUI launch path.
	if len(os.Args) >= 2 && os.Args[1] == "init" {
		return runInit()
	}

	cfg, cfgPath, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	prID, err := parsePRArg(os.Args[1:])
	if err != nil {
		return err
	}

	// First-run UX: if the user has never written a config (vs. wrote
	// one with empty org, which is their choice), auto-run init so
	// the README quick start can collapse to `az login` + `adotop`.
	// Skip when launching against a specific PR (`adotop 1234`) — the
	// user's already past onboarding.
	if cfg.Org == "" && prID == 0 {
		exists, _ := config.Exists()
		if !exists {
			fmt.Fprintln(os.Stderr, "No config found. Walking you through setup.")
			fmt.Fprintln(os.Stderr)
			if err := runInit(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr)
			cfg, cfgPath, err = config.Load()
			if err != nil {
				return fmt.Errorf("reload config after init: %w", err)
			}
		}
	}

	logPath, closeLog, err := applog.Init(slog.LevelInfo)
	if err != nil {
		return fmt.Errorf("init log: %w", err)
	}
	defer closeLog()
	slog.Info("adotop starting", "config", cfgPath, "log", logPath, "org", cfg.Org, "project", cfg.Project, "initial_pr", prID)

	if cfg.Org == "" {
		// Org isn't fatal — we render a clear message in the shell — but we can't call ADO without it.
		slog.Warn("no org configured; ADO calls will fail until you set 'org' in the config file")
	}

	tokens := ado.NewAzCLITokenProvider()
	// Pre-flight: verify `az` is installed and the user is logged in
	// before launching the TUI. Without this, an unauthenticated user
	// sees a working-but-empty TUI with a cryptic "auth: get token:
	// exec: az: not found" footer banner — bad first impression.
	// Errors are classified so the message tells the user what to do
	// rather than dumping exec internals.
	if cfg.Org != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := tokens.Token(ctx); err != nil {
			printAuthError(os.Stderr, err)
			return errSilent
		}
	}
	client := ado.NewClient(cfg.Org, tokens)

	return ui.Run(cfg, client, prID)
}

// printAuthError translates a token-fetch failure into an actionable
// message on stderr. Sentinel errors get bespoke text; anything else
// falls through to the raw error so we don't hide unexpected failures.
func printAuthError(w io.Writer, err error) {
	switch {
	case errors.Is(err, ado.ErrAzNotInstalled):
		fmt.Fprintln(w, "adotop: the `az` CLI is required but isn't installed.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Install it from https://aka.ms/install-azure-cli")
		fmt.Fprintln(w, "  Then run `az login` and try `adotop` again.")
	case errors.Is(err, ado.ErrAzNotLoggedIn):
		fmt.Fprintln(w, "adotop: not logged in to Azure DevOps.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Run `az login` and try `adotop` again.")
	default:
		fmt.Fprintf(w, "adotop: auth failed: %v\n", err)
	}
}

// printUsage writes the top-level help text. Short by design — full
// in-app keybinding reference lives behind `?` in the TUI itself.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "adotop — terminal UI for Azure DevOps pull requests")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  adotop                           Launch the TUI on the recents tab")
	fmt.Fprintln(w, "  adotop <pr-id>                   Open a specific PR by ID")
	fmt.Fprintln(w, "  adotop <pr-url>                  Open a PR by Azure DevOps URL")
	fmt.Fprintln(w, "  adotop init                      Write or update ~/.adotop/config.toml")
	fmt.Fprintln(w, "  adotop --version                 Print version, commit, build date")
	fmt.Fprintln(w, "  adotop --help                    This message")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Press ? from any screen for the in-app keybinding reference.")
	fmt.Fprintln(w, "Logs land in ~/.adotop/logs/adotop.log.")
	fmt.Fprintln(w, "Project home: https://github.com/superyyrrzz/adotop")
}

// prURLRE matches the trailing `/pullrequest/<id>` (also `/pull/<id>` for
// safety) of an Azure DevOps PR URL. The host/org/project/repo prefix
// varies across dev.azure.com vs visualstudio.com vs on-prem; we only
// care about the ID.
var prURLRE = regexp.MustCompile(`(?i)/pull(?:request)?/(\d+)`)

// parsePRArg interprets the program's positional args. Accepts:
//   - no args (returns 0, no error — normal startup)
//   - one arg: an ADO PR URL or a bare numeric PR ID
//
// Returns 0 and a friendly error for anything else.
func parsePRArg(args []string) (int, error) {
	if len(args) == 0 {
		return 0, nil
	}
	if len(args) > 1 {
		return 0, errors.New("usage: adotop [<pr-url-or-id>]")
	}
	a := strings.TrimSpace(args[0])
	if n, err := strconv.Atoi(a); err == nil && n > 0 {
		return n, nil
	}
	if m := prURLRE.FindStringSubmatch(a); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, nil
	}
	return 0, fmt.Errorf("can't parse %q as a PR URL or ID", a)
}
