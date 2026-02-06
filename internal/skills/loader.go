package skills

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/lhdbsbz/aido/internal/agent"
)

// LoadFromDirs scans directories for skill folders (each containing SKILL.md).
// Returns skill entries with name, description, and path.
// Compatible with OpenClaw's skill format.
func LoadFromDirs(dirs []string) []agent.SkillEntry {
	var skills []agent.SkillEntry
	seen := make(map[string]bool)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillPath); err != nil {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}
			seen[name] = true

			desc := parseSkillDescription(skillPath)
			skills = append(skills, agent.SkillEntry{
				Name:        name,
				Description: desc,
				Path:        skillPath,
			})
		}
	}

	return skills
}

// parseSkillDescription reads the YAML frontmatter of a SKILL.md to extract the description.
func parseSkillDescription(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}

		if inFrontmatter && strings.HasPrefix(trimmed, "description:") {
			desc := strings.TrimPrefix(trimmed, "description:")
			desc = strings.TrimSpace(desc)
			desc = strings.Trim(desc, "\"'")
			return desc
		}
	}

	// Fallback: use first non-empty line after frontmatter as description
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			if len(line) > 200 {
				return line[:200] + "..."
			}
			return line
		}
	}

	return ""
}
