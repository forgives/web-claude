package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
)

const historyLimit = 4 << 20

type Attachment struct {
	Snapshot []byte
	Updates  <-chan []byte
	Cancel   func()
	Running  bool
}

type Manager struct {
	workingDir         string
	command            string
	commandArgs        []string
	restartOnReconnect bool

	mu       sync.Mutex
	sessions map[string]*Session
}

type Session struct {
	workingDir  string
	command     string
	commandArgs []string

	mu          sync.RWMutex
	cmd         *exec.Cmd
	ptmx        *os.File
	running     bool
	history     []byte
	subscribers map[chan []byte]struct{}
}

func NewManager(workingDir, command string, commandArgs []string, restartOnReconnect bool) *Manager {
	return &Manager{
		workingDir:         workingDir,
		command:            command,
		commandArgs:        append([]string(nil), commandArgs...),
		restartOnReconnect: restartOnReconnect,
		sessions:           make(map[string]*Session),
	}
}

func (m *Manager) Attach(id string, cols, rows int) (*Attachment, error) {
	session := m.get(id)
	if err := session.EnsureRunning(cols, rows, m.restartOnReconnect); err != nil {
		return nil, err
	}
	snapshot, running := session.Snapshot()
	updates, cancel := session.Subscribe()
	return &Attachment{
		Snapshot: snapshot,
		Updates:  updates,
		Cancel:   cancel,
		Running:  running,
	}, nil
}

func (m *Manager) Input(id string, data []byte) error {
	return m.get(id).Input(data)
}

func (m *Manager) Resize(id string, cols, rows int) error {
	return m.get(id).Resize(cols, rows)
}

func (m *Manager) get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[id]
	if session == nil {
		session = &Session{
			workingDir:  m.workingDir,
			command:     m.command,
			commandArgs: append([]string(nil), m.commandArgs...),
			subscribers: make(map[chan []byte]struct{}),
		}
		m.sessions[id] = session
	}
	return session
}

func (s *Session) EnsureRunning(cols, rows int, restart bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running && s.ptmx != nil {
		if restart {
			if err := s.stopLocked(); err != nil {
				return err
			}
		} else {
			return pty.Setsize(s.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
		}
	}

	cmd := exec.Command(s.command, s.commandArgs...)
	cmd.Dir = s.workingDir
	env := os.Environ()
	filteredEnv := make([]string, 0, len(env))
	hasUTF8Locale := false
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		if strings.HasPrefix(e, "LC_ALL=") || strings.HasPrefix(e, "LC_CTYPE=") || strings.HasPrefix(e, "LANG=") {
			value := strings.ToLower(strings.SplitN(e, "=", 2)[1])
			if strings.Contains(value, "utf-8") || strings.Contains(value, "utf8") {
				hasUTF8Locale = true
			}
		}
		filteredEnv = append(filteredEnv, e)
	}
	cmd.Env = append(filteredEnv,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
	if !hasUTF8Locale {
		cmd.Env = append(cmd.Env, "LANG=en_US.UTF-8")
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return err
	}

	s.cmd = cmd
	s.ptmx = ptmx
	s.running = true
	s.history = s.history[:0]

	go s.readLoop(ptmx)
	go s.waitLoop(cmd, ptmx)
	return nil
}

func (s *Session) Input(data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ptmx == nil {
		return errors.New("terminal is not running")
	}
	_, err := s.ptmx.Write(data)
	return err
}

func (s *Session) Resize(cols, rows int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ptmx == nil {
		return nil
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (s *Session) Snapshot() ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]byte(nil), s.history...), s.running
}

func (s *Session) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 128)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		delete(s.subscribers, ch)
		s.mu.Unlock()
	}
	return ch, cancel
}

func (s *Session) readLoop(ptmx *os.File) {
	buffer := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			s.appendHistory(chunk)
			s.broadcast(chunk)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && s.shouldReportStreamError(ptmx) {
				s.broadcast([]byte(fmt.Sprintf("\r\n[web-claude] terminal stream closed: %v\r\n", err)))
			}
			return
		}
	}
}

func (s *Session) waitLoop(cmd *exec.Cmd, ptmx *os.File) {
	err := cmd.Wait()
	if s.finishProcess(cmd, ptmx) && err != nil {
		s.broadcast([]byte(fmt.Sprintf("\r\n[web-claude] process exited: %v\r\n", err)))
	}
}

func (s *Session) appendHistory(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(chunk) >= historyLimit {
		s.history = append([]byte(nil), chunk[len(chunk)-historyLimit:]...)
		return
	}
	s.history = append(s.history, chunk...)
	if len(s.history) <= historyLimit {
		return
	}
	extra := len(s.history) - historyLimit
	s.history = bytes.Clone(s.history[extra:])
}

func (s *Session) broadcast(chunk []byte) {
	s.mu.RLock()
	subscribers := make([]chan []byte, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subscribers = append(subscribers, ch)
	}
	s.mu.RUnlock()
	for _, subscriber := range subscribers {
		select {
		case subscriber <- append([]byte(nil), chunk...):
		default:
		}
	}
}

func (s *Session) stopLocked() error {
	if s.ptmx != nil {
		_ = s.ptmx.Close()
		s.ptmx = nil
	}
	if s.cmd == nil || s.cmd.Process == nil {
		s.running = false
		return nil
	}
	err := s.cmd.Process.Kill()
	s.cmd = nil
	s.running = false
	return err
}

func (s *Session) finishProcess(cmd *exec.Cmd, ptmx *os.File) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != cmd {
		return false
	}
	s.cmd = nil
	if s.ptmx == ptmx {
		_ = s.ptmx.Close()
		s.ptmx = nil
	}
	s.running = false
	return true
}

func (s *Session) shouldReportStreamError(ptmx *os.File) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ptmx == ptmx
}
