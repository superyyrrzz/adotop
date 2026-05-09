package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/applog"
	"github.com/superyyrrzz/adotop/internal/config"
	"github.com/superyyrrzz/adotop/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "adotop:", err)
		os.Exit(1)
	}
}

func run() error {
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
	client := ado.NewClient(cfg.Org, tokens)

	return ui.Run(cfg, client, prID)
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
