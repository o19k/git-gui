package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportPanicPutsTheTerminalBack(t *testing.T) {
	var out, errOut bytes.Buffer

	reportPanic(&out, &errOut, "nil map write", []byte("goroutine 1 [running]:\nmain.main()\n"), true)

	// The alternate screen and the hidden cursor both have to go, or there is
	// nothing to read the failure on.
	if !strings.Contains(out.String(), leaveAltScreen) {
		t.Errorf("the alternate screen was not left: %q", out.String())
	}
	if !strings.Contains(out.String(), showCursor) {
		t.Errorf("the cursor was left hidden: %q", out.String())
	}
	// Clearing here would erase the only record of what happened.
	if strings.Contains(out.String(), clearSequence) {
		t.Error("the screen was cleared over the stack trace")
	}

	if !strings.Contains(errOut.String(), "nil map write") {
		t.Errorf("the panic value is missing: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "main.main()") {
		t.Errorf("the stack is missing: %q", errOut.String())
	}
}

func TestReportPanicWritesNoEscapesWhenRedirected(t *testing.T) {
	var out, errOut bytes.Buffer

	reportPanic(&out, &errOut, "boom", []byte("stack\n"), false)

	if out.Len() != 0 {
		t.Errorf("escape codes went into a redirected stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "boom") {
		t.Error("the failure was not reported at all")
	}
}
