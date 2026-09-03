package sshclient

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

type Task struct {
	ID        string
	Command   string
	Cwd       string
	StartTime time.Time
	Session   *ssh.Session
	Stdin     io.WriteCloser
	Stdout    *syncBuffer
	Stderr    *syncBuffer
	DoneChan  chan struct{}
	ExitCode  int
	Completed atomic.Bool
	ErrorMsg  string
}

type syncBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func newSyncBuffer() *syncBuffer {
	return &syncBuffer{}
}

func (s *syncBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buf.String()
}

func (s *syncBuffer) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buf.Len()
}

type TaskManager struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	nextID atomic.Int64
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*Task),
	}
}

func (tm *TaskManager) Register(cmd, cwd string, sess *ssh.Session, stdin io.WriteCloser, stdout, stderr *syncBuffer) *Task {
	idNum := tm.nextID.Add(1)
	taskID := fmt.Sprintf("task-%d", idNum)

	t := &Task{
		ID:        taskID,
		Command:   cmd,
		Cwd:       cwd,
		StartTime: time.Now(),
		Session:   sess,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		DoneChan:  make(chan struct{}),
		ExitCode:  -1,
	}

	tm.mu.Lock()
	tm.tasks[taskID] = t
	tm.mu.Unlock()

	return t
}

func (tm *TaskManager) Get(id string) (*Task, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.tasks[id]
	return t, ok
}

func (tm *TaskManager) List() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	list := make([]*Task, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		list = append(list, t)
	}
	return list
}

func (tm *TaskManager) Kill(id string) error {
	t, ok := tm.Get(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if t.Completed.Load() {
		return fmt.Errorf("task %s already finished", id)
	}
	if t.Session != nil {
		_ = t.Session.Signal(ssh.SIGKILL)
		_ = t.Session.Close()
	}
	return nil
}

func (t *Task) Tail(lines int) (stdoutTail, stderrTail string) {
	if lines <= 0 {
		lines = 50
	}
	stdoutTail = tailLines(t.Stdout.String(), lines)
	stderrTail = tailLines(t.Stderr.String(), lines)
	return stdoutTail, stderrTail
}

func tailLines(s string, n int) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) <= n {
		return s
	}
	return strings.Join(parts[len(parts)-n:], "\n") + "\n"
}
