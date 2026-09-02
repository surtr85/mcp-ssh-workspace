#!/usr/bin/env bash
LOG_DIR="/tmp/mcp-debug"
mkdir -p "$LOG_DIR"
exec /nix/store/fjdrq1lafjcxkgaw3qxas4r3ih4w3gr9-mcp-ssh-workspace-1.0.0/bin/mcp-ssh-workspace "$@" 2> >(tee -a "$LOG_DIR/stderr.log" >&2) < <(tee -a "$LOG_DIR/stdin.log") > >(tee -a "$LOG_DIR/stdout.log")
