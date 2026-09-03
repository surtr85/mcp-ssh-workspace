package sshclient

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

type Tunnel struct {
	ID         string    `json:"id"`
	LocalPort  int       `json:"local_port"`
	RemoteHost string    `json:"remote_host"`
	RemotePort int       `json:"remote_port"`
	CreatedAt  string    `json:"created_at"`
	listener   net.Listener
	closed     atomic.Bool
	stopChan   chan struct{}
}

type TunnelManager struct {
	mu        sync.RWMutex
	tunnels   map[string]*Tunnel
	nextID    atomic.Int64
	sshClient func() (*ssh.Client, error)
}

func NewTunnelManager(sshClientGetter func() (*ssh.Client, error)) *TunnelManager {
	return &TunnelManager{
		tunnels:   make(map[string]*Tunnel),
		sshClient: sshClientGetter,
	}
}

func (tm *TunnelManager) Open(localPort int, remoteHost string, remotePort int) (*Tunnel, error) {
	if remoteHost == "" {
		remoteHost = "127.0.0.1"
	}
	if remotePort <= 0 {
		return nil, fmt.Errorf("remotePort is required and must be > 0")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return nil, fmt.Errorf("failed to bind local port %d: %w", localPort, err)
	}

	assignedLocalPort := listener.Addr().(*net.TCPAddr).Port
	idNum := tm.nextID.Add(1)
	tunnelID := fmt.Sprintf("tun-%d", idNum)

	t := &Tunnel{
		ID:         tunnelID,
		LocalPort:  assignedLocalPort,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
		CreatedAt:  time.Now().Format(time.RFC3339),
		listener:   listener,
		stopChan:   make(chan struct{}),
	}

	tm.mu.Lock()
	tm.tunnels[tunnelID] = t
	tm.mu.Unlock()

	go tm.forwardLoop(t)

	return t, nil
}

func (tm *TunnelManager) forwardLoop(t *Tunnel) {
	defer t.listener.Close()

	for {
		localConn, err := t.listener.Accept()
		if err != nil {
			if t.closed.Load() {
				return
			}
			select {
			case <-t.stopChan:
				return
			default:
				continue
			}
		}

		go func(lConn net.Conn) {
			defer lConn.Close()

			sshCli, err := tm.sshClient()
			if err != nil {
				return
			}

			remoteConn, err := sshCli.Dial("tcp", fmt.Sprintf("%s:%d", t.RemoteHost, t.RemotePort))
			if err != nil {
				return
			}
			defer remoteConn.Close()

			done := make(chan struct{}, 2)
			go func() {
				_, _ = io.Copy(remoteConn, lConn)
				done <- struct{}{}
			}()
			go func() {
				_, _ = io.Copy(lConn, remoteConn)
				done <- struct{}{}
			}()

			<-done
		}(localConn)
	}
}

func (tm *TunnelManager) Close(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t, ok := tm.tunnels[id]
	if !ok {
		return fmt.Errorf("tunnel %s not found", id)
	}

	if t.closed.CompareAndSwap(false, true) {
		close(t.stopChan)
		_ = t.listener.Close()
	}
	delete(tm.tunnels, id)
	return nil
}

func (tm *TunnelManager) List() []*Tunnel {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	list := make([]*Tunnel, 0, len(tm.tunnels))
	for _, t := range tm.tunnels {
		list = append(list, t)
	}
	return list
}

func (tm *TunnelManager) CloseAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for id, t := range tm.tunnels {
		if t.closed.CompareAndSwap(false, true) {
			close(t.stopChan)
			_ = t.listener.Close()
		}
		delete(tm.tunnels, id)
	}
}
