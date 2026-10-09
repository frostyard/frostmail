package main

import (
	"slices"
	"strings"
	"testing"
)

func TestCodexCommand(t *testing.T) {
	e := executor{name: "codex", root: "/repo", writable: []string{"/home/u/.cache/go-build", "/home/u/.cache/mise"}}
	name, args, err := e.command("do it", false)
	if err != nil || name != "codex" {
		t.Fatalf("command = %s, %v", name, err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"exec --color never -C /repo",
		`sandbox_mode="workspace-write"`,
		`sandbox_workspace_write.writable_roots=["/home/u/.cache/go-build", "/home/u/.cache/mise"]`,
		`approval_policy="never"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %q", want, args)
		}
	}
	if args[len(args)-1] != "do it" || slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "model=") }) {
		t.Errorf("args = %q; want the prompt last and the user's default model", args)
	}

	e.model = "gpt-test"
	_, args, _ = e.command("again", true)
	if strings.Join(args[:3], " ") != "exec resume --last" || !slices.Contains(args, `model="gpt-test"`) ||
		!slices.Contains(args, `sandbox_mode="workspace-write"`) || args[len(args)-1] != "again" {
		t.Errorf("resume args = %q", args)
	}
}

func TestOpencodeCommand(t *testing.T) {
	if _, _, err := (executor{name: "opencode"}).command("p", false); err == nil {
		t.Error("opencode without a model was accepted")
	}
	e := executor{name: "opencode", model: "local/qwen", title: "T-0099"}
	if _, args, _ := e.command("p", false); strings.Join(args, " ") != "run --model local/qwen --title T-0099 p" {
		t.Errorf("first args = %q", args)
	}
	if _, args, _ := e.command("q", true); strings.Join(args, " ") != "run --continue --model local/qwen q" {
		t.Errorf("resume args = %q", args)
	}
	if _, _, err := (executor{name: "cursor"}).command("p", false); err == nil {
		t.Error("an unknown executor was accepted")
	}
}

func TestGuidance(t *testing.T) {
	goCard := &Card{ID: "0099", Touch: []string{"internal/x/x.go"}}
	appCard := &Card{ID: "0100", Touch: []string{"app/src/features/x/X.tsx"}}
	codex := executor{name: "codex"}
	if g := codex.guidance(goCard); !strings.Contains(g, "mise exec -- make accept T=0099") || strings.Contains(g, "nsl") {
		t.Errorf("Go card guidance = %q", g)
	}
	if g := codex.guidance(appCard); !strings.Contains(g, "nsl machine") || !strings.Contains(g, "do not run them") {
		t.Errorf("app card guidance = %q", g)
	}
	if g := (executor{name: "opencode"}).guidance(appCard); g != "" {
		t.Errorf("opencode guidance = %q", g)
	}
}
