package gitlocal

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

// Diff runs `git -C clonePath diff target..source -- file`. If useDelta is true
// and `delta` is on PATH, the diff is piped through it for syntax highlighting.
func Diff(ctx context.Context, clonePath, targetSha, sourceSha, file string, useDelta bool) ([]byte, error) {
	args := []string{"-C", clonePath, "diff", "--no-color"}
	if useDelta {
		args = []string{"-C", clonePath, "diff"}
	}
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
		return Diff(ctx, clonePath, targetSha, sourceSha, file, false)
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
