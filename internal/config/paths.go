package config

import "path/filepath"

// 所有 Aido 自控目录均固定在 home（~/.aido 或 AIDO_HOME）下，便于迁移与备份，不提供配置覆盖。

// Home 返回 Aido 根目录（ResolveHome()）。
func Home() string {
	return ResolveHome()
}

// Workspace 返回工作区根目录，固定为 home/workspace。
func Workspace() string {
	return filepath.Join(Home(), "workspace")
}

// DataDir 返回数据目录，固定为 home/data。
func DataDir() string {
	return filepath.Join(Home(), "data")
}

// SessionDir 返回会话存储目录，固定为 home/data/sessions。
func SessionDir() string {
	return filepath.Join(DataDir(), "sessions")
}

// CronDir 返回 cron 数据目录，固定为 home/data/cron。
func CronDir() string {
	return filepath.Join(DataDir(), "cron")
}

// CronJobsPath 返回 cron 任务文件路径，固定为 home/data/cron/jobs.json。
func CronJobsPath() string {
	return filepath.Join(CronDir(), "jobs.json")
}

// LogsDir 返回日志目录，固定为 home/logs。
func LogsDir() string {
	return filepath.Join(Home(), "logs")
}

// SkillsDir 返回技能目录，固定为 home/workspace/skills。
func SkillsDir() string {
	return filepath.Join(Workspace(), "skills")
}
