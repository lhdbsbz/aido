package skills

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// SkillEntry represents a loaded skill (name, description, path to SKILL.md).
type SkillEntry struct {
	Name        string
	Description string
	Path        string
}

// LoadFromDirs scans directories for skill folders (each containing SKILL.md).
// Expected layout: <dir>/<skill-name>/SKILL.md; a SKILL.md placed directly under dir is ignored.
// Returns skill entries with name, description, and path. See README "技能目录 (Skills)" for details.
func LoadFromDirs(dirs []string) []SkillEntry {
	var list []SkillEntry
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
			list = append(list, SkillEntry{
				Name:        name,
				Description: desc,
				Path:        skillPath,
			})
		}
	}

	return list
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
