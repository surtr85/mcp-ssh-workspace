package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/surtr85/mcp-ssh-workspace/internal/sshclient"
)

func RegisterTools(s *server.MCPServer, client *sshclient.Client) {
	registerCommandTools(s, client)
	registerFileTools(s, client)
	registerSearchTools(s, client)
}

func registerCommandTools(s *server.MCPServer, client *sshclient.Client) {
	// 0. remote_connect
	connectTool := mcp.NewTool("remote_connect",
		mcp.WithDescription("Connect to a remote SSH host dynamically. Resolves ~/.ssh/config aliases automatically."),
		mcp.WithString("Host", mcp.Required(), mcp.Description("Remote SSH host, IP, or ~/.ssh/config alias.")),
		mcp.WithInteger("Port", mcp.Description("SSH port (default 22).")),
		mcp.WithString("User", mcp.Description("SSH username.")),
		mcp.WithString("KeyPath", mcp.Description("Path to private key file.")),
		mcp.WithString("Password", mcp.Description("SSH password if not using key.")),
	)

	s.AddTool(connectTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		host, err := request.RequireString("Host")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		port := request.GetInt("Port", 22)
		user := request.GetString("User", "")
		keyPath := request.GetString("KeyPath", "")
		password := request.GetString("Password", "")

		if err := client.ConnectTo(host, port, user, keyPath, password); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to connect to %s: %v", host, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully connected to %s. CWD: %s", host, client.GetCwd())), nil
	})

	// 0.1 remote_disconnect
	disconnectTool := mcp.NewTool("remote_disconnect",
		mcp.WithDescription("Disconnect from the current remote SSH host."),
	)

	s.AddTool(disconnectTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client.Close()
		return mcp.NewToolResultText("Disconnected from remote SSH host."), nil
	})

	// 1. remote_run_command
	runCmdTool := mcp.NewTool("remote_run_command",
		mcp.WithDescription("Execute a bash command on the remote host with persistent working directory (CWD) and optional background task management."),
		mcp.WithString("CommandLine", mcp.Required(), mcp.Description("The exact command line string to execute on the remote machine.")),
		mcp.WithString("Cwd", mcp.Description("Optional remote working directory. If omitted, uses the persistent session CWD.")),
		mcp.WithBoolean("IsDaemon", mcp.Description("Set to true for long-running support processes that should keep running in the background indefinitely.")),
		mcp.WithInteger("WaitMsBeforeAsync", mcp.Description("Milliseconds to wait for the command to finish before detaching to background (default 5000ms).")),
	)

	s.AddTool(runCmdTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cmd, err := request.RequireString("CommandLine")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		cwd := request.GetString("Cwd", "")
		isDaemon := request.GetBool("IsDaemon", false)
		waitMs := request.GetInt("WaitMsBeforeAsync", 5000)

		res, err := client.RunCommand(cmd, cwd, isDaemon, waitMs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Execution error: %v", err)), nil
		}

		jsonBytes, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(jsonBytes)), nil
	})

	// 2. remote_manage_task
	manageTaskTool := mcp.NewTool("remote_manage_task",
		mcp.WithDescription("Manage background tasks running on the remote host (list, status, kill, send_input)."),
		mcp.WithString("Action", mcp.Required(), mcp.Description("The action to perform: 'list', 'status', 'kill', or 'send_input'.")),
		mcp.WithString("TaskId", mcp.Description("Task ID (e.g. 'task-1'). Required for 'status', 'kill', and 'send_input'.")),
		mcp.WithString("Input", mcp.Description("Input string to send to stdin of the task. Required when Action is 'send_input'.")),
	)

	s.AddTool(manageTaskTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action, err := request.RequireString("Action")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		taskID := request.GetString("TaskId", "")
		input := request.GetString("Input", "")

		tm := client.TaskManager()

		switch action {
		case "list":
			tasks := tm.List()
			type taskSummary struct {
				ID        string `json:"id"`
				Command   string `json:"command"`
				Cwd       string `json:"cwd"`
				StartTime string `json:"start_time"`
				Completed bool   `json:"completed"`
				ExitCode  int    `json:"exit_code"`
			}
			var list []taskSummary
			for _, t := range tasks {
				list = append(list, taskSummary{
					ID:        t.ID,
					Command:   t.Command,
					Cwd:       t.Cwd,
					StartTime: t.StartTime.Format(time.RFC3339),
					Completed: t.Completed.Load(),
					ExitCode:  t.ExitCode,
				})
			}
			out, _ := json.MarshalIndent(list, "", "  ")
			return mcp.NewToolResultText(string(out)), nil

		case "status":
			if taskID == "" {
				return mcp.NewToolResultError("TaskId is required for 'status' action"), nil
			}
			t, ok := tm.Get(taskID)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("Task %s not found", taskID)), nil
			}
			status := "RUNNING"
			if t.Completed.Load() {
				status = "DONE"
			}
			res := map[string]any{
				"id":         t.ID,
				"command":    t.Command,
				"cwd":        t.Cwd,
				"status":     status,
				"start_time": t.StartTime.Format(time.RFC3339),
				"exit_code":  t.ExitCode,
				"stdout":     t.Stdout.String(),
				"stderr":     t.Stderr.String(),
			}
			out, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(out)), nil

		case "kill":
			if taskID == "" {
				return mcp.NewToolResultError("TaskId is required for 'kill' action"), nil
			}
			if err := tm.Kill(taskID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to kill task: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Task %s successfully terminated", taskID)), nil

		case "send_input":
			if taskID == "" {
				return mcp.NewToolResultError("TaskId is required for 'send_input' action"), nil
			}
			t, ok := tm.Get(taskID)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("Task %s not found", taskID)), nil
			}
			if t.Completed.Load() {
				return mcp.NewToolResultError(fmt.Sprintf("Task %s is already completed", taskID)), nil
			}
			if t.Stdin == nil {
				return mcp.NewToolResultError("Task stdin is not writable"), nil
			}
			_, err := t.Stdin.Write([]byte(input + "\n"))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to send input: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Sent input to task %s", taskID)), nil

		default:
			return mcp.NewToolResultError(fmt.Sprintf("Unknown action: %s", action)), nil
		}
	})

	// 3. remote_session_info
	sessionInfoTool := mcp.NewTool("remote_session_info",
		mcp.WithDescription("Get remote host environment information, current remote user, hostname, OS release, and persistent CWD."),
	)

	s.AddTool(sessionInfoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := client.RunCommand("uname -a && cat /etc/os-release 2>/dev/null | head -n 5 && whoami && hostname", client.GetCwd(), false, 5000)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get session info: %v", err)), nil
		}
		info := map[string]any{
			"current_cwd": client.GetCwd(),
			"remote_info": strings.TrimSpace(res.Stdout),
		}
		out, _ := json.MarshalIndent(info, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	})
}

