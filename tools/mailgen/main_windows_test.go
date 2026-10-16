package main

import (
	"fmt"
	"os"
	"testing"
)

// TestMain skips mailgen's tests on Windows: a Maildir's file names hold
// ":2,", which Windows cannot (NTFS reads a colon as a stream name).
// mailgen is a Linux development tool.
func TestMain(m *testing.M) {
	fmt.Println("skipping mailgen: Maildir names cannot be Windows file names")
	_ = m
	os.Exit(0)
}
