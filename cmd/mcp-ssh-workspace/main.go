package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/surtr85/mcp-ssh-workspace/internal/config"
	"github.com/surtr85/mcp-ssh-workspace/internal/sshclient"
	"github.com/surtr85/mcp-ssh-workspace/internal/tools"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	client := sshclient.NewClient(cfg)
	defer client.Close()

	s := server.NewMCPServer(
		"mcp-ssh-workspace",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithDescription("Ultra-fast, native-like SSH workspace MCP server for AI coding agents"),
	)

	tools.RegisterTools(s, client)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server stopped: %v\n", err)
		os.Exit(1)
	}
}
