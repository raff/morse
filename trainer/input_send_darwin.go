//go:build darwin

package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>

// Device-specific left/right control masks (IOKit/hidsystem/IOLLEvent.h).
#define NX_DEVICELCTLKEYMASK  0x00000001
#define NX_DEVICERCTLKEYMASK  0x00002000

// Virtual key codes.
#define kVK_Control      0x3B
#define kVK_RightControl 0x3E

// Configurable dit/dah virtual key codes (set before creating the tap).
static int64_t g_dit_kc = 0x21;   // default: [
static int64_t g_dah_kc = 0x1E;   // default: ]

static CFMachPortRef g_tap;
static CFRunLoopRef  g_loop;

// g_windowFocused is set by Go via setWindowFocused() when the terminal
// sends DECSET-1004 focus-in (ESC [ I) or focus-out (ESC [ O) events.
// Starts true: we assume the trainer was launched in the active window.
static int g_windowFocused = 1;

static void setWindowFocused(int f) { g_windowFocused = f; }

// goDeliverSendKey is the single delivery point from C to Go.
// key:     0 = dit, 1 = dah
// pressed: 0 = release (direct), 1 = direct press (modifier keys), 2 = pending press (regular keys)
extern void goDeliverSendKey(int key, int pressed);

static CGEventRef sendCB(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *ctx) {
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        CGEventTapEnable(g_tap, true);
        return event;
    }

    if (type == kCGEventKeyDown) {
        // Ignore OS key-repeat — IambicAdapter drives its own timing.
        if (CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat))
            return event;

        int64_t kc = CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);

        // Regular dit/dah keys: emit a PENDING press (pressed=2).
        // Delivery is held until stdin confirms the event came from our window.
        if (kc == g_dit_kc) { goDeliverSendKey(0, 2); return event; }
        if (kc == g_dah_kc) { goDeliverSendKey(1, 2); return event; }

        // All other keys (Enter, Delete, Ctrl+C/D, Escape) are handled
        // via stdin so they only fire for our own terminal window.

    } else if (type == kCGEventKeyUp) {
        int64_t kc = CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        // Release is delivered only if the press was previously confirmed.
        // goDeliverSendKey checks the confirmed state on the Go side.
        if (kc == g_dit_kc) { goDeliverSendKey(0, 0); return event; }
        if (kc == g_dah_kc) { goDeliverSendKey(1, 0); return event; }

    } else if (type == kCGEventFlagsChanged) {
        // Modifier-only keys (left/right Ctrl from a USB iambic paddle) have no
        // stdin byte so they bypass stdin correlation.  Guard with a focus check
        // so that Ctrl presses in other apps don't affect the trainer.
        if (!g_windowFocused) return event;
        int64_t kc    = CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        CGEventFlags f = CGEventGetFlags(event);
        if (kc == kVK_Control)
            goDeliverSendKey(0, (f & NX_DEVICELCTLKEYMASK) ? 1 : 0);
        else if (kc == kVK_RightControl)
            goDeliverSendKey(1, (f & NX_DEVICERCTLKEYMASK) ? 1 : 0);
    }

    return event;
}

static int createSendTap(int64_t dit_kc, int64_t dah_kc) {
    g_dit_kc = dit_kc;
    g_dah_kc = dah_kc;
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown) |
                       CGEventMaskBit(kCGEventKeyUp)   |
                       CGEventMaskBit(kCGEventFlagsChanged);
    g_tap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGTailAppendEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        sendCB,
        NULL
    );
    return g_tap != NULL ? 1 : 0;
}

static void runSendTap() {
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, g_tap, 0);
    g_loop = CFRunLoopGetCurrent();
    CFRunLoopAddSource(g_loop, src, kCFRunLoopCommonModes);
    CGEventTapEnable(g_tap, true);
    CFRunLoopRun();
    CFRelease(src);
}