func registerFileTools(s *server.MCPServer, client *sshclient.Client) {
	// 4. remote_view_file
	viewFileTool := mcp.NewTool("remote_view_file",
		mcp.WithDescription("View contents of a remote file with line numbers, line slicing (StartLine/EndLine), and token-safe truncation via SFTP."),
		mcp.WithString("AbsolutePath", mcp.Required(), mcp.Description("Remote file path (absolute or relative to current CWD).")),
		mcp.WithInteger("StartLine", mcp.Description("1-indexed starting line number.")),
		mcp.WithInteger("EndLine", mcp.Description("1-indexed ending line number.")),
		mcp.WithInteger("MaxBytes", mcp.Description("Maximum byte size to return (defaults to 46080 bytes).")),
	)

	s.AddTool(viewFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := request.RequireString("AbsolutePath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		startLine := request.GetInt("StartLine", 1)
		endLine := request.GetInt("EndLine", 0)
		maxBytes := request.GetInt("MaxBytes", 46080)

		res, err := client.ViewFile(path, startLine, endLine, 0, maxBytes)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to view file: %v", err)), nil
		}

		out, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	})

	// 5. remote_replace_file_content
	replaceFileTool := mcp.NewTool("remote_replace_file_content",
		mcp.WithDescription("Surgically replace a contiguous block of text in a remote file. Binary-safe and atomic via SFTP."),
		mcp.WithString("TargetFile", mcp.Required(), mcp.Description("Path to remote file.")),
		mcp.WithString("TargetContent", mcp.Required(), mcp.Description("Exact text block to replace.")),
		mcp.WithString("ReplacementContent", mcp.Required(), mcp.Description("Replacement text block.")),
		mcp.WithInteger("StartLine", mcp.Description("Optional starting line number hint.")),
		mcp.WithInteger("EndLine", mcp.Description("Optional ending line number hint.")),
		mcp.WithBoolean("AllowMultiple", mcp.Description("Allow multiple occurrences to be replaced.")),
	)

	s.AddTool(replaceFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		targetFile, err := request.RequireString("TargetFile")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		targetContent, err := request.RequireString("TargetContent")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		replacementContent, err := request.RequireString("ReplacementContent")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		startLine := request.GetInt("StartLine", 0)
		endLine := request.GetInt("EndLine", 0)
		allowMultiple := request.GetBool("AllowMultiple", false)

		res, err := client.ReplaceFileContent(targetFile, targetContent, replacementContent, startLine, endLine, allowMultiple)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to replace file content: %v", err)), nil
		}

		out, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	})

	// 6. remote_write_file
	writeFileTool := mcp.NewTool("remote_write_file",
		mcp.WithDescription("Create a new file or overwrite an existing file on the remote host via SFTP. Automatically creates parent directories."),
		mcp.WithString("TargetFile", mcp.Required(), mcp.Description("Remote file path to write to.")),
		mcp.WithString("CodeContent", mcp.Required(), mcp.Description("Text/Code contents to write.")),
		mcp.WithBoolean("Overwrite", mcp.Required(), mcp.Description("Set to true to overwrite existing file.")),
	)

	s.AddTool(writeFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		targetFile, err := request.RequireString("TargetFile")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		codeContent, err := request.RequireString("CodeContent")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		overwrite, err := request.RequireBool("Overwrite")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		err = client.WriteFile(targetFile, []byte(codeContent), overwrite)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to write file: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully wrote %d bytes to %s", len(codeContent), targetFile)), nil
	})

	// 7. remote_list_dir
	listDirTool := mcp.NewTool("remote_list_dir",
		mcp.WithDescription("List directory contents on remote host via SFTP with file size, permissions, and modification times."),
		mcp.WithString("DirectoryPath", mcp.Required(), mcp.Description("Remote directory path to list.")),
	)

	s.AddTool(listDirTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dirPath, err := request.RequireString("DirectoryPath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		entries, err := client.ListDir(dirPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list directory: %v", err)), nil
		}

		out, _ := json.MarshalIndent(entries, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	})
}

