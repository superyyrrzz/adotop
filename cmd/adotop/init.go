package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/config"
)

// Self-contained style set. The TUI's theme system isn't initialized
// before `adotop init` runs (theme load happens inside ui.Run), so we
// can't reuse the package-level styles. Adaptive colors degrade
// gracefully: lipgloss strips SGR on terminals that don't support
// color, so the same code path works in CI, ssh tunnels, and piped
// input without an extra detection step.
var (
	initHeading = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#8839ef", Dark: "#cba6f7"}) // mauve, matches TUI Cursor accent
	initLabel = lipgloss.NewStyle().Bold(true)
	initHint  = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6c6f85", Dark: "#a6adc8"}) // muted text
	initOK = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#a6e3a1"}) // green for "saved"
	initRule = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#ccd0da", Dark: "#45475a"}) // subtle divider
	initChevron = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#8839ef", Dark: "#cba6f7"}) // mauve ›
)

// runInit walks the user through writing ~/.adotop/config.toml. Plain
// stdin/stdout prompts — first-run setup is the wrong place for a TUI
// the user hasn't learned yet. Returns nil on successful save (or
// when the user declines to overwrite an existing file); errors out
// only on real I/O problems.
func runInit() error {
	return runInitWith(os.Stdin, os.Stdout)
}

// runInitWith is the testable form. The CLI calls it via runInit
// with stdin/stdout; tests pump scripted input through any reader
// and capture the resulting config from the disk write or by
// inspecting the captured stdout.
//
// Visual layout: a thin section bar at the top, then each field as a
// short three-line block (bold label, faint hint, prompt with mauve
// chevron). Mirrors the help-modal aesthetic in plain output so the
// init flow doesn't look like a foreign tool dropped in front of the
// TUI.
func runInitWith(in io.Reader, out io.Writer) error {
	exists, err := config.Exists()
	if err != nil {
		return fmt.Errorf("check config: %w", err)
	}
	path, err := config.Path()
	if err != nil {
		return fmt.Errorf("config path: %w", err)
	}

	r := bufio.NewReader(in)

	// Header — section bar in mauve, like the title row of TUI modals.
	fmt.Fprintln(out, initRule.Render("─── ")+initHeading.Render("adotop init")+initRule.Render(" ──────────────────────"))
	fmt.Fprintln(out)
	if exists {
		fmt.Fprintln(out, "  "+initLabel.Render("Editing")+" "+path)
		fmt.Fprintln(out, "  "+initHint.Render("Press enter to keep the value in [brackets]."))
	} else {
		fmt.Fprintln(out, "  "+initLabel.Render("Creating")+" "+path)
		fmt.Fprintln(out, "  "+initHint.Render("These two values are all adotop needs to start."))
	}
	fmt.Fprintln(out)

	cur, _, _ := config.Load() // missing file → defaults; we're about to write either way

	org := promptField(r, out, fieldSpec{
		label:   "Azure DevOps organization",
		hint:    "The `<org>` in https://dev.azure.com/<org>/.",
		current: cur.Org,
	})
	project := promptField(r, out, fieldSpec{
		label:   "Project name",
		hint:    fmt.Sprintf("Find yours at https://dev.azure.com/%s/.", org),
		current: cur.Project,
	})

	cfg := config.Default()
	cfg.Org = org
	cfg.Project = project
	// Preserve any non-default fields the user already had — repo_roots,
	// custom keybindings, the live-test PR ID. init shouldn't wipe
	// state the prompts didn't ask about.
	cfg.RepoRoots = cur.RepoRoots
	cfg.Keybindings = cur.Keybindings
	cfg.PRIDForLiveTest = cur.PRIDForLiveTest

	written, err := config.Write(cfg)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+initOK.Render("✓ Saved")+" "+written)
	fmt.Fprintln(out, "  "+initHint.Render("Run `adotop` to start."))
	return nil
}

// fieldSpec describes one labeled prompt block: bold label, faint
// hint line, then the actual input prompt. current shows up in
// [brackets] when non-empty and is returned on bare enter.
type fieldSpec struct {
	label   string
	hint    string
	current string
}

// promptField renders a three-line field block and reads the answer.
// Required-by-default: a bare enter on an empty default re-asks; a
// bare enter on a non-empty default accepts it.
func promptField(r *bufio.Reader, out io.Writer, f fieldSpec) string {
	fmt.Fprintln(out, "  "+initLabel.Render(f.label))
	fmt.Fprintln(out, "  "+initHint.Render("└─ "+f.hint))
	prompt := "  " + initChevron.Render("›") + " "
	if f.current != "" {
		prompt += fmt.Sprintf("[%s]: ", f.current)
	} else {
		prompt += ": "
	}
	for {
		ans := strings.TrimSpace(promptLine(r, out, prompt))
		if ans != "" {
			fmt.Fprintln(out)
			return ans
		}
		if f.current != "" {
			fmt.Fprintln(out)
			return f.current
		}
		fmt.Fprintln(out, "  "+initHint.Render("(required — try again)"))
	}
}

// promptLine writes the question and reads one line of input from
// the shared reader. We share a single bufio.Reader across the flow
// so the read pointer advances naturally — multiple readers over
// the same underlying stream would each buffer independently and
// drop bytes between prompts.
func promptLine(r *bufio.Reader, out io.Writer, q string) string {
	fmt.Fprint(out, q)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}
