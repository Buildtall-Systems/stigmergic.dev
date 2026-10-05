// Package feedlang is the one home of a feed's channel language: the default
// every creation path presets and every reader falls back to, and the check
// a declared value must pass. A value is an RSS language code, the BCP 47
// form RSS readers consume (en-us, pt-br, de), lowercased.
//
// The check is deliberately permissive: a primary subtag of two or three
// letters, then any number of letter-or-digit subtags, hyphen-joined. It
// refuses what no reader can use without second-guessing registry
// membership.
package feedlang

import (
	"errors"
	"fmt"
	"strings"
)

// Default is the language a feed carries when its creator declared none, and
// the value every creation form presets.
const Default = "en-us"

// ErrInvalid marks a declared value that is not a language code. A creation
// path refuses it; a reader serves Default instead.
var ErrInvalid = errors.New("not a language code such as " + Default)

const (
	subtagSep       = "-"
	primaryMinLen   = 2
	primaryMaxLen   = 3
	subtagMinLen    = 1
	subtagMaxLen    = 8
	asciiLowerFirst = 'a'
	asciiLowerLast  = 'z'
	asciiDigitFirst = '0'
	asciiDigitLast  = '9'
)

// Normalize trims and lowercases a raw value and reports whether the result
// is a language code. The empty string is not one.
func Normalize(raw string) (string, bool) {
	code := strings.ToLower(strings.TrimSpace(raw))
	subtags := strings.Split(code, subtagSep)
	primary := subtags[0]
	if len(primary) < primaryMinLen || len(primary) > primaryMaxLen || !all(primary, isLetter) {
		return code, false
	}
	for _, subtag := range subtags[1:] {
		if len(subtag) < subtagMinLen || len(subtag) > subtagMaxLen || !all(subtag, isAlnum) {
			return code, false
		}
	}
	return code, true
}

// Resolve returns the normalized value when it is a language code, and
// Default otherwise: the reader's rule, which never fails a request.
func Resolve(raw string) string {
	if code, ok := Normalize(raw); ok {
		return code
	}
	return Default
}

// Declare is the creation paths' rule: a blank value takes Default, a
// language code is normalized, and anything else is refused with ErrInvalid.
func Declare(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return Default, nil
	}
	if code, ok := Normalize(raw); ok {
		return code, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalid, raw)
}

func all(s string, accept func(byte) bool) bool {
	for i := range len(s) {
		if !accept(s[i]) {
			return false
		}
	}
	return true
}

func isLetter(c byte) bool {
	return c >= asciiLowerFirst && c <= asciiLowerLast
}

func isAlnum(c byte) bool {
	return isLetter(c) || (c >= asciiDigitFirst && c <= asciiDigitLast)
}
