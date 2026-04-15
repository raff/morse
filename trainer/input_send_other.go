//go:build !darwin

package main

// StartSendKeySource wraps the raw terminal byte stream as a KeyEvent source.
// On non-macOS platforms there is no CGEventTap, so press+release events are
// not available: each keystroke emits a synthetic press+release pair, and the
// IambicAdapter will not auto-repeat while a key is held.
func StartSendKeySource(stdinChars <-chan byte, ditKey, dahKey byte) (<-chan KeyEvent, func(), error) {
	ch := NewTerminalKeySource(stdinChars, ditKey, dahKey)
	return ch, func() {}, nil
}
