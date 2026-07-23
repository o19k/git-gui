package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// showCursor turns the cursor back on. The TUI hides it, and a shell prompt
// with no cursor looks like a hung terminal.
const showCursor = "\x1b[?25h"

// resetAttributes drops any colour left half-applied by the frame that was
// being drawn when the panic hit.
const resetAttributes = "\x1b[0m"

// restoreOnPanic is deferred from main. bubbletea catches panics raised inside
// its own loop and its commands, restores the terminal itself and reports the
// stack; this covers what is left — anything raised before the program starts
// or after it returns — which would otherwise leave the terminal on the
// alternate screen with no cursor and no explanation.
//
// Raw mode is not undone here: the only panics reaching this point are ones
// raised while the terminal was never put into it.
func restoreOnPanic() {
	value := recover()
	if value == nil {
		return
	}
	reportPanic(os.Stdout, os.Stderr, value, debug.Stack(), isTerminal(os.Stdout))
	os.Exit(2)
}

// reportPanic puts the terminal back and writes the failure where it can be
// read. The screen is deliberately not cleared: the stack is the only record of
// what happened.
func reportPanic(out, errOut io.Writer, value any, stack []byte, terminal bool) {
	if terminal {
		fmt.Fprint(out, leaveAltScreen+showCursor+resetAttributes)
	}
	fmt.Fprintf(errOut, "git-gui: panic: %v\n\n%s", value, stack)
}
