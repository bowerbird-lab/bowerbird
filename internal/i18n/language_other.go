//go:build !darwin

package i18n

// osLanguage has no platform lookup on this system yet; systemLanguage falls
// back to LANG/LC_ALL/LC_MESSAGES and then English.
func osLanguage() string { return "" }
