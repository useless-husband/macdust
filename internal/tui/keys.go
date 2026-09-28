package tui

import "unicode/utf8"

// Key identifies a non-printable key; printable input uses KeyRune.
type Key int

const (
	KeyRune Key = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyBackspace
	KeyEsc
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeyCtrlC
)

// KeyEvent is one decoded key press.
type KeyEvent struct {
	Key  Key
	Rune rune // set when Key == KeyRune
}

// ParseKeys decodes raw terminal input into key events. It understands
// CSI (ESC [) and SS3 (ESC O) arrow/paging sequences; unknown escape
// sequences are swallowed rather than typed into the search box.
func ParseKeys(b []byte) []KeyEvent {
	var out []KeyEvent
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			if i+1 >= len(b) {
				out = append(out, KeyEvent{Key: KeyEsc})
				i++
				continue
			}
			if b[i+1] != '[' && b[i+1] != 'O' {
				out = append(out, KeyEvent{Key: KeyEsc})
				i++
				continue
			}
			j := i + 2
			for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
				j++
			}
			if j >= len(b) {
				i = len(b)
				continue
			}
			params, final := string(b[i+2:j]), b[j]
			i = j + 1
			switch final {
			case 'A':
				out = append(out, KeyEvent{Key: KeyUp})
			case 'B':
				out = append(out, KeyEvent{Key: KeyDown})
			case 'C':
				out = append(out, KeyEvent{Key: KeyRight})
			case 'D':
				out = append(out, KeyEvent{Key: KeyLeft})
			case 'H':
				out = append(out, KeyEvent{Key: KeyHome})
			case 'F':
				out = append(out, KeyEvent{Key: KeyEnd})
			case '~':
				switch params {
				case "1", "7":
					out = append(out, KeyEvent{Key: KeyHome})
				case "4", "8":
					out = append(out, KeyEvent{Key: KeyEnd})
				case "5":
					out = append(out, KeyEvent{Key: KeyPgUp})
				case "6":
					out = append(out, KeyEvent{Key: KeyPgDn})
				}
			}
		case c == '\r' || c == '\n':
			out = append(out, KeyEvent{Key: KeyEnter})
			i++
		case c == 0x7f || c == 0x08:
			out = append(out, KeyEvent{Key: KeyBackspace})
			i++
		case c == 0x03:
			out = append(out, KeyEvent{Key: KeyCtrlC})
			i++
		case c < 0x20:
			i++ // other control characters are ignored
		default:
			r, size := utf8.DecodeRune(b[i:])
			if r != utf8.RuneError || size > 1 {
				out = append(out, KeyEvent{Key: KeyRune, Rune: r})
			}
			i += size
		}
	}
	return out
}
