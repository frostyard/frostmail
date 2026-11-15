package workflowcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

var (
	commitAction = regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	dockerAction = regexp.MustCompile(`^docker://[^@]+@sha256:[0-9a-f]{64}$`)
)

// immutable reports whether an action reference names one version for
// good: a local action, a commit, or an image digest.
func immutable(ref string) bool {
	return strings.HasPrefix(ref, "./") || commitAction.MatchString(ref) || dockerAction.MatchString(ref)
}

func TestImmutableReferences(t *testing.T) {
	sha, digest := strings.Repeat("a", 40), strings.Repeat("b", 64)
	for ref, want := range map[string]bool{
		"actions/checkout@" + sha:                   true,
		"owner/repo/.github/actions/publish@" + sha: true,
		"docker://alpine@sha256:" + digest:          true,
		"./.github/actions/check":                   true,
		"actions/checkout@v7":                       false,
		"owner/repo/.github/actions/publish@main":   false,
		"actions/checkout@deadbeef":                 false,
		"actions/checkout@${{inputs.ref}}":          false,
		"docker://alpine:3.22":                      false,
		"actions/checkout@" + strings.ToUpper(sha):  false,
	} {
		if got := immutable(ref); got != want {
			t.Errorf("immutable(%q) = %v, want %v", ref, got, want)
		}
	}
}

// workflows parses every workflow in .github/workflows.
func workflows(t *testing.T) map[string]*yaml.Node {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no workflows found")
	}
	out := map[string]*yaml.Node{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
			t.Fatalf("parse %s: %v", p, err)
		}
		out[filepath.Base(p)] = doc.Content[0]
	}
	return out
}

// get returns the value of key in a mapping node, or nil.
func get(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// walk calls f for every mapping node under n.
func walk(n *yaml.Node, f func(*yaml.Node)) {
	if n.Kind == yaml.MappingNode {
		f(n)
	}
	for _, c := range n.Content {
		walk(c, f)
	}
}

// Every external action is pinned to a full commit SHA (the version a
// trailing comment names is for people; the SHA is what runs), every
// workflow starts from declared permissions, and every checkout drops the
// token it would otherwise leave in .git/config.
func TestWorkflowsFollowADR0021(t *testing.T) {
	uses := 0
	for name, root := range workflows(t) {
		if get(root, "permissions") == nil {
			t.Errorf("%s: no top-level permissions (start from permissions: {})", name)
		}
		walk(root, func(m *yaml.Node) {
			ref := get(m, "uses")
			if ref == nil {
				return
			}
			uses++
			if ref.Kind != yaml.ScalarNode || !immutable(ref.Value) {
				t.Errorf("%s:%d: %q must name a full commit SHA or image digest", name, ref.Line, ref.Value)
			}
			if strings.HasPrefix(ref.Value, "actions/checkout@") {
				if pc := get(get(m, "with"), "persist-credentials"); pc == nil || pc.Value != "false" {
					t.Errorf("%s:%d: checkout must set persist-credentials: false", name, ref.Line)
				}
			}
		})
	}
	if uses == 0 {
		t.Fatal("no action references found")
	}
}

// Every job that runs in flathub's flatpak-github-actions image mounts a
// named volume over /var/lib/flatpak. flatpak-builder updates the image's
// runtime on every build, and the update renames a directory of an image
// layer; on an overlayfs with redirect_dir off that rename fails with EXDEV,
// and the build failed that way on AWS-hosted runners. A named volume holds a
// copy of the installation on the host's own file system.
func TestFlatpakJobsKeepTheInstallationOffOverlayfs(t *testing.T) {
	jobs := 0
	for name, root := range workflows(t) {
		all := get(root, "jobs")
		if all == nil || all.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(all.Content); i += 2 {
			id, job := all.Content[i].Value, all.Content[i+1]
			container := get(job, "container")
			image := get(container, "image")
			if container != nil && container.Kind == yaml.ScalarNode {
				image = container
			}
			if image == nil || !strings.Contains(image.Value, "/flatpak-github-actions:") {
				continue
			}
			jobs++
			if !namedVolumeAt(get(container, "volumes"), "/var/lib/flatpak") {
				t.Errorf("%s: job %s runs in %s without a named volume at /var/lib/flatpak", name, id, image.Value)
			}
		}
	}
	if jobs == 0 {
		t.Fatal("no job runs in the flatpak-github-actions image")
	}
	t.Logf("%d jobs run in the flatpak-github-actions image", jobs)
}

// namedVolumeAt reports whether a container's volumes mount a named volume
// (not a host path) at target.
func namedVolumeAt(volumes *yaml.Node, target string) bool {
	if volumes == nil || volumes.Kind != yaml.SequenceNode {
		return false
	}
	for _, v := range volumes.Content {
		source, dest, ok := strings.Cut(v.Value, ":")
		dest, _, _ = strings.Cut(dest, ":")
		if ok && dest == target && source != "" && !strings.ContainsAny(source[:1], "/.~$") {
			return true
		}
	}
	return false
}

func TestNamedVolumeAt(t *testing.T) {
	for spec, want := range map[string]bool{
		"flatpak-installation:/var/lib/flatpak":    true,
		"flatpak-installation:/var/lib/flatpak:rw": true,
		"/srv/flatpak:/var/lib/flatpak":            false,
		"./flatpak:/var/lib/flatpak":               false,
		"/var/lib/flatpak":                         false,
		"flatpak-installation:/var/lib/flatpak2":   false,
		"flatpak-installation:/tmp":                false,
	} {
		volumes := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: spec}}}
		if got := namedVolumeAt(volumes, "/var/lib/flatpak"); got != want {
			t.Errorf("namedVolumeAt(%q) = %v, want %v", spec, got, want)
		}
	}
}

// The release workflow's build job runs third-party npm code, so it holds no
// write token and no id-token; only the release job may write, attest and
// mint an id-token (frostyard/core ADR-0011, ADR-0021).
func TestReleaseBuildJobCannotWrite(t *testing.T) {
	root, ok := workflows(t)["release.yml"]
	if !ok {
		t.Fatal("no release.yml")
	}
	jobs := get(root, "jobs")
	build := get(get(jobs, "build"), "permissions")
	if build == nil || build.Kind != yaml.MappingNode {
		t.Fatal("release.yml: the build job declares no permissions")
	}
	for i := 0; i+1 < len(build.Content); i += 2 {
		if k, v := build.Content[i].Value, build.Content[i+1].Value; k != "contents" || v != "read" {
			t.Errorf("release.yml: build job has permission %s: %s; it may only read contents", k, v)
		}
	}
	if get(get(jobs, "release"), "permissions") == nil {
		t.Error("release.yml: the release job declares no permissions")
	}
}