func registerSearchTools(s *server.MCPServer, client *sshclient.Client) {
	// 8. remote_grep_search
	grepTool := mcp.NewTool("remote_grep_search",
		mcp.WithDescription("Search for text or regex patterns across remote files. Uses remote 'rg' (ripgrep) if available, falling back to 'grep -rn'."),
		mcp.WithString("SearchPath", mcp.Required(), mcp.Description("Remote path or directory to search within.")),
		mcp.WithString("Query", mcp.Required(), mcp.Description("Search pattern or regex.")),
		mcp.WithBoolean("IsRegex", mcp.Description("Whether query is a regex.")),
		mcp.WithBoolean("CaseInsensitive", mcp.Description("Perform case-insensitive search.")),
		mcp.WithArray("Includes", mcp.Description("File glob patterns to include (e.g. ['*.go', '*.rs']).")),
	)

	s.AddTool(grepTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchPath, err := request.RequireString("SearchPath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		query, err := request.RequireString("Query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		isRegex := request.GetBool("IsRegex", false)
		caseInsensitive := request.GetBool("CaseInsensitive", false)
		includes := request.GetStringSlice("Includes", nil)

		// Build fast shell command that checks for rg first
		var cmd strings.Builder
		cmd.WriteString("if command -v rg >/dev/null 2>&1; then rg --max-columns 500 -n")
		if caseInsensitive {
			cmd.WriteString(" -i")
		}
		if !isRegex {
			cmd.WriteString(" -F")
		}
		for _, inc := range includes {
			cmd.WriteString(fmt.Sprintf(" --glob %q", inc))
		}
		cmd.WriteString(fmt.Sprintf(" %q %q", query, searchPath))

		cmd.WriteString("; else grep -rn")
		if caseInsensitive {
			cmd.WriteString(" -i")
		}
		if !isRegex {
			cmd.WriteString(" -F")
		}
		for _, inc := range includes {
			cmd.WriteString(fmt.Sprintf(" --include=%q", inc))
		}
		cmd.WriteString(fmt.Sprintf(" %q %q", query, searchPath))
		cmd.WriteString("; fi | head -n 100")

		res, err := client.RunCommand(cmd.String(), client.GetCwd(), false, 10000)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Search error: %v", err)), nil
		}

		return mcp.NewToolResultText(res.Stdout), nil
	})

	// 9. remote_find_by_name
	findTool := mcp.NewTool("remote_find_by_name",
		mcp.WithDescription("Find files and directories by name pattern on remote host. Uses 'fd' if available, falling back to 'find'."),
		mcp.WithString("SearchDirectory", mcp.Required(), mcp.Description("Remote directory to search within.")),
		mcp.WithString("Pattern", mcp.Required(), mcp.Description("Filename pattern or glob (e.g. '*.nix', 'main.*').")),
		mcp.WithString("Type", mcp.Description("Filter by type: 'file', 'directory', or 'any'.")),
		mcp.WithInteger("MaxDepth", mcp.Description("Maximum search depth.")),
	)

	s.AddTool(findTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchDir, err := request.RequireString("SearchDirectory")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		pattern, err := request.RequireString("Pattern")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fileType := request.GetString("Type", "any")
		maxDepth := request.GetInt("MaxDepth", 0)

		var cmd strings.Builder
		cmd.WriteString("if command -v fd >/dev/null 2>&1; then fd")
		if maxDepth > 0 {
			cmd.WriteString(fmt.Sprintf(" --max-depth %d", maxDepth))
		}
		if fileType == "file" {
			cmd.WriteString(" -t f")
		} else if fileType == "directory" {
			cmd.WriteString(" -t d")
		}
		cmd.WriteString(fmt.Sprintf(" %q %q", pattern, searchDir))

		cmd.WriteString("; else find %q")
		if maxDepth > 0 {
			cmd.WriteString(fmt.Sprintf(" -maxdepth %d", maxDepth))
		}
		if fileType == "file" {
			cmd.WriteString(" -type f")
		} else if fileType == "directory" {
			cmd.WriteString(" -type d")
		}
		cmd.WriteString(fmt.Sprintf(" -name %q", pattern))
		cmd.WriteString("; fi | head -n 100")

		res, err := client.RunCommand(cmd.String(), client.GetCwd(), false, 10000)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Find error: %v", err)), nil
		}

		return mcp.NewToolResultText(res.Stdout), nil
	})
}
