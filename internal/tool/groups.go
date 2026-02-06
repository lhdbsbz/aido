package tool

// Groups defines tool group shorthands for policy configuration.
var Groups = map[string][]string{
	"group:fs": {
		"read_file", "write_file", "edit_file",
		"list_dir", "grep", "find_files",
	},
	"group:runtime": {
		"exec", "process",
	},
	"group:web": {
		"web_search", "web_fetch",
	},
	"group:sessions": {
		"session_status", "sessions_list", "sessions_history",
		"sessions_send", "sessions_spawn",
	},
	"group:messaging": {
		"message",
	},
	"group:memory": {
		"memory_search", "memory_get",
	},
	"group:cron": {
		"cron_list", "cron_add", "cron_remove",
	},
}
