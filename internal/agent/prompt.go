package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lhdbsbz/aido/internal/config"
	"github.com/lhdbsbz/aido/internal/llm"
	"github.com/lhdbsbz/aido/internal/skills"
)

const maxBootstrapChars = 20000

// PromptBuilder assembles the system prompt from multiple sections.
type PromptBuilder struct {
	AgentConfig *config.AgentConfig
	AgentID     string
	ToolDefs    []llm.ToolDef
	Skills      []skills.SkillEntry
	Workspace   string
	ConfigPath  string
}

// Build constructs the full system prompt.
func (b *PromptBuilder) Build() string {
	var sb strings.Builder

	b.writeIdentity(&sb)
	b.writeTooling(&sb)
	b.writeSkills(&sb)
	b.writeWorkspaceContext(&sb)
	b.writeRuntime(&sb)

	return sb.String()
}

func (b *PromptBuilder) writeIdentity(sb *strings.Builder) {
	sb.WriteString("You are Aido, an AI assistant running as a personal agent.\n")
	sb.WriteString("You help the user by using tools to accomplish tasks.\n")
	sb.WriteString("Always be direct, concise, and helpful.\n\n")
}

func (b *PromptBuilder) writeTooling(sb *strings.Builder) {
	if len(b.ToolDefs) == 0 {
		return
	}
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("You have the following tools available. Use them when needed:\n\n")
	for _, t := range b.ToolDefs {
		fmt.Fprintf(sb, "- **%s**: %s\n", t.Name, t.Description)
	}
	sb.WriteString("\nWhen you need to perform actions like reading files, executing commands, or searching the web, use the appropriate tool.\n")
	sb.WriteString("You can chain multiple tool calls in sequence to accomplish complex tasks.\n\n")
}

func (b *PromptBuilder) writeSkills(sb *strings.Builder) {
	if len(b.Skills) == 0 {
		return
	}
	sb.WriteString("## Available Skills\n\n")
	sb.WriteString("<available_skills>\n")
	for _, s := range b.Skills {
		fmt.Fprintf(sb, "  <skill>\n    <name>%s</name>\n    <description>%s</description>\n    <location>%s</location>\n  </skill>\n",
			s.Name, s.Description, s.Path)
	}
	sb.WriteString("</available_skills>\n\n")
	sb.WriteString("If a skill is relevant to the user's request, use the read_file tool to read its SKILL.md for detailed instructions.\n")
	sb.WriteString("Only read one skill at a time, and only when needed.\n\n")
}

func (b *PromptBuilder) writeWorkspaceContext(sb *strings.Builder) {
	if b.Workspace == "" {
		return
	}

	sb.WriteString("## Workspace\n\n")
	fmt.Fprintf(sb, "Working directory: %s\n\n", b.Workspace)

	bootstrapFiles := []struct {
		name    string
		display string
		note    string
	}{
		{"SOUL.md", "SOUL.md (Persona & Tone)", "Embody this persona and tone in all interactions."},
		{"AGENTS.md", "AGENTS.md (Operating Instructions)", "Follow these instructions."},
		{"TOOLS.md", "TOOLS.md (Tool Usage Notes)", ""},
		{"USER.md", "USER.md (User Profile)", ""},
	}

	for _, bf := range bootstrapFiles {
		path := filepath.Join(b.Workspace, bf.name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(content))
		if text == "" {
			continue
		}
		text = truncateBootstrap(text, bf.name)
		fmt.Fprintf(sb, "### %s\n\n", bf.display)
		if bf.note != "" {
			fmt.Fprintf(sb, "*%s*\n\n", bf.note)
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
}

func (b *PromptBuilder) writeRuntime(sb *strings.Builder) {
	sb.WriteString("## Runtime Information\n\n")
	fmt.Fprintf(sb, "- Agent: %s\n", b.AgentID)
	fmt.Fprintf(sb, "- Model: %s\n", b.AgentConfig.Model)
	fmt.Fprintf(sb, "- OS: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(sb, "- Time: %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	if b.Workspace != "" {
		fmt.Fprintf(sb, "- Workspace: %s\n", b.Workspace)
	}
	if b.ConfigPath != "" {
		fmt.Fprintf(sb, "- Config file: %s\n", b.ConfigPath)
		sb.WriteString("  You can read or edit this file to change agent behavior (e.g. model, tools, skills). Changes take effect after Aido restarts.\n")
	}
	sb.WriteString("\n")
}

// truncateBootstrap truncates a bootstrap file keeping 70% head + 20% tail.
func truncateBootstrap(content, filename string) string {
	if len(content) <= maxBootstrapChars {
		return content
	}
	headSize := int(float64(maxBootstrapChars) * 0.7)
	tailSize := int(float64(maxBootstrapChars) * 0.2)
	return content[:headSize] +
		fmt.Sprintf("\n\n[...truncated, read %s for full content...]\n\n", filename) +
		content[len(content)-tailSize:]
}
