package sshclient

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/surtr85/mcp-ssh-workspace/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type Client struct {
	cfg        *config.Config
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	taskMgr    *TaskManager
	mu         sync.Mutex
	cwdMu      sync.RWMutex
	currentCwd string
}

func NewClient(cfg *config.Config) (*Client, error) {
	c := &Client{
		cfg:        cfg,
		taskMgr:    NewTaskManager(),
		currentCwd: cfg.WorkDir,
	}

	if err := c.Connect(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var authMethods []ssh.AuthMethod

	// 1. SSH Agent
	if c.cfg.UseAgent {
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
			if agentConn, err := net.Dial("unix", sock); err == nil {
				ag := agent.NewClient(agentConn)
				authMethods = append(authMethods, ssh.PublicKeysCallback(ag.Signers))
			}
		}
	}

	// 2. Private Key
	if c.cfg.KeyPath != "" {
		keyBytes, err := os.ReadFile(c.cfg.KeyPath)
		if err == nil {
			signer, err := ssh.ParsePrivateKey(keyBytes)
			if err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	// 3. Password
	if c.cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(c.cfg.Password))
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication methods available (provide SSH key, password, or run ssh-agent)")
	}

	sshConfig := &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := c.cfg.Addr()
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	sftpCli, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to initialize SFTP subsystem: %w", err)
	}

	c.sshClient = client
	c.sftpClient = sftpCli

	// Determine initial remote CWD if not set
	if c.currentCwd == "" {
		if home, err := sftpCli.Getwd(); err == nil && home != "" {
			c.currentCwd = home
		} else {
			c.currentCwd = "/"
		}
	}

	return nil
}

func (c *Client) EnsureConnected() error {
	c.mu.Lock()
	needReconnect := c.sshClient == nil
	c.mu.Unlock()

	if needReconnect {
		return c.Connect()
	}

	// Quick check using SFTP Getwd
	c.mu.Lock()
	sftpCli := c.sftpClient
	c.mu.Unlock()

	if sftpCli == nil {
		return c.Connect()
	}

	if _, err := sftpCli.Getwd(); err != nil {
		// Connection lost, reconnect
		return c.Connect()
	}

	return nil
}

func (c *Client) GetCwd() string {
	c.cwdMu.RLock()
	defer c.cwdMu.RUnlock()
	return c.currentCwd
}

func (c *Client) SetCwd(newCwd string) {
	c.cwdMu.Lock()
	defer c.cwdMu.Unlock()
	c.currentCwd = newCwd
}

func (c *Client) TaskManager() *TaskManager {
	return c.taskMgr
}

func (c *Client) SFTP() (*sftp.Client, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}
	return c.sftpClient, nil
}

func (c *Client) SSH() (*ssh.Client, error) {
	if err := c.EnsureConnected(); err != nil {
		return nil, err
	}
	return c.sshClient, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sftpClient != nil {
		_ = c.sftpClient.Close()
	}
	if c.sshClient != nil {
		_ = c.sshClient.Close()
	}
}