static void stopSendTap() {
    if (g_loop) {
        CFRunLoopStop(g_loop);
        g_loop = NULL;
    }
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// charToKeycode maps ASCII paddle-key characters to US-ANSI virtual key codes.
var charToKeycode = map[byte]C.int64_t{
	'[': 0x21, // kVK_ANSI_LeftBracket
	']': 0x1E, // kVK_ANSI_RightBracket
	'.': 0x2F, // kVK_ANSI_Period
	',': 0x2B, // kVK_ANSI_Comma
	'/': 0x2C, // kVK_ANSI_Slash
	';': 0x29, // kVK_ANSI_Semicolon
	' ': 0x31, // kVK_Space
}

// tapState tracks which dit/dah presses are pending stdin confirmation and
// which are confirmed (held).  Accessed from both the CGEventTap callback
// thread and the stdin goroutine, so all fields are protected by the mutex.
var tapState struct {
	sync.Mutex
	ditPendingAt time.Time // time of last unconfirmed dit KeyDown; zero if none
	dahPendingAt time.Time // time of last unconfirmed dah KeyDown; zero if none
	ditHeld      bool      // true after stdin confirmed the dit press
	dahHeld      bool      // true after stdin confirmed the dah press (or modifier)
}

var sendKeysCh chan KeyEvent

// goDeliverSendKey is called from the CGEventTap callback.
//
// pressed values:
//
//	2 = pending press  (regular key; needs stdin confirmation before delivery)
//	1 = direct press   (modifier key; delivered immediately without stdin confirmation)
//	0 = release        (delivered only if the key was previously confirmed)
//
//export goDeliverSendKey
func goDeliverSendKey(key C.int, pressed C.int) {
	ch := sendKeysCh
	if ch == nil {
		return
	}
	k := KeyDit
	if key != 0 {
		k = KeyDah
	}

	switch pressed {
	case 2: // pending press — record timestamp, wait for stdin confirmation
		tapState.Lock()
		if key == 0 {
			tapState.ditPendingAt = time.Now()
		} else {
			tapState.dahPendingAt = time.Now()
		}
		tapState.Unlock()

	case 1: // direct press (modifier keys bypass stdin correlation)
		tapState.Lock()
		if key == 0 {
			tapState.ditHeld = true
		} else {
			tapState.dahHeld = true
		}
		tapState.Unlock()
		select {
		case ch <- KeyEvent{Key: k, Pressed: true, At: time.Now(), Direct: true}:
		default:
		}

	case 0: // release — only deliver if the press was confirmed
		tapState.Lock()
		var held bool
		if key == 0 {
			held = tapState.ditHeld
			tapState.ditHeld = false
		} else {
			held = tapState.dahHeld
			tapState.dahHeld = false
		}
		tapState.Unlock()
		if held {
			select {
			case ch <- KeyEvent{Key: k, Pressed: false, At: time.Now()}:
			default:
			}
		}
	}
}

// confirmWindow is how long after a CGEventTap KeyDown we expect the
// corresponding stdin byte to arrive from Terminal.app.  The actual latency
// is typically < 5 ms; 30 ms gives a comfortable margin.
const confirmWindow = 30 * time.Millisecond

// StartSendKeySource creates a CGEventTap for send-mode input.
//
// Regular dit/dah keys ([, ] or configured alternatives) use stdin correlation:
// the tap records the keydown timestamp, and the stdin byte from our own PTY
// confirms it so that keypresses in other terminal windows are ignored.
//
// Left/right Control (USB iambic paddle) bypass stdin correlation and are
// delivered directly, since modifier keys never produce a stdin byte.
//
// Enter, Delete, and Ctrl+C/D are read from stdin so they are also scoped
// to our own terminal window.
//
// Requires Accessibility access: System Settings → Privacy & Security →
// Accessibility.
func StartSendKeySource(stdinChars <-chan byte, ditKey, dahKey byte) (<-chan KeyEvent, func(), error) {
	ditKC, ok1 := charToKeycode[ditKey]
	dahKC, ok2 := charToKeycode[dahKey]
	if !ok1 || !ok2 {
		return nil, nil, fmt.Errorf(
			"no keycode mapping for dit-key=%q or dah-key=%q;\n"+
				"  supported keys: [ ] . , / ;",
			ditKey, dahKey)
	}

	if C.createSendTap(ditKC, dahKC) == 0 {
		return nil, nil, fmt.Errorf(
			"CGEventTapCreate failed — grant Accessibility access to this terminal in\n" +
				"  System Settings → Privacy & Security → Accessibility")
	}

	ch := make(chan KeyEvent, 64)
	sendKeysCh = ch

	go func() {
		runtime.LockOSThread()
		C.runSendTap()
	}()

	// Enable terminal focus-event reporting (DECSET 1004).
	// The terminal will send ESC [ I on focus-in and ESC [ O on focus-out.
	// These arrive as stdin bytes and let us track whether our specific
	// terminal window is active — distinguishing windows within the same app.
	fmt.Print("\033[?1004h")

	// Stdin correlation goroutine.
	//
	// For dit/dah bytes: confirm a pending CGEventTap press (within confirmWindow)
	// and deliver it.  Events from other windows never produce a stdin byte so
	// their pending entries simply expire.
	//
	// For control bytes (Enter, Delete, Ctrl+C/D): deliver directly — they are
	// scoped to our terminal by virtue of coming through stdin.
	//
	// Focus events (ESC [ I / ESC [ O) update g_windowFocused so the
	// CGEventTap ignores modifier-key presses from unfocused windows.
	//
	// All other bytes (including OS key-repeat bytes for held dit/dah keys) are
	// discarded; the IambicAdapter drives its own repeat timing.
	go func() {
		for b := range stdinChars {
			now := time.Now()
			switch {
			case b == ditKey:
				tapState.Lock()
				pending := tapState.ditPendingAt
				tapState.ditPendingAt = time.Time{} // clear (confirmed or stale)
				if !pending.IsZero() && now.Sub(pending) < confirmWindow {
					tapState.ditHeld = true
					tapState.Unlock()
					select {
					case ch <- KeyEvent{Key: KeyDit, Pressed: true, At: pending}:
					default:
					}
				} else {
					tapState.Unlock() // stale or spurious, discard
				}

			case b == dahKey:
				tapState.Lock()
				pending := tapState.dahPendingAt
				tapState.dahPendingAt = time.Time{}
				if !pending.IsZero() && now.Sub(pending) < confirmWindow {
					tapState.dahHeld = true
					tapState.Unlock()
					select {
					case ch <- KeyEvent{Key: KeyDah, Pressed: true, At: pending}:
					default:
					}
				} else {
					tapState.Unlock()
				}

			case b == '\r' || b == '\n':
				select {
				case ch <- KeyEvent{Key: KeyEnter, Pressed: true, At: now}:
				default:
				}

			case b == 0x7F || b == 0x08:
				select {
				case ch <- KeyEvent{Key: KeyDelete, Pressed: true, At: now}:
				default:
				}

			case b == 3 || b == 4: // Ctrl+C / Ctrl+D
				select {
				case ch <- KeyEvent{Key: KeyQuit, Pressed: true, At: now}:
				default:
				}
				return // no more input after quit

			case b == 27: // ESC — read the rest of the escape sequence
				time.Sleep(5 * time.Millisecond)
				var seq []byte
				for len(stdinChars) > 0 {
					seq = append(seq, <-stdinChars)
				}
				// DECSET 1004 focus events: ESC [ I = focus-in, ESC [ O = focus-out.
				if len(seq) == 2 && seq[0] == '[' {
					switch seq[1] {
					case 'I':
						C.setWindowFocused(1)
					case 'O':
						C.setWindowFocused(0)
					}
				}
				// All other escape sequences are discarded.

			// Everything else (OS key-repeat bytes, etc.) is discarded.
			}
		}
	}()

	stop := func() {
		fmt.Print("\033[?1004l") // disable terminal focus-event reporting
		C.stopSendTap()
		sendKeysCh = nil
		for len(ch) > 0 {
			<-ch
		}
		close(ch)
	}

	return ch, stop, nil
}
