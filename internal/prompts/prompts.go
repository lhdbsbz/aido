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
	IdentityLine1    string
	IdentityLine2    string
	IdentityLine3    string
	IdentityEnvLine  string // optional: "You run inside Aido: ..."

	SectionToolsTitle string
	ToolsEnvHint      string // optional: tools executed by Aido, call by name
	ToolsIntro        string
	ToolsHint         string
	ToolsChainHint    string

	SectionSkillsTitle   string
	SkillRelevantHint    string
	SkillOneAtATimeHint  string

	SectionWorkspaceTitle string
	WorkingDirLabel       string
	WorkingDirNote        string // optional: workspace scope, skills from fixed dir

	BootstrapFiles []BootstrapFile

	SectionRuntimeTitle    string
	DirLayoutRules         string // directory discipline: home, temp, store; empty = omit
	ConfigFileHint         string
	ConfigTroubleshootHint string // optional: when in doubt, read config or ask user
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
