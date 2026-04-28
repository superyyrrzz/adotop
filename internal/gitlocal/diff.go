package gitlocal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// diffNormalizationFlags returns the git-diff options that align local
// rendering with the Azure DevOps web UI:
//
//   - --ignore-cr-at-eol      : treat CRLF and LF as equal (Windows checkouts).
//   - --ignore-space-at-eol   : ignore trailing-whitespace edits (web UI hides
//     these by default and reviewers rarely care).
//
// Set ADOTOP_DIFF_STRICT=1 to disable both and see every byte difference.
func diffNormalizationFlags() []string {
	if v := strings.TrimSpace(os.Getenv("ADOTOP_DIFF_STRICT")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return nil
	}
	return []string{"--ignore-cr-at-eol", "--ignore-space-at-eol"}
}

// Diff runs `git -C clonePath diff target..source -- file`. If useDelta is true
// and `delta` is on PATH, the diff is piped through it for syntax highlighting.
//
// ctxLines is the unified-diff context size (`-U<n>`). Pass 3 for the
// git default. Pass a very large number (e.g. 1<<30) to effectively show
// the entire file.
//
// Diff applies whitespace normalization (CRLF, trailing spaces) by default to
// match Azure DevOps' web UI; set ADOTOP_DIFF_STRICT=1 to opt out.
func Diff(ctx context.Context, clonePath, targetSha, sourceSha, file string, useDelta bool, ctxLines int) ([]byte, error) {
	args := []string{"-C", clonePath, "diff", "--no-color"}
	if useDelta {
		args = []string{"-C", clonePath, "diff"}
	}
	args = append(args, fmt.Sprintf("-U%d", ctxLines))
	args = append(args, diffNormalizationFlags()...)
	args = append(args, targetSha+".."+sourceSha, "--", file)

	gitCmd := exec.CommandContext(ctx, "git", args...)

	if !useDelta {
		out, err := gitCmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	if _, err := exec.LookPath("delta"); err != nil {
		return Diff(ctx, clonePath, targetSha, sourceSha, file, false, ctxLines)
	}

	pr, pw := io.Pipe()
	gitCmd.Stdout = pw
	deltaCmd := exec.CommandContext(ctx, "delta", "--paging=never")
	deltaCmd.Stdin = pr
	var buf bytes.Buffer
	deltaCmd.Stdout = &buf

	if err := gitCmd.Start(); err != nil {
		return nil, err
	}
	if err := deltaCmd.Start(); err != nil {
		return nil, err
	}
	gitErr := gitCmd.Wait()
	pw.Close()
	deltaErr := deltaCmd.Wait()
	if gitErr != nil {
		return nil, gitErr
	}
	if deltaErr != nil {
		return nil, deltaErr
	}
	return buf.Bytes(), nil
}

// HasDelta reports whether `delta` is on PATH.
func HasDelta() bool {
	_, err := exec.LookPath("delta")
	return err == nil
}
