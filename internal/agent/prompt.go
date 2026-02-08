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
	"github.com/lhdbsbz/aido/internal/prompts"
	"github.com/lhdbsbz/aido/internal/skills"
)

const maxBootstrapChars = 20000

// PromptBuilder assembles the system prompt from multiple sections.
type PromptBuilder struct {
	Prompts     *prompts.Prompts
	AgentConfig *config.AgentConfig
	AgentID     string
	ToolDefs    []llm.ToolDef
	Skills      []skills.SkillEntry
	Workspace   string
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
	if b.Prompts.AuthorAndRepo != "" {
		sb.WriteString(b.Prompts.AuthorAndRepo)
	}
	sb.WriteString(b.Prompts.IdentityLine1)
	sb.WriteString(b.Prompts.IdentityLine2)
	sb.WriteString(b.Prompts.IdentityLine3)
}

func (b *PromptBuilder) writeTooling(sb *strings.Builder) {
	if len(b.ToolDefs) == 0 {
		return
	}
	sb.WriteString(b.Prompts.SectionToolsTitle)
	sb.WriteString(b.Prompts.ToolsIntro)
	for _, t := range b.ToolDefs {
		fmt.Fprintf(sb, "- **%s**: %s\n", t.Name, t.Description)
	}
	sb.WriteString(b.Prompts.ToolsHint)
	sb.WriteString(b.Prompts.ToolsChainHint)
}

func (b *PromptBuilder) writeSkills(sb *strings.Builder) {
	if len(b.Skills) == 0 {
		return
	}
	sb.WriteString(b.Prompts.SectionSkillsTitle)
	sb.WriteString("<available_skills>\n")
	for _, s := range b.Skills {
		fmt.Fprintf(sb, "  <skill>\n    <name>%s</name>\n    <description>%s</description>\n    <location>%s</location>\n  </skill>\n",
			s.Name, s.Description, s.Path)
	}
	sb.WriteString("</available_skills>\n\n")
	sb.WriteString(b.Prompts.SkillRelevantHint)
	sb.WriteString(b.Prompts.SkillOneAtATimeHint)
}

func (b *PromptBuilder) writeWorkspaceContext(sb *strings.Builder) {
	if b.Workspace == "" {
		return
	}

	sb.WriteString(b.Prompts.SectionWorkspaceTitle)
	fmt.Fprintf(sb, b.Prompts.WorkingDirLabel, b.Workspace)

	for _, bf := range b.Prompts.BootstrapFiles {
		path := filepath.Join(b.Workspace, bf.Name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(content))
		if text == "" {
			continue
		}
		text = truncateBootstrap(text, bf.Name, b.Prompts.TruncateBootstrapFmt)
		fmt.Fprintf(sb, "### %s\n\n", bf.Display)
		if bf.Note != "" {
			fmt.Fprintf(sb, "*%s*\n\n", bf.Note)
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
}

func (b *PromptBuilder) writeRuntime(sb *strings.Builder) {
	sb.WriteString(b.Prompts.SectionRuntimeTitle)
	fmt.Fprintf(sb, "- Agent: %s\n", b.AgentID)
	modelDisplay := b.AgentConfig.Model
	if b.AgentConfig.Provider != "" {
		modelDisplay = b.AgentConfig.Provider + "/" + b.AgentConfig.Model
	}
	fmt.Fprintf(sb, "- Model: %s\n", modelDisplay)
	fmt.Fprintf(sb, "- OS: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(sb, "- Time: %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	if b.Workspace != "" {
		fmt.Fprintf(sb, "- Workspace: %s\n", b.Workspace)
	}
	if path := config.Path(); path != "" {
		fmt.Fprintf(sb, "- Config file: %s\n", path)
		sb.WriteString(b.Prompts.ConfigFileHint)
	}
	sb.WriteString("\n")
}

// truncateBootstrap truncates a bootstrap file keeping 70% head + 20% tail.
func truncateBootstrap(content, filename, truncateFmt string) string {
	if len(content) <= maxBootstrapChars {
		return content
	}
	headSize := int(float64(maxBootstrapChars) * 0.7)
	tailSize := int(float64(maxBootstrapChars) * 0.2)
	return content[:headSize] +
		fmt.Sprintf(truncateFmt, filename) +
		content[len(content)-tailSize:]
}
