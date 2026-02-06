package tool

// PolicyLayer represents one layer of tool access control.
type PolicyLayer struct {
	Profile string   // "minimal" | "coding" | "messaging" | "full"
	Allow   []string // allowlist (supports "group:fs" etc.)
	Deny    []string // denylist (always wins)
}

// Policy evaluates multi-layer tool access control.
// All layers must pass for a tool to be allowed.
type Policy struct {
	layers []PolicyLayer
}

func NewPolicy(layers ...PolicyLayer) *Policy {
	return &Policy{layers: layers}
}

// IsAllowed checks if a tool name passes all policy layers.
func (p *Policy) IsAllowed(toolName string) bool {
	for _, layer := range p.layers {
		if !isAllowedByLayer(toolName, layer) {
			return false
		}
	}
	return true
}

func isAllowedByLayer(toolName string, layer PolicyLayer) bool {
	expandedAllow := expandNames(layer.Allow)
	expandedDeny := expandNames(layer.Deny)

	// Apply profile defaults first
	if layer.Profile != "" {
		profileTools := profileDefaults(layer.Profile)
		if profileTools != nil {
			expandedAllow = append(profileTools, expandedAllow...)
		}
	}

	// Deny always wins
	if matchAny(toolName, expandedDeny) {
		return false
	}

	// If allow is empty (and no profile), allow everything
	if len(expandedAllow) == 0 {
		return true
	}

	// Allow list is non-empty: must match
	return matchAny(toolName, expandedAllow)
}

func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if p == name || p == "*" {
			return true
		}
	}
	return false
}

// expandNames expands group references like "group:fs" into individual tool names.
func expandNames(names []string) []string {
	var expanded []string
	for _, name := range names {
		if group, ok := Groups[name]; ok {
			expanded = append(expanded, group...)
		} else {
			expanded = append(expanded, name)
		}
	}
	return expanded
}

// profileDefaults returns the default tool list for a profile.
func profileDefaults(profile string) []string {
	switch profile {
	case "minimal":
		return []string{"session_status"}
	case "coding":
		return expandNames([]string{"group:fs", "group:runtime", "group:sessions", "group:web"})
	case "messaging":
		return expandNames([]string{"group:messaging", "session_status"})
	case "full":
		return nil // nil means no restriction (allow all)
	default:
		return nil
	}
}

// ResolvePolicyLayers builds the multi-layer policy stack from config.
func ResolvePolicyLayers(global PolicyLayer, providerLayer, agentLayer *PolicyLayer) *Policy {
	layers := []PolicyLayer{global}
	if providerLayer != nil {
		layers = append(layers, *providerLayer)
	}
	if agentLayer != nil {
		layers = append(layers, *agentLayer)
	}
	return NewPolicy(layers...)
}
