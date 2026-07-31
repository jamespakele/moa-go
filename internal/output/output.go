// Package output writes MoA synthesis results to markdown files.
package output

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// resolveBmadArtifactsDir reads bmad's config.yaml for implementation_artifacts
// and falls back to {fallbackBase}/_bmad-output/implementation-artifacts/.
func resolveBmadArtifactsDir(bmadConfigPath string, fallbackBase string) string {
	if _, err := os.Stat(bmadConfigPath); err == nil {
		content, err := os.ReadFile(bmadConfigPath)
		if err == nil {
			for _, line := range strings.Split(string(content), "\n") {
				trimmed := strings.TrimSpace(line)
				if rest, ok := strings.CutPrefix(trimmed, "implementation_artifacts:"); ok {
					value := strings.Trim(strings.TrimSpace(rest), `"`)
					value = strings.TrimPrefix(value, "{project-root}/")
					return value
				}
			}
		}
	}
	return filepath.Join(fallbackBase, "_bmad-output", "implementation-artifacts")
}

// slugify converts a prompt into a kebab-case slug, trimmed to 40 chars.
func slugify(input string) string {
	var sb strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(unicode.ToLower(r))
		} else {
			sb.WriteRune('-')
		}
	}
	collapsed := regexp.MustCompile("-+")
	slug := collapsed.ReplaceAllString(sb.String(), "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return strings.Trim(slug, "-")
}

// wrapBmadSpec wraps the synthesis in a bmad-quick-dev spec template.
func wrapBmadSpec(content, prompt string) string {
	date := time.Now().Format("2006-01-02")
	title := prompt
	if len(title) > 80 {
		title = title[:80]
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: 'MoA: %s'\n", strings.TrimSpace(title)))
	b.WriteString("type: 'feature'\n")
	b.WriteString(fmt.Sprintf("created: '%s'\n", date))
	b.WriteString("status: 'draft'\n")
	b.WriteString("review_loop_iteration: 0\n")
	b.WriteString("context: []\n")
	b.WriteString("---\n\n")
	b.WriteString("<frozen-after-approval reason=\"MoA deliberation output — human reviews before bmad-quick-dev plans\">\n\n")
	b.WriteString("## Intent\n\n")
	b.WriteString("This spec was produced by moa-rust (Mixture of Agents). Multiple reference\n")
	b.WriteString("models proposed, an adversarial aggregator reviewed, and the deliberation\n")
	b.WriteString("converged. The synthesis below is the starting intent for bmad-quick-dev.\n\n")
	b.WriteString("### MoA Synthesis\n\n")
	b.WriteString(content)
	b.WriteString("\n\n")
	b.WriteString("</frozen-after-approval>\n\n")
	b.WriteString("## Code Map\n\n")
	b.WriteString("<!-- Agent-populated during planning -->\n\n")
	b.WriteString("## Tasks & Acceptance\n\n")
	b.WriteString("**Execution:**\n")
	b.WriteString("- [ ] `FILE` -- ACTION -- RATIONALE\n\n")
	b.WriteString("**Acceptance Criteria:**\n")
	b.WriteString("- Given PRECONDITION, when ACTION, then EXPECTED_RESULT\n\n")
	b.WriteString("## Spec Change Log\n\n")
	b.WriteString("## Verification\n\n")
	b.WriteString("**Commands:**\n")
	b.WriteString("- `go test ./...` -- expected: all tests pass\n")

	return b.String()
}

// WriteOutput writes synthesized content to a markdown file.
// Normal mode:   moa-{YYYYMMDD-HHMMSS}-{sha256-prefix}.md in outputDir.
// BMad mode:     spec-moa-{slug}.md in implementation artifacts dir.
func WriteOutput(content, outputDir, prompt string, bmadCompatible bool, bmadConfigPath string) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	hash := sha256.Sum256([]byte(prompt))
	shortHash := hex.EncodeToString(hash[:4])

	var finalContent, targetDir, filename string
	if bmadCompatible {
		finalContent = wrapBmadSpec(content, prompt)
		slug := slugify(prompt)
		targetDir = resolveBmadArtifactsDir(bmadConfigPath, outputDir)
		filename = fmt.Sprintf("spec-moa-%s.md", slug)
	} else {
		finalContent = content
		targetDir = outputDir
		filename = fmt.Sprintf("moa-%s-%s.md", timestamp, shortHash)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(targetDir, filename)
	if err := os.WriteFile(path, []byte(finalContent), 0644); err != nil {
		return "", err
	}
	return path, nil
}
