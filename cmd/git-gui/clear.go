package main

import (
	"fmt"
	"io"
	"os"
)

// clearSequence is what `clear` sends: cursor home, erase the screen, then erase
// the scrollback (the E3 capability). Terminals without E3 ignore the last part.
const clearSequence = "\x1b[H\x1b[2J\x1b[3J"

// leaveAltScreen is rmcup. bubbletea already sends it on exit; repeating it is a
// no-op on the main buffer and covers the case where its own teardown was cut
// short.
const leaveAltScreen = "\x1b[?1049l"

// clearScreen wipes what the TUI left behind. The alt screen is supposed to hand
// the old buffer back untouched, but over ssh, inside multiplexers, and with a
// TERM that has no alt-screen caps that restore does not happen and the last
// frame stays under the prompt. Clearing costs nothing when the restore worked.
func clearScreen(w io.Writer) {
	fmt.Fprint(w, leaveAltScreen+clearSequence)
}

// isTerminal reports whether f is a tty, so redirected output keeps its escape
// codes out of the file.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
