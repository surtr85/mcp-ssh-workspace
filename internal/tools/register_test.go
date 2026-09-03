package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestParameterResolvers(t *testing.T) {
	// Test getParamString with camelCase vs PascalCase
	reqCamel := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"commandLine": "echo hello",
				"cwd":         "/tmp",
				"isDaemon":    true,
				"waitMs":      1000,
			},
		},
	}
	if got := getParamString(reqCamel, "commandLine", "CommandLine", "command"); got != "echo hello" {
		t.Errorf("expected 'echo hello', got %q", got)
	}
	if got := getParamString(reqCamel, "cwd", "Cwd"); got != "/tmp" {
		t.Errorf("expected '/tmp', got %q", got)
	}
	if got := getParamBool(reqCamel, false, "isDaemon", "IsDaemon"); got != true {
		t.Errorf("expected true, got %v", got)
	}

	reqPascal := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"CommandLine": "echo world",
				"Cwd":         "/var",
				"IsDaemon":    false,
			},
		},
	}
	if got := getParamString(reqPascal, "commandLine", "CommandLine", "command"); got != "echo world" {
		t.Errorf("expected 'echo world', got %q", got)
	}
	if got := getParamString(reqPascal, "cwd", "Cwd"); got != "/var" {
		t.Errorf("expected '/var', got %q", got)
	}
	if got := getParamBool(reqPascal, true, "isDaemon", "IsDaemon"); got != false {
		t.Errorf("expected false, got %v", got)
	}
}
