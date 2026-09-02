# 🚀 mcp-ssh-workspace

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Built with Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Nix Flake](https://img.shields.io/badge/Nix-Flake-5277C3?logo=nixos&logoColor=white)](flake.nix)
[![MCP Protocol](https://img.shields.io/badge/MCP-Compliant-8A2BE2)](https://modelcontextprotocol.io)

An ultra-fast, native-like **SSH Workspace Model Context Protocol (MCP)** server tailored specifically for AI coding assistants (Claude Desktop, Cursor, Antigravity, Cline, and custom agents).

Instead of treating your remote machine like a dumb, stateless `ssh exec` target, **`mcp-ssh-workspace`** exposes the **exact workspace primitives** that modern AI coding agents need to navigate, inspect, edit, and run code seamlessly on remote servers—as if it were their local host.

---

## ⚡ Why `mcp-ssh-workspace`?

Existing SSH MCP servers suffer from critical flaws when paired with AI agents:

| Feature | Generic SSH MCP Servers | `mcp-ssh-workspace` |
| :--- | :--- | :--- |
| **Shell State (`cwd` & env)** | ❌ **Stateless:** Every command runs in a new shell. `cd /path` vanishes. | ✅ **Persistent:** Session tracks `cwd` and preserves state across calls. |
| **File Reading** | ❌ Dumps entire files with `cat` (blows context limits). | ✅ **Line-slicing:** `StartLine`, `EndLine`, byte offsets, and pagination. |
| **File Editing** | ❌ Fragile bash `sed`/`cat << EOF` that breaks on quotes/escapes. | ✅ **Surgical replacement:** Exact chunk search-and-replace via atomic SFTP. |
| **Long-Running Commands** | ❌ Hangs and times out on background daemons or dev servers. | ✅ **Background tasks:** Async execution with `TaskId`, `kill`, and `send_input`. |
| **Latency** | ❌ Re-authenticates and negotiates SSH handshake on every call (~1s). | ✅ **Connection pooling:** Persistent multiplexed SSH & SFTP session (<10ms). |
| **Remote Footprint** | ❌ Requires Node/Python or custom daemons on remote. | ✅ **Zero footprint:** Requires only standard OpenSSH and SFTP. |

---

## 🛠️ Provided Tools

`mcp-ssh-workspace` exposes 9 specialized tools to the AI agent:

### 1. Terminal & Command Execution
- **`remote_run_command`**: Run any command on the remote host. Respects the session working directory, returns clean `stdout`/`stderr` separation, real exit codes, and can automatically detach into a background task if execution exceeds `WaitMsBeforeAsync`.
- **`remote_manage_task`**: Manage long-running remote processes (`list`, `status`, `kill`, `send_input` to stdin).
- **`remote_session_info`**: Retrieve OS information (`uname`, `/etc/os-release`), hostname, current user, and active `cwd`.

### 2. File Manipulation (SFTP-Powered)
- **`remote_view_file`**: Read remote files with line numbers, custom range (`StartLine` to `EndLine`), and token-safe byte caps.
- **`remote_replace_file_content`**: Atomically search and replace a chunk of text in a remote file without rewriting the entire file.
- **`remote_write_file`**: Write or overwrite remote files directly with automatic recursive directory creation (`mkdir -p`).
- **`remote_list_dir`**: Browse remote directories with file sizes, permissions, and modification timestamps.

### 3. Fast Code Search
- **`remote_grep_search`**: Search for code patterns or regular expressions. Automatically uses `ripgrep` (`rg`) if installed on the remote machine, with fallback to `grep -rn`.
- **`remote_find_by_name`**: Find files matching glob patterns or names. Automatically uses `fd` if available, with fallback to `find`.

---

## 📦 Installation & Quick Start

### Option 1: Run with Nix (Zero-Install)

You can run `mcp-ssh-workspace` directly without installing any Go toolchain:

```bash
# Connect using host defined in ~/.ssh/config
nix run github:surtr85/mcp-ssh-workspace -- --host myserver

# Or specify user, host, and private key
nix run github:surtr85/mcp-ssh-workspace -- --host 192.168.1.100 --user ubuntu --key ~/.ssh/id_ed25519
```

### Option 2: Build from Source

```bash
git clone https://github.com/surtr85/mcp-ssh-workspace.git
cd mcp-ssh-workspace
go build -o mcp-ssh-workspace ./cmd/mcp-ssh-workspace
```

---

## ⚙️ Configuration

### CLI Flags

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `--host`, `-H` | `SSH_HOST` | *(Required)* | Remote host IP, domain, or `~/.ssh/config` alias |
| `--user`, `-u` | `SSH_USER` | Current `$USER` | Remote SSH user |
| `--port`, `-p` | `SSH_PORT` | `22` | Remote SSH port |
| `--key`, `-i` | `SSH_KEY` | Auto-detected | Path to private SSH key (`~/.ssh/id_ed25519`, etc.) |
| `--password` | `SSH_PASSWORD` | - | SSH password (if not using key) |
| `--workdir`, `-w`| `SSH_WORKDIR` | Remote Home | Initial remote working directory |
| `--agent` | - | `true` | Use SSH agent (`$SSH_AUTH_SOCK`) if available |

> 💡 **Tip:** If you have hosts configured in `~/.ssh/config` (including aliases, identity files, and ports), `mcp-ssh-workspace` will automatically read and apply them!

---

## 🤖 Client Setup

### Claude Desktop
Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "remote-server": {
      "command": "mcp-ssh-workspace",
      "args": [
        "--host", "my-server-alias",
        "--workdir", "/var/www/my-project"
      ]
    }
  }
}
```

Or using `nix run`:
```json
{
  "mcpServers": {
    "remote-server": {
      "command": "nix",
      "args": [
        "run",
        "github:surtr85/mcp-ssh-workspace",
        "--",
        "--host", "my-server-alias"
      ]
    }
  }
}
```

### Antigravity / Cursor
In your `mcp.json`:
```json
{
  "mcpServers": {
    "remote-workspace": {
      "command": "/path/to/mcp-ssh-workspace",
      "args": ["--host", "prod-server", "--user", "deploy"]
    }
  }
}
```

---

## 🔒 Security Best Practices

- Use **SSH Key Authentication** or SSH Agent forwarding rather than hardcoding passwords.
- It is recommended to connect with an unprivileged remote user account for agent operations.
- The server validates paths and uses atomic SFTP file writes to prevent corrupted files.

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
