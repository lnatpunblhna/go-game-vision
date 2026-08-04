package keyboard

import "unicode/utf16"

// utf16Encode converts a string to UTF-16 code units.
//
// Both platforms' text-injection APIs take UTF-16, and both need surrogate pairs
// sent as two units rather than one — so emoji and other non-BMP characters cannot
// be handled by casting runes.
func utf16Encode(text string) []uint16 {
	return utf16.Encode([]rune(text))
}
