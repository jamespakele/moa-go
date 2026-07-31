package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpakele/moa-go/internal/callctx"
)

// canonicalizeAndVerifyPath resolves the path to an absolute, symlink-free
// location and ensures it remains inside the project root. It mirrors the
// behavior of the project root containment.
func canonicalizeAndVerifyPath(path string, projectRoot string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("Path does not exist or cannot be resolved: %s: %v", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("Path does not exist or cannot be resolved: %s: %v", path, err)
	}
	sep := string(os.PathSeparator)
	if !strings.HasPrefix(resolved+sep, projectRoot+sep) {
		return "", fmt.Errorf("Path %s escapes project root", path)
	}
	return resolved, nil
}

// buildPreamble constructs the system prompt from system_prompt, skills, and
// append_system_prompt. Skill directories are read from their SKILL.md entry only.
func buildPreamble(ctx callctx.CallContext, projectRoot string) (string, error) {
	var text strings.Builder

	// (1) system_prompt
	hasSystemPrompt := ctx.SystemPrompt != ""
	if hasSystemPrompt {
		text.WriteString(ctx.SystemPrompt)
	}

	// (2) skills
	anySkillOK := false
	anySkillErr := false
	for _, skillPath := range ctx.Skills {
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[moa] Warning: skill file not found: %s\n", skillPath)
			anySkillErr = true
			continue
		}
		verifiedPath, err := canonicalizeAndVerifyPath(skillPath, projectRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[moa] Warning: skill path rejected: %s\n", err)
			anySkillErr = true
			continue
		}
		info, err := os.Stat(verifiedPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[moa] Warning: failed to stat skill file %s: %v\n", skillPath, err)
			anySkillErr = true
			continue
		}
		if info.IsDir() {
			skillMd := filepath.Join(verifiedPath, "SKILL.md")
			if _, err := os.Stat(skillMd); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "[moa] Warning: skill directory has no SKILL.md: %s\n", skillPath)
				anySkillErr = true
				continue
			}
			verifiedSkillMd, err := filepath.EvalSymlinks(skillMd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[moa] Warning: failed to resolve SKILL.md %s: %v\n", skillMd, err)
				anySkillErr = true
				continue
			}
			if !strings.HasPrefix(verifiedSkillMd+string(os.PathSeparator), projectRoot+string(os.PathSeparator)) {
				fmt.Fprintf(os.Stderr, "[moa] Warning: skill file escapes project root: %s\n", skillMd)
				anySkillErr = true
				continue
			}
			content, err := os.ReadFile(verifiedSkillMd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[moa] Warning: failed to read skill file %s: %v\n", verifiedSkillMd, err)
				anySkillErr = true
				continue
			}
			if text.Len() > 0 {
				text.WriteString("\n\n---\n\n")
			}
			text.Write(content)
			anySkillOK = true
		} else {
			content, err := os.ReadFile(verifiedPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[moa] Warning: failed to read skill file %s: %v\n", skillPath, err)
				anySkillErr = true
				continue
			}
			if text.Len() > 0 {
				text.WriteString("\n\n---\n\n")
			}
			text.Write(content)
			anySkillOK = true
		}
	}

	// Hard error only when there's no system_prompt fallback and no skill loaded
	// successfully. With a system_prompt set, a skill failure is a warning.
	if !hasSystemPrompt && !anySkillOK && anySkillErr && len(ctx.Skills) > 0 {
		return "", fmt.Errorf("All skill files failed to read")
	}

	// (3) append_system_prompt
	if ctx.AppendSystemPrompt != "" {
		if text.Len() > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString(ctx.AppendSystemPrompt)
	}

	return text.String(), nil
}
