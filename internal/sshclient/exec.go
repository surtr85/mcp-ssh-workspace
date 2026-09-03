package sshclient

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Cwd      string `json:"cwd"`
	IsAsync  bool   `json:"is_async,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
}

type CommandOptions struct {
	SudoPassword string
	RunAsSudo    bool
}

// WrapCommand prepares a shell command to execute inside a POSIX subshell (/bin/sh or sh)
// safely from ANY remote login shell (including fish, zsh, csh, tcsh, bash, ash, dash).
// It encodes the command via Base64 to guarantee complete character/quote/variable safety across
// all remote login shells.
// If sudoPassword is provided, it automatically configures a secure SUDO_ASKPASS wrapper
// so that any sudo invocation runs non-interactively without requiring a TTY or manual password prompts.
func WrapCommand(cmd, activeCwd, sudoPassword string) (fullShellCmd, markerExit, markerCwd string) {
	markerExit = "__MCP_EXIT__"
	markerCwd = "__MCP_CWD__"

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("cd %q 2>/dev/null || cd /\n", activeCwd))

	if sudoPassword != "" {
		// Non-interactive sudo askpass setup using /tmp or ~/.cache (avoiding /dev/shm which often has noexec)
		sb.WriteString(`_AP=$(mktemp /tmp/.mcp_ap.XXXXXX 2>/dev/null || mktemp ~/.cache/.mcp_ap.XXXXXX 2>/dev/null || echo "/tmp/.mcp_ap_$$")` + "\n")
		sb.WriteString(fmt.Sprintf("cat << 'EOF_MCP_AP' > \"$_AP\"\n#!/bin/sh\ncat << 'EOF_MCP_PW'\n%s\nEOF_MCP_PW\nEOF_MCP_AP\n", sudoPassword))
		sb.WriteString("chmod 700 \"$_AP\" 2>/dev/null\n")
		sb.WriteString("export SUDO_ASKPASS=\"$_AP\"\n")
		sb.WriteString("sudo() { command sudo -A \"$@\"; }\n")
		sb.WriteString("trap 'rm -f \"$_AP\" 2>/dev/null' EXIT INT TERM HUP\n")
	}

	sb.WriteString(cmd)
	sb.WriteString("\n__EC__=$?\n")
	sb.WriteString(fmt.Sprintf("echo -n \"%s:$__EC__:%s:\" && pwd", markerExit, markerCwd))

	rawScript := sb.String()
	b64Script := base64.StdEncoding.EncodeToString([]byte(rawScript))

	// By executing via base64 pipeline into POSIX sh:
	// 1. Completely agnostic to ANY remote login shell (fish, zsh, csh, tcsh, bash, ash, dash, etc.)
	// 2. No quotes, newlines, variables or syntax characters are evaluated by the remote login shell.
	// 3. Both GNU base64 (-d) and BSD base64 (-D) are supported.
	fullShellCmd = fmt.Sprintf("sh -c 'echo %s | (base64 -d 2>/dev/null || base64 -D) | sh'", b64Script)
	return fullShellCmd, markerExit, markerCwd
}

func (c *Client) RunCommand(cmd string, targetCwd string, isDaemon bool, waitMs int, opts ...CommandOptions) (*ExecResult, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}

	activeCwd := targetCwd
	if activeCwd == "" {
		activeCwd = c.GetCwd()
	}

	var opt CommandOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt.RunAsSudo {
		trimmed := strings.TrimSpace(cmd)
		if !strings.HasPrefix(trimmed, "sudo") {
			cmd = "sudo " + cmd
		}
	}

	sudoPass := opt.SudoPassword
	if sudoPass != "" {
		c.SetSudoPassword(sudoPass)
	} else {
		sudoPass = c.GetSudoPassword()
	}

	sshCli, err := c.SSH()
	if err != nil {
		return nil, err
	}

	sess, err := sshCli.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}

	stdinPipe, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, fmt.Errorf("failed to open stdin pipe: %w", err)
	}

	stdoutBuf := newSyncBuffer()
	stderrBuf := newSyncBuffer()
	sess.Stdout = stdoutBuf
	sess.Stderr = stderrBuf

	fullShellCmd, markerExit, markerCwd := WrapCommand(cmd, activeCwd, sudoPass)

	if isDaemon {
		// Start immediately in background
		if err := sess.Start(fullShellCmd); err != nil {
			sess.Close()
			return nil, fmt.Errorf("failed to start background command: %w", err)
		}

		task := c.taskMgr.Register(cmd, activeCwd, sess, stdinPipe, stdoutBuf, stderrBuf)

		go func() {
			err := sess.Wait()
			task.Completed.Store(true)
			close(task.DoneChan)
			if err != nil {
				if exitErr, ok := err.(*ssh.ExitError); ok {
					task.ExitCode = exitErr.ExitStatus()
				} else {
					task.ErrorMsg = err.Error()
				}
			} else {
				task.ExitCode = 0
			}
			_ = sess.Close()
		}()

		return &ExecResult{
			IsAsync: true,
			TaskID:  task.ID,
			Cwd:     activeCwd,
		}, nil
	}

	// Normal command: start and wait up to waitMs
	if waitMs <= 0 {
		waitMs = 5000
	}

	if err := sess.Start(fullShellCmd); err != nil {
		sess.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	task := c.taskMgr.Register(cmd, activeCwd, sess, stdinPipe, stdoutBuf, stderrBuf)

	done := make(chan error, 1)
	go func() {
		done <- sess.Wait()
	}()

	select {
	case err := <-done:
		task.Completed.Store(true)
		close(task.DoneChan)
		_ = sess.Close()

		rawStdout := stdoutBuf.String()
		rawStderr := stderrBuf.String()

		cleanStdout, newCwd, exitCode := parseCommandMarkers(rawStdout, markerExit, markerCwd)
		if err != nil && exitCode == 0 {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			}
		}

		if newCwd != "" {
			c.SetCwd(newCwd)
		} else {
			newCwd = activeCwd
		}

		return &ExecResult{
			Stdout:   cleanStdout,
			Stderr:   rawStderr,
			ExitCode: exitCode,
			Cwd:      newCwd,
		}, nil

	case <-time.After(time.Duration(waitMs) * time.Millisecond):
		// Command is still running after waitMs -> convert to background task!
		go func() {
			err := <-done
			task.Completed.Store(true)
			close(task.DoneChan)
			if err != nil {
				if exitErr, ok := err.(*ssh.ExitError); ok {
					task.ExitCode = exitErr.ExitStatus()
				} else {
					task.ErrorMsg = err.Error()
				}
			} else {
				task.ExitCode = 0
			}
			_ = sess.Close()
		}()

		return &ExecResult{
			IsAsync: true,
			TaskID:  task.ID,
			Stdout:  stdoutBuf.String(),
			Stderr:  stderrBuf.String(),
			Cwd:     activeCwd,
		}, nil
	}
}

func parseCommandMarkers(output, markerExit, markerCwd string) (string, string, int) {
	// Look for "__MCP_EXIT__:<exitCode>:__MCP_CWD__:<path>"
	idx := strings.LastIndex(output, markerExit+":")
	if idx == -1 {
		return output, "", 0
	}

	cleanOutput := output[:idx]
	meta := output[idx+len(markerExit)+1:]

	parts := strings.SplitN(meta, ":"+markerCwd+":", 2)
	exitCode := 0
	newCwd := ""

	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &exitCode)
	}
	if len(parts) == 2 {
		newCwd = strings.TrimSpace(parts[1])
	}

	return cleanOutput, newCwd, exitCode
}
