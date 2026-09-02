<div align="center">

# ⚡ mcp-ssh-workspace

### *The High-Performance, Native-Like SSH Workspace Engine for Autonomous AI Agents*

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![Nix Flake](https://img.shields.io/badge/Nix-Flake-5277C3?style=for-the-badge&logo=nixos&logoColor=white)](flake.nix)
[![MCP Compliant](https://img.shields.io/badge/MCP-2024--11--05%20%7C%202026--07--28-8A2BE2?style=for-the-badge)](https://modelcontextprotocol.io)
[![Zero Footprint](https://img.shields.io/badge/Remote-Zero--Footprint-success?style=for-the-badge)](https://github.com/surtr85/mcp-ssh-workspace)

<p align="center">
  <b>Transform remote servers into local-feeling coding environments for AI coding assistants.</b><br/>
  Persistent sessions • Multiplexed connection pooling • Surgical atomic edits • Token-capped line slicing • Universal shell-agnostic runner
</p>

---

[Key Features](#-why-mcp-ssh-workspace) • [Benchmarks](#-real-world-benchmarks) • [Architecture](#-architecture) • [The 11 Tools](#-the-11-agent-primitives) • [Quickstart](#-quickstart--installation) • [Client Setup](#-mcp-client-configurations) • [NixOS Integration](#-declarative-nixos--home-manager)

---

</div>

## 💡 Why `mcp-ssh-workspace`?

Existing SSH MCP implementations treat remote machines like dumb `ssh exec` targets:
- Every command spawns a fresh shell, **discarding `cd` and environment state**.
- Large files are dumped via `cat`, **instantly blowing LLM context budgets**.
- Edits rely on fragile bash `sed` or `cat << EOF` scripts that **corrupt files on quotes and escapes**.
- Remote login shells like `fish` or `zsh` break naive bash command assumptions.
- Background daemons and web servers **hang the agent indefinitely**.

**`mcp-ssh-workspace` was engineered from scratch in Go to eradicate every single one of these failure modes.**

### 📊 Feature Matrix

| Feature | Generic SSH MCP Solutions | ⚡ `mcp-ssh-workspace` |
| :--- | :--- | :--- |
| **Shell State (`cwd` & env)** | ❌ **Stateless:** Every call opens a new session. `cd /app` is immediately lost. | ✅ **Persistent Session:** Tracks `cwd` and maintains directory context seamlessly across calls. |
| **Network Overhead** | ❌ Re-authenticates & renegotiates SSH crypto on *every single* tool call (~1000ms). | ✅ **Multiplexed Pool:** Single long-lived SSH/SFTP connection (**<30ms** tool round-trip). |
| **File Reading** | ❌ Dumps entire files with `cat` (overflows model context tokens). | ✅ **Token-Capped Slicing:** Exact `StartLine`/`EndLine` ranges, line numbering, and byte caps. |
| **File Editing** | ❌ Fragile `sed` / `echo` scripts that mangle escapes, quotes, and indentation. | ✅ **Surgical Replacement:** Atomic chunk find-and-replace over binary SFTP. |
| **Long-Running Daemons** | ❌ Blocks and times out on dev servers (`npm run dev`, `cargo watch`). | ✅ **Async Task Supervisor:** Detaches into tracked background tasks (`status`, `kill`, `stdin`). |
| **Remote Host Setup** | ❌ Requires Python, Node.js runtime, or server-side agent daemons. | ✅ **Zero Footprint:** Requires only standard OpenSSH and SFTP on the remote host. |
| **Shell Compatibility** | ❌ Assumes standard bash; crashes if user's remote shell is `fish` or `csh`. | ✅ **Universal Shell Engine:** Base64-piped execution immune to remote login shell syntax. |
| **Host Connectivity** | ❌ Hardcoded host at boot; crashes if server is offline or unreachable. | ✅ **Dual Mode:** Static host via flags **OR** dynamic `remote_connect` in-flight. |

---

## 🔥 Brutal Stress-Test & Real-World Benchmarks

To validate production readiness under extreme conditions, `mcp-ssh-workspace` was subjected to a rigorous 6-tier stress test against a real production VPS over the public internet (**Void Linux x86_64, Kernel 6.18, Remote Shell: Fish**).

```text
================================================================================
       🔥 BRUTAL STRESS TEST & BENCHMARK: mcp-ssh-workspace v1.0.0 🔥         
                  Target: amadeus@ssh.surtr.ir (Void Linux)                     
================================================================================

[*] MCP Handshake:                        5.99 ms
[*] SSH Key Exchange & SFTP Multiplexing: 132.52 ms
    Status: Successfully connected to ssh.surtr.ir. CWD: /home/amadeus

[TEST 1] 🚀 High-Frequency Sequential Burst (50 RPC Round-Trips)...
    ✔ Total Requests: 50 | Errors: 0
    ✔ Min Latency:    22.29 ms
    ✔ Median Latency: 26.04 ms
    ✔ Mean Latency:   29.34 ms
    ✔ P95 Latency:    43.71 ms
    ✔ Max Latency:    52.27 ms
    ✔ Throughput:     34.1 commands/sec over WAN!

[TEST 2] 🔬 Adversarial Surgical Editing (Special chars, unicode, shell injections)...
    ✔ Surgical atomic chunk replace: 84.44 ms
    ✔ Line-sliced verification read: 24.55 ms
    ✔ Integrity Verified: Unicode, shell symbols, and regex chars preserved flawlessly!

[TEST 3] 📦 High-Volume SFTP Transfer & Cryptographic Checksum...
    ✔ Wrote 2.00 MB via SFTP in 0.96s (2.09 MB/s)
    ✔ Local SHA256:  b2e5e895620462e661c6a4f7d6833ad6b302063332330a0892f07104ce24366b
    ✔ Remote SHA256: b2e5e895620462e661c6a4f7d6833ad6b302063332330a0892f07104ce24366b
    ✔ 100% Zero-Loss Binary Integrity Verified!

[TEST 4] 🧭 Stateful Multi-Hop CWD Tracking...
    ✔ Navigated into: /tmp/deep_nest_test/alpha/beta/gamma
    ✔ Persistent session CWD: /tmp/deep_nest_test/alpha/beta/gamma
    ✔ Command executed inside retained CWD: /tmp/deep_nest_test/alpha/beta/gamma
    ✔ Verified relative filesystem state preserved!

[TEST 5] ⚙️ Daemon Process Lifecycle: Async Detach, Stdin Pipe & Termination...
    ✔ Daemon started & detached in 17.01 ms. TaskID: task-56
    ✔ Task Status: RUNNING | Stdout: 'DAEMON_READY'
    ✔ Piped input to remote daemon stdin in 0.23 ms
    ✔ Captured Daemon Response: 'DAEMON_READY\nECHO: PING_MESSAGE_HELLO_FROM_AI'
    ✔ Force-killed daemon process in 0.20 ms
    ✔ Final Task Status: DONE (Completed: True)

[TEST 6] 🧹 Zero-Footprint Remote Cleanup...
    ✔ Cleaned up all benchmark test artifacts.
    ✔ Cleanly closed multiplexed SSH/SFTP session.
================================================================================
             🏆 ALL BRUTAL STRESS TESTS PASSED WITH 100% INTEGRITY!            
================================================================================
```

### 🔬 What These Results Prove:

1. **Sub-30ms WAN Latency:** A single multiplexed SSH channel eliminates the 1000ms TCP+crypto handshake penalty on every tool call. AI agent commands run at **34+ ops/sec over the internet**, feeling identical to local tools.
2. **Zero-Loss Surgical Precision:** Even when code chunks contain dangerous shell metacharacters (`$(rm -rf /)`, backticks, `${VAR:-default}`, regex delimiters, and Persian/Unicode characters), the atomic SFTP pipeline preserves files byte-for-byte without escaping bugs.
3. **True Background Daemons:** Background tasks (`isDaemon: true`) detach in **17ms**, run independently, accept live interactive `stdin` input in **0.23ms**, and terminate cleanly on command.
4. **Universal Shell Agnostic:** Works flawlessly even when the remote user's default login shell is non-POSIX (like `fish` on Void Linux), thanks to subshell-isolated Base64 execution.

---

## 🏗️ Architecture

```mermaid
flowchart TD
    subgraph ClientLayer["🖥️ Local Host / Agent Environment"]
        Agent["🤖 AI Coding Assistant<br/>(Claude / Antigravity / Cursor / Cline / Zed)"]
        Config["🔑 Local SSH Assets<br/>(~/.ssh/config • ~/.ssh/id_* • SSH-Agent)"]
        Server["⚡ mcp-ssh-workspace<br/>(Lightweight Go Daemon)"]

        Agent <== "MCP JSON-RPC Protocol (stdio)" ==> Server
        Config -. "Auto-resolves aliases & keys" .-> Server
    end

    subgraph Multiplex["🔒 Encrypted SSH Tunnel (Single Persistent TCP Socket)"]
        Tunnel["Persistent SSH Transport Layer (<30ms RTT)"]
    end

    subgraph RemoteLayer["☁️ Remote Server (Any POSIX / Linux / BSD / Cloud VPS)"]
        SSHD["OpenSSH Daemon (:22)"]
        
        subgraph Channels["Multiplexed Subsystems"]
            SFTP["📁 SFTP Subsystem<br/>• Atomic file writes<br/>• Chunk replacements<br/>• Sliced reads<br/>• Fast directory listings"]
            Exec["🐚 Universal Shell Engine<br/>• Base64-piped runner<br/>• Stateful CWD tracking<br/>• Agnostic to fish/zsh/bash"]
            Supervisor["⚙️ Process Supervisor<br/>• Background Daemons<br/>• PID Tracking<br/>• Stdin interactive pipe"]
        end
    end

    Server <== "Connection Pool" ==> Tunnel
    Tunnel <== "Subsystem multiplexing" ==> SSHD
    SSHD --> SFTP
    SSHD --> Exec
    SSHD --> Supervisor
```

---

## 🛠️ The 11 Agent Primitives

`mcp-ssh-workspace` exposes 11 surgical tools specifically tailored to LLM reasoning:

### 1. Connection & Session Lifecycle
- **`remote_connect`**: Dynamically connect or switch between remote hosts at runtime. Auto-resolves hosts, ports, identity files, and proxies from `~/.ssh/config`.
- **`remote_disconnect`**: Cleanly terminate active SSH and SFTP channels.
- **`remote_session_info`**: Fetch remote host environment metadata (`/etc/os-release`, `uname -mrs`, active user, and persistent `cwd`).

### 2. Terminal & Process Supervision
- **`remote_run_command`**: Execute bash commands with clean `stdout`/`stderr` separation, exit code capture, and persistent directory retention.
- **`remote_manage_task`**: Manage long-running daemons and dev servers (`action: "list" | "status" | "kill" | "send_input"`).

### 3. Surgical SFTP File Operations
- **`remote_view_file`**: Read files with token-safe line ranges (`StartLine` to `EndLine`), line numbering, and byte budget protections.
- **`remote_replace_file_content`**: Surgically replace an exact code block without rewriting or risking whole-file corruption.
- **`remote_write_file`**: Atomically create or overwrite remote files with automatic recursive directory creation (`mkdir -p`).
- **`remote_list_dir`**: Inspect remote directory listings with exact byte sizes, POSIX permissions, and modification timestamps.

### 4. High-Performance Code Search
- **`remote_grep_search`**: Fast regex search across the remote workspace (automatically uses `rg` if present, fallback to `grep -rn`).
- **`remote_find_by_name`**: Fast file and directory glob finding (automatically uses `fd` if present, fallback to `find`).

---

## 🚀 Quickstart & Installation

### Option 1: Zero-Install via Nix (Recommended)

Run instantly without installing Go or compiler toolchains:

```bash
# Connect using an alias from ~/.ssh/config:
nix run github:surtr85/mcp-ssh-workspace -- --host my-vps

# Or specify user and host directly:
nix run github:surtr85/mcp-ssh-workspace -- --host 192.168.1.50 --user ubuntu

# Or start in Dynamic Mode (let the AI agent connect when needed):
nix run github:surtr85/mcp-ssh-workspace
```

### Option 2: Install via Go

```bash
go install github.com/surtr85/mcp-ssh-workspace/cmd/mcp-ssh-workspace@latest
```

### Option 3: Build from Source

```bash
git clone https://github.com/surtr85/mcp-ssh-workspace.git
cd mcp-ssh-workspace
go build -o mcp-ssh-workspace ./cmd/mcp-ssh-workspace
```

---

## ⚙️ Configuration & Flags

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `--host`, `-H` | `SSH_HOST` | `""` | Remote host, IP, or `~/.ssh/config` alias *(optional on boot)* |
| `--user`, `-u` | `SSH_USER` | Current `$USER` | Remote SSH username |
| `--port`, `-p` | `SSH_PORT` | `22` | Remote SSH port (or auto-resolved from ssh config) |
| `--key`, `-i` | `SSH_KEY` | Auto-detected | Path to private SSH key (`~/.ssh/id_ed25519`, etc.) |
| `--password` | `SSH_PASSWORD` | `""` | SSH password (if not using key authentication) |
| `--workdir`, `-w`| `SSH_WORKDIR` | Remote Home | Initial remote working directory |
| `--agent` | - | `true` | Use local SSH agent (`$SSH_AUTH_SOCK`) |

> 💡 **Dynamic Mode:** If `--host` is omitted at startup, `mcp-ssh-workspace` boots in **Dynamic Mode**, allowing the AI agent to connect to any host on-demand using `remote_connect`.

---

## 🤖 MCP Client Configurations

### Claude Desktop
Add to `~/.config/Claude/claude_desktop_config.json` (Linux) or `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS):

```json
{
  "mcpServers": {
    "ssh-workspace": {
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

### Google Antigravity
Add to `~/.gemini/config/mcp_config.json`:

```json
{
  "mcpServers": {
    "ssh-workspace": {
      "command": "mcp-ssh-workspace"
    }
  }
}
```

### Cursor / Cline / Zed
Add to your client's MCP configuration settings:

```json
{
  "mcpServers": {
    "ssh-workspace": {
      "command": "mcp-ssh-workspace",
      "args": ["--host", "prod-box", "--workdir", "/var/www/app"]
    }
  }
}
```

---

## ❄️ Declarative NixOS & Home-Manager

Integrate directly into your declarative NixOS Flake configuration:

```nix
# flake.nix
{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    mcp-ssh-workspace = {
      url = "github:surtr85/mcp-ssh-workspace";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, mcp-ssh-workspace, ... }: {
    # In your Home-Manager configuration:
    homeManagerConfiguration = {
      modules = [
        ({ pkgs, ... }: {
          home.packages = [
            mcp-ssh-workspace.packages.${pkgs.system}.default
          ];

          home.file.".gemini/config/mcp_config.json".text = builtins.toJSON {
            mcpServers = {
              ssh-workspace = {
                command = "${mcp-ssh-workspace.packages.${pkgs.system}.default}/bin/mcp-ssh-workspace";
              };
            };
          };
        })
      ];
    };
  };
}
```

---

## 🔒 Security Principles

- **Zero Unencrypted Passwords:** Defaults to public-key cryptography (`Ed25519` / `RSA`) and local SSH Agent forwarding.
- **Surgical Atomic Swapping:** File edits are staged in temporary files and swapped using atomic SFTP renames, completely preventing truncated or corrupted remote files.
- **Context Budget Protection:** Hard bounds on directory listing depths, grep results, and file slice reads prevent denial-of-service against LLM context windows.

---

## 📄 License

MIT License © 2026 [surtr85](https://github.com/surtr85). Distributed under the [MIT License](LICENSE).
