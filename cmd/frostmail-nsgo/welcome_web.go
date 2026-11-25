//go:build js && wasm

package main

// A browser is on no computer's maild: it has no host install to find.
func hostMailFound() bool { return false }
