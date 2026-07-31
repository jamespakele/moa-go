package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOutputCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteOutput("# Synthesis Result\n\nTest content.", dir, "test prompt", false, "_bmad/bmm/config.yaml")
	if err != nil {
		t.Fatalf("WriteOutput failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("output file does not exist: %v", err)
	}
	if filepath.Ext(path) != ".md" {
		t.Errorf("expected .md extension, got %s", filepath.Ext(path))
	}
	if !strings.Contains(filepath.Base(path), "moa-") {
		t.Errorf("expected filename to contain moa-, got %s", filepath.Base(path))
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(written) != "# Synthesis Result\n\nTest content." {
		t.Errorf("unexpected output content: %s", string(written))
	}
}

func TestWriteOutputCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	path, err := WriteOutput("# Test", dir, "prompt", false, "_bmad/bmm/config.yaml")
	if err != nil {
		t.Fatalf("WriteOutput failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("output file does not exist: %v", err)
	}
}

func TestWriteOutputFilenameFormat(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteOutput("content", dir, "unique-prompt-123", false, "_bmad/bmm/config.yaml")
	if err != nil {
		t.Fatalf("WriteOutput failed: %v", err)
	}
	filename := filepath.Base(path)
	if !strings.HasPrefix(filename, "moa-") || !strings.HasSuffix(filename, ".md") {
		t.Errorf("unexpected filename: %s", filename)
	}
	parts := strings.Split(filename, "-")
	if len(parts) != 4 {
		t.Errorf("expected 4 parts (moa, date, time, hash.md), got %d in %s", len(parts), filename)
	}
	if len(parts[1]) != 8 {
		t.Errorf("expected 8-char date, got %s", parts[1])
	}
	if len(parts[2]) != 6 {
		t.Errorf("expected 6-char time, got %s", parts[2])
	}
}

func TestWrapBmadSpecHasFrontmatter(t *testing.T) {
	result := wrapBmadSpec("This is the synthesis.", "Create a rig backend")
	if !strings.Contains(result, "---") {
		t.Error("missing frontmatter ---")
	}
	if !strings.Contains(result, "status: 'draft'") {
		t.Error("missing status draft")
	}
	if !strings.Contains(result, "title: 'MoA: Create a rig backend'") {
		t.Error("missing title")
	}
	if !strings.Contains(result, "type: 'feature'") {
		t.Error("missing type feature")
	}
}

func TestWrapBmadSpecHasFrozenBlock(t *testing.T) {
	result := wrapBmadSpec("The answer is 42.", "What is the answer?")
	if !strings.Contains(result, "<frozen-after-approval") {
		t.Error("missing frozen-after-approval open")
	}
	if !strings.Contains(result, "</frozen-after-approval>") {
		t.Error("missing frozen-after-approval close")
	}
	if !strings.Contains(result, "The answer is 42.") {
		t.Error("missing content")
	}
}

func TestWrapBmadSpecHasEmptySections(t *testing.T) {
	result := wrapBmadSpec("synthesis", "prompt")
	for _, want := range []string{"## Code Map", "## Tasks & Acceptance", "## Spec Change Log", "## Verification"} {
		if !strings.Contains(result, want) {
			t.Errorf("missing section %q", want)
		}
	}
}

func TestWriteOutputBmadModeWrapsContent(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteOutput("synthesis content", dir, "test prompt", true, "_bmad/bmm/config.yaml")
	if err != nil {
		t.Fatalf("WriteOutput failed: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(written), "status: 'draft'") {
		t.Error("missing status draft")
	}
	if !strings.Contains(string(written), "<frozen-after-approval") {
		t.Error("missing frozen block")
	}
	if !strings.Contains(string(written), "synthesis content") {
		t.Error("missing synthesis content")
	}
	filename := filepath.Base(path)
	if !strings.HasPrefix(filename, "spec-moa-") || !strings.HasSuffix(filename, ".md") {
		t.Errorf("unexpected bmad filename: %s", filename)
	}
	if !strings.Contains(path, "_bmad-output/implementation-artifacts") {
		t.Errorf("expected bmad artifacts dir in path, got %s", path)
	}
}

func TestSlugifyBasic(t *testing.T) {
	cases := map[string]string{
		"Create a rig backend": "create-a-rig-backend",
		"Plan the migration!":  "plan-the-migration",
		"  hello   world  ":     "hello-world",
	}
	for input, want := range cases {
		if got := slugify(input); got != want {
			t.Errorf("slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSlugifyTruncates(t *testing.T) {
	long := strings.Repeat("a", 100)
	if got := slugify(long); len(got) != 40 {
		t.Errorf("slugify long string length = %d, want 40", len(got))
	}
}

func TestResolveBmadArtifactsDirReadsConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("implementation_artifacts: \"{project-root}/custom/specs\""), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	resolved := resolveBmadArtifactsDir(configPath, dir)
	if resolved != "custom/specs" {
		t.Errorf("resolved = %q, want custom/specs", resolved)
	}
}

func TestResolveBmadArtifactsDirFallback(t *testing.T) {
	base := "./output"
	resolved := resolveBmadArtifactsDir("nonexistent-config.yaml", base)
	want := filepath.Join(base, "_bmad-output", "implementation-artifacts")
	if resolved != want {
		t.Errorf("resolved = %q, want %q", resolved, want)
	}
}
