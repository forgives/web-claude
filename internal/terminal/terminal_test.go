package terminal

import (
	"os"
	"os/exec"
	"testing"
)

func TestFinishProcessDoesNotClearNewSessionState(t *testing.T) {
	oldPTY, err := os.CreateTemp(t.TempDir(), "old-pty")
	if err != nil {
		t.Fatalf("CreateTemp oldPTY: %v", err)
	}
	defer oldPTY.Close()

	newPTY, err := os.CreateTemp(t.TempDir(), "new-pty")
	if err != nil {
		t.Fatalf("CreateTemp newPTY: %v", err)
	}
	defer newPTY.Close()

	oldCmd := exec.Command("true")
	newCmd := exec.Command("true")

	session := &Session{
		cmd:     newCmd,
		ptmx:    newPTY,
		running: true,
	}

	if cleared := session.finishProcess(oldCmd, oldPTY); cleared {
		t.Fatal("old process should not clear current session state")
	}
	if session.cmd != newCmd {
		t.Fatal("current command was cleared by old process exit")
	}
	if session.ptmx != newPTY {
		t.Fatal("current PTY was cleared by old process exit")
	}
	if !session.running {
		t.Fatal("session should still be running")
	}
}

func TestFinishProcessClearsMatchingSessionState(t *testing.T) {
	ptyFile, err := os.CreateTemp(t.TempDir(), "pty")
	if err != nil {
		t.Fatalf("CreateTemp pty: %v", err)
	}

	cmd := exec.Command("true")
	session := &Session{
		cmd:     cmd,
		ptmx:    ptyFile,
		running: true,
	}

	if cleared := session.finishProcess(cmd, ptyFile); !cleared {
		t.Fatal("matching process should clear session state")
	}
	if session.cmd != nil {
		t.Fatal("expected command to be cleared")
	}
	if session.ptmx != nil {
		t.Fatal("expected PTY to be cleared")
	}
	if session.running {
		t.Fatal("expected session to be stopped")
	}
}
