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

func getParamString(req mcp.CallToolRequest, keys ...string) string {
	for _, k := range keys {
		if val := req.GetString(k, ""); val != "" {
			return val
		}
	}
	return ""
}

func getParamInt(req mcp.CallToolRequest, defaultValue int, keys ...string) int {
	args := req.GetArguments()
	for _, k := range keys {
		if _, ok := args[k]; ok {
			return req.GetInt(k, defaultValue)
		}
	}
	return defaultValue
}

func getParamBool(req mcp.CallToolRequest, defaultValue bool, keys ...string) bool {
	args := req.GetArguments()
	for _, k := range keys {
		if _, ok := args[k]; ok {
			return req.GetBool(k, defaultValue)
		}
	}
	return defaultValue
}

func getParamStringSlice(req mcp.CallToolRequest, keys ...string) []string {
	args := req.GetArguments()
	for _, k := range keys {
		if _, ok := args[k]; ok {
			return req.GetStringSlice(k, nil)
		}
	}
	return nil
}

func registerCommandTools(s *server.MCPServer, client *sshclient.Client) {
	// 0. remote_connect
	connectTool := mcp.NewTool("remote_connect",
		mcp.WithDescription("Connect to a remote SSH host dynamically. Resolves ~/.ssh/config aliases automatically."),
		mcp.WithString("host", mcp.Description("Remote SSH host, IP, or ~/.ssh/config alias (Required, alias: Host).")),
		mcp.WithString("Host", mcp.Description("Alias for host.")),
		mcp.WithInteger("port", mcp.Description("SSH port (default 22, alias: Port).")),
		mcp.WithInteger("Port", mcp.Description("Alias for port.")),
		mcp.WithString("user", mcp.Description("SSH username (alias: User).")),
		mcp.WithString("User", mcp.Description("Alias for user.")),
		mcp.WithString("keyPath", mcp.Description("Path to private key file (alias: KeyPath).")),
		mcp.WithString("KeyPath", mcp.Description("Alias for keyPath.")),
		mcp.WithString("password", mcp.Description("SSH password if not using key (alias: Password).")),
		mcp.WithString("Password", mcp.Description("Alias for password.")),
	)

	s.AddTool(connectTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		host := getParamString(request, "host", "Host")
		if host == "" {
			return mcp.NewToolResultError("host is required"), nil
		}
		port := getParamInt(request, 22, "port", "Port")
		user := getParamString(request, "user", "User")
		keyPath := getParamString(request, "keyPath", "KeyPath", "key_path")
		password := getParamString(request, "password", "Password")

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
		mcp.WithDescription("Execute a command on the remote host with persistent working directory (CWD) and optional background task management."),
		mcp.WithString("commandLine", mcp.Description("The exact command line string to execute on the remote machine (Required, aliases: command, CommandLine).")),
		mcp.WithString("CommandLine", mcp.Description("Alias for commandLine.")),
		mcp.WithString("command", mcp.Description("Alias for commandLine.")),
		mcp.WithString("cwd", mcp.Description("Optional remote working directory (alias: Cwd).")),
		mcp.WithString("Cwd", mcp.Description("Alias for cwd.")),
		mcp.WithBoolean("isDaemon", mcp.Description("Set to true for long-running support processes that should keep running in the background indefinitely (alias: IsDaemon).")),
		mcp.WithBoolean("IsDaemon", mcp.Description("Alias for isDaemon.")),
		mcp.WithInteger("waitMsBeforeAsync", mcp.Description("Milliseconds to wait for the command to finish before detaching to background (default 5000ms, alias: WaitMsBeforeAsync).")),
		mcp.WithInteger("WaitMsBeforeAsync", mcp.Description("Alias for waitMsBeforeAsync.")),
	)

	s.AddTool(runCmdTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cmd := getParamString(request, "commandLine", "CommandLine", "command", "command_line", "cmd")
		if cmd == "" {
			return mcp.NewToolResultError("commandLine (or command / CommandLine) is required"), nil
		}
		cwd := getParamString(request, "cwd", "Cwd")
		isDaemon := getParamBool(request, false, "isDaemon", "IsDaemon", "is_daemon", "daemon")
		waitMs := getParamInt(request, 5000, "waitMsBeforeAsync", "WaitMsBeforeAsync", "wait_ms_before_async", "timeout")

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
		mcp.WithString("action", mcp.Description("The action to perform: 'list', 'status', 'kill', or 'send_input' (Required, alias: Action).")),
		mcp.WithString("Action", mcp.Description("Alias for action.")),
		mcp.WithString("taskId", mcp.Description("Task ID (e.g. 'task-1'). Required for 'status', 'kill', and 'send_input' (alias: TaskId).")),
		mcp.WithString("TaskId", mcp.Description("Alias for taskId.")),
		mcp.WithString("input", mcp.Description("Input string to send to stdin of the task. Required when action is 'send_input' (alias: Input).")),
		mcp.WithString("Input", mcp.Description("Alias for input.")),
	)

	s.AddTool(manageTaskTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action := getParamString(request, "action", "Action")
		if action == "" {
			return mcp.NewToolResultError("action is required"), nil
		}
		taskID := getParamString(request, "taskId", "TaskId", "task_id", "id")
		input := getParamString(request, "input", "Input")

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
				return mcp.NewToolResultError("taskId is required for 'status' action"), nil
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
				return mcp.NewToolResultError("taskId is required for 'kill' action"), nil
			}
			if err := tm.Kill(taskID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to kill task: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Task %s successfully terminated", taskID)), nil

		case "send_input":
			if taskID == "" {
				return mcp.NewToolResultError("taskId is required for 'send_input' action"), nil
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
		mcp.WithDescription("View contents of a remote file with line numbers, line slicing (startLine/endLine), and token-safe truncation via SFTP."),
		mcp.WithString("absolutePath", mcp.Description("Remote file path (Required, aliases: path, AbsolutePath, filePath).")),
		mcp.WithString("AbsolutePath", mcp.Description("Alias for absolutePath.")),
		mcp.WithString("path", mcp.Description("Alias for absolutePath.")),
		mcp.WithInteger("startLine", mcp.Description("1-indexed starting line number (alias: StartLine).")),
		mcp.WithInteger("StartLine", mcp.Description("Alias for startLine.")),
		mcp.WithInteger("endLine", mcp.Description("1-indexed ending line number (alias: EndLine).")),
		mcp.WithInteger("EndLine", mcp.Description("Alias for endLine.")),
		mcp.WithInteger("maxBytes", mcp.Description("Maximum byte size to return (default 46080 bytes, alias: MaxBytes).")),
		mcp.WithInteger("MaxBytes", mcp.Description("Alias for maxBytes.")),
	)

	s.AddTool(viewFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := getParamString(request, "absolutePath", "AbsolutePath", "path", "filePath", "file_path")
		if path == "" {
			return mcp.NewToolResultError("absolutePath is required"), nil
		}
		startLine := getParamInt(request, 1, "startLine", "StartLine", "start_line")
		endLine := getParamInt(request, 0, "endLine", "EndLine", "end_line")
		maxBytes := getParamInt(request, 46080, "maxBytes", "MaxBytes", "max_bytes")

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
		mcp.WithString("targetFile", mcp.Description("Path to remote file (Required, aliases: TargetFile, path).")),
		mcp.WithString("TargetFile", mcp.Description("Alias for targetFile.")),
		mcp.WithString("targetContent", mcp.Description("Exact text block to replace (Required, aliases: TargetContent, oldText).")),
		mcp.WithString("TargetContent", mcp.Description("Alias for targetContent.")),
		mcp.WithString("replacementContent", mcp.Description("Replacement text block (Required, aliases: ReplacementContent, newText).")),
		mcp.WithString("ReplacementContent", mcp.Description("Alias for replacementContent.")),
		mcp.WithInteger("startLine", mcp.Description("Optional starting line number hint (alias: StartLine).")),
		mcp.WithInteger("StartLine", mcp.Description("Alias for startLine.")),
		mcp.WithInteger("endLine", mcp.Description("Optional ending line number hint (alias: EndLine).")),
		mcp.WithInteger("EndLine", mcp.Description("Alias for endLine.")),
		mcp.WithBoolean("allowMultiple", mcp.Description("Allow multiple occurrences to be replaced (alias: AllowMultiple).")),
		mcp.WithBoolean("AllowMultiple", mcp.Description("Alias for allowMultiple.")),
	)

	s.AddTool(replaceFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		targetFile := getParamString(request, "targetFile", "TargetFile", "path", "file")
		if targetFile == "" {
			return mcp.NewToolResultError("targetFile is required"), nil
		}
		targetContent := getParamString(request, "targetContent", "TargetContent", "oldText", "old_text")
		if targetContent == "" {
			return mcp.NewToolResultError("targetContent is required"), nil
		}
		replacementContent := getParamString(request, "replacementContent", "ReplacementContent", "newText", "new_text")
		startLine := getParamInt(request, 0, "startLine", "StartLine", "start_line")
		endLine := getParamInt(request, 0, "endLine", "EndLine", "end_line")
		allowMultiple := getParamBool(request, false, "allowMultiple", "AllowMultiple", "allow_multiple")

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
		mcp.WithString("targetFile", mcp.Description("Remote file path to write to (Required, aliases: TargetFile, path).")),
		mcp.WithString("TargetFile", mcp.Description("Alias for targetFile.")),
		mcp.WithString("codeContent", mcp.Description("Text/Code contents to write (Required, aliases: CodeContent, content).")),
		mcp.WithString("CodeContent", mcp.Description("Alias for codeContent.")),
		mcp.WithString("content", mcp.Description("Alias for codeContent.")),
		mcp.WithBoolean("overwrite", mcp.Description("Set to true to overwrite existing file (alias: Overwrite).")),
		mcp.WithBoolean("Overwrite", mcp.Description("Alias for overwrite.")),
	)

	s.AddTool(writeFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		targetFile := getParamString(request, "targetFile", "TargetFile", "path", "file")
		if targetFile == "" {
			return mcp.NewToolResultError("targetFile is required"), nil
		}
		codeContent := getParamString(request, "codeContent", "CodeContent", "content")
		overwrite := getParamBool(request, false, "overwrite", "Overwrite")

		err := client.WriteFile(targetFile, []byte(codeContent), overwrite)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to write file: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully wrote %d bytes to %s", len(codeContent), targetFile)), nil
	})

	// 7. remote_list_dir
	listDirTool := mcp.NewTool("remote_list_dir",
		mcp.WithDescription("List directory contents on remote host via SFTP with file size, permissions, and modification times."),
		mcp.WithString("directoryPath", mcp.Description("Remote directory path to list (Required, aliases: DirectoryPath, path).")),
		mcp.WithString("DirectoryPath", mcp.Description("Alias for directoryPath.")),
		mcp.WithString("path", mcp.Description("Alias for directoryPath.")),
	)

	s.AddTool(listDirTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dirPath := getParamString(request, "directoryPath", "DirectoryPath", "path", "dir_path")
		if dirPath == "" {
			return mcp.NewToolResultError("directoryPath is required"), nil
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
		mcp.WithString("searchPath", mcp.Description("Remote path or directory to search within (Required, aliases: SearchPath, path).")),
		mcp.WithString("SearchPath", mcp.Description("Alias for searchPath.")),
		mcp.WithString("query", mcp.Description("Search pattern or regex (Required, alias: Query).")),
		mcp.WithString("Query", mcp.Description("Alias for query.")),
		mcp.WithBoolean("isRegex", mcp.Description("Whether query is a regex (alias: IsRegex).")),
		mcp.WithBoolean("IsRegex", mcp.Description("Alias for isRegex.")),
		mcp.WithBoolean("caseInsensitive", mcp.Description("Perform case-insensitive search (alias: CaseInsensitive).")),
		mcp.WithBoolean("CaseInsensitive", mcp.Description("Alias for caseInsensitive.")),
		mcp.WithArray("includes", mcp.Description("File glob patterns to include (e.g. ['*.go', '*.rs'], alias: Includes).")),
		mcp.WithArray("Includes", mcp.Description("Alias for includes.")),
	)

	s.AddTool(grepTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchPath := getParamString(request, "searchPath", "SearchPath", "path")
		if searchPath == "" {
			return mcp.NewToolResultError("searchPath is required"), nil
		}
		query := getParamString(request, "query", "Query")
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		isRegex := getParamBool(request, false, "isRegex", "IsRegex", "is_regex")
		caseInsensitive := getParamBool(request, false, "caseInsensitive", "CaseInsensitive", "case_insensitive")
		includes := getParamStringSlice(request, "includes", "Includes")

		// Build fast shell command that checks for rg first
		var cmd strings.Builder
		cmd.WriteString("if command -v rg >/dev/null 2>&1; then rg --hidden --max-columns 500 -n")
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

		outText := strings.TrimSpace(res.Stdout)
		if outText == "" {
			if strings.TrimSpace(res.Stderr) != "" {
				return mcp.NewToolResultError(fmt.Sprintf("Search completed with error: %s", strings.TrimSpace(res.Stderr))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("No matches found for %q in %s", query, searchPath)), nil
		}

		return mcp.NewToolResultText(outText), nil
	})

	// 9. remote_find_by_name
	findTool := mcp.NewTool("remote_find_by_name",
		mcp.WithDescription("Find files and directories by name pattern on remote host. Uses 'fd' if available, falling back to 'find'."),
		mcp.WithString("searchDirectory", mcp.Description("Remote directory to search within (Required, aliases: SearchDirectory, path, dir).")),
		mcp.WithString("SearchDirectory", mcp.Description("Alias for searchDirectory.")),
		mcp.WithString("pattern", mcp.Description("Filename pattern or glob (e.g. '*.nix', 'main.*', Required, alias: Pattern).")),
		mcp.WithString("Pattern", mcp.Description("Alias for pattern.")),
		mcp.WithString("type", mcp.Description("Filter by type: 'file', 'directory', or 'any' (alias: Type).")),
		mcp.WithString("Type", mcp.Description("Alias for type.")),
		mcp.WithInteger("maxDepth", mcp.Description("Maximum search depth (alias: MaxDepth).")),
		mcp.WithInteger("MaxDepth", mcp.Description("Alias for maxDepth.")),
	)

	s.AddTool(findTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchDir := getParamString(request, "searchDirectory", "SearchDirectory", "path", "dir")
		if searchDir == "" {
			return mcp.NewToolResultError("searchDirectory is required"), nil
		}
		pattern := getParamString(request, "pattern", "Pattern")
		if pattern == "" {
			return mcp.NewToolResultError("pattern is required"), nil
		}
		fileType := getParamString(request, "type", "Type")
		if fileType == "" {
			fileType = "any"
		}
		maxDepth := getParamInt(request, 0, "maxDepth", "MaxDepth", "max_depth")

		var cmd strings.Builder
		cmd.WriteString("if command -v fd >/dev/null 2>&1; then fd -H -I -g")
		if maxDepth > 0 {
			cmd.WriteString(fmt.Sprintf(" --max-depth %d", maxDepth))
		}
		if fileType == "file" {
			cmd.WriteString(" -t f")
		} else if fileType == "directory" {
			cmd.WriteString(" -t d")
		}
		cmd.WriteString(fmt.Sprintf(" %q %q", pattern, searchDir))

		cmd.WriteString(fmt.Sprintf("; else find %q", searchDir))
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

		outText := strings.TrimSpace(res.Stdout)
		if outText == "" {
			if strings.TrimSpace(res.Stderr) != "" {
				return mcp.NewToolResultError(fmt.Sprintf("Find completed with error: %s", strings.TrimSpace(res.Stderr))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("No files or directories matching %q found in %s", pattern, searchDir)), nil
		}

		return mcp.NewToolResultText(outText), nil
	})
}
