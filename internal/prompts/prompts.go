package prompts

// BootstrapFile describes a workspace bootstrap file (SOUL.md, AGENTS.md, etc.).
type BootstrapFile struct {
	Name    string
	Display string
	Note    string
}

// Prompts holds all user-facing prompt strings for a locale.
type Prompts struct {
	AuthorAndRepo string // author and GitHub repo URL
	IdentityLine1 string
	IdentityLine2 string
	IdentityLine3 string

	SectionToolsTitle string
	ToolsIntro        string
	ToolsHint         string
	ToolsChainHint    string

	SectionSkillsTitle   string
	SkillRelevantHint    string
	SkillOneAtATimeHint  string

	SectionWorkspaceTitle string
	WorkingDirLabel       string

	BootstrapFiles []BootstrapFile

	SectionRuntimeTitle string
	ConfigFileHint      string
	TruncateBootstrapFmt string

	SummarizePromptTemplate string
}

// Get returns prompts for the given locale. Only "en" uses English; empty or unknown defaults to Chinese ("zh").
func Get(locale string) *Prompts {
	if locale == "en" {
		return PromptsEN
	}
	return PromptsZH
}
