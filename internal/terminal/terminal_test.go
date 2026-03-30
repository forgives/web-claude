package terminal

import (
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestNewManagerCopiesCommandArgsIntoSession(t *testing.T) {
	manager := NewManager("/tmp/workspace", "claude", []string{"-c"}, false)

	session := manager.get("default")
	if session.command != "claude" {
		t.Fatalf("unexpected command: %q", session.command)
	}
	if !reflect.DeepEqual(session.commandArgs, []string{"-c"}) {
		t.Fatalf("unexpected command args: %#v", session.commandArgs)
	}

	manager.commandArgs[0] = "--resume"
	if !reflect.DeepEqual(session.commandArgs, []string{"-c"}) {
		t.Fatalf("session command args should be isolated from manager mutation: %#v", session.commandArgs)
	}
}

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
