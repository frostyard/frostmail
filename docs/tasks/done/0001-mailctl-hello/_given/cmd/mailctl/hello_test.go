package main

// CONTRACT TEST for task card T-0001 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// runMailctl runs mailctl with args through clix, as main does, and returns
// what it wrote to stdout.
func runMailctl(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	clix.Stdout = &out
	t.Cleanup(func() { clix.Stdout = nil })
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(&out)
	err := (&clix.App{Version: "test"}).Run(root)
	return out.String(), err
}

func TestHelloText(t *testing.T) {
	srv := rpctest.Start(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "maild rpctest (protocol 1)\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestHelloJSON(t *testing.T) {
	srv := rpctest.Start(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "--json", "hello")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Protocol int    `json:"protocol"`
		Server   string `json:"server"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output %q is not JSON: %v", out, err)
	}
	if got.Protocol != 1 || got.Server != "maild rpctest" {
		t.Fatalf("JSON = %+v", got)
	}
}

func TestHelloWithoutMaild(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "maild.sock")
	_, err := runMailctl(t, "--socket", missing, "hello")
	if err == nil || !strings.Contains(err.Error(), "connect to maild") {
		t.Fatalf("error = %v, want one mentioning connect to maild", err)
	}
}
