package tool

import "context"

type contextKey string

const runInfoKey contextKey = "runInfo"

// RunInfo holds current run context (session, agent, model, workspace) for tools that need it.
type RunInfo struct {
	SessionKey string
	AgentID    string
	Model      string
	Workspace  string
}

// WithRunInfo attaches RunInfo to ctx. Used by the agent loop before executing tools.
func WithRunInfo(ctx context.Context, info RunInfo) context.Context {
	return context.WithValue(ctx, runInfoKey, info)
}

// RunInfoFromContext returns RunInfo from ctx if present.
func RunInfoFromContext(ctx context.Context) (RunInfo, bool) {
	info, ok := ctx.Value(runInfoKey).(RunInfo)
	return info, ok
}
