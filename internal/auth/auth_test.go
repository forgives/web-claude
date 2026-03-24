package auth

import "testing"

func TestSanitizeTerminalInputAllowsCommonControls(t *testing.T) {
	input := []byte{0x03, 0x04, 0x0c, 0x15, 0x1b, '[', 'A', 0x1b, 'f'}

	got, err := SanitizeTerminalInput(input)
	if err != nil {
		t.Fatalf("SanitizeTerminalInput returned error: %v", err)
	}
	if string(got) != string(input) {
		t.Fatalf("SanitizeTerminalInput changed input: got %v want %v", got, input)
	}
}

func TestSanitizeTerminalInputRejectsDangerousEscape(t *testing.T) {
	if _, err := SanitizeTerminalInput([]byte{0x1b, ']', '0', ';', 'x'}); err == nil {
		t.Fatal("expected OSC sequence to be rejected")
	}
}

func TestSanitizeTerminalInputRejectsNullByte(t *testing.T) {
	if _, err := SanitizeTerminalInput([]byte{'a', 0, 'b'}); err == nil {
		t.Fatal("expected null byte to be rejected")
	}
}
