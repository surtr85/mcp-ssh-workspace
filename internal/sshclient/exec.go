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

func (c *Client) RunCommand(cmd string, targetCwd string, isDaemon bool, waitMs int) (*ExecResult, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}

	activeCwd := targetCwd
	if activeCwd == "" {
		activeCwd = c.GetCwd()
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

	// Wrap command to execute in the right cwd and report back the final cwd & exit code cleanly
	// Use bash if available, fallback to sh
	markerExit := "__MCP_EXIT__"
	markerCwd := "__MCP_CWD__"

	wrappedCmd := fmt.Sprintf(
		"cd %q 2>/dev/null || cd / ; %s\n__EC__=$?\necho -n \"%s:$__EC__:%s:\" && pwd",
		activeCwd,
		cmd,
		markerExit,
		markerCwd,
	)
	encodedCmd := base64.StdEncoding.EncodeToString([]byte(wrappedCmd))
	fullShellCmd := fmt.Sprintf("echo %s | base64 -d | sh", encodedCmd)

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
