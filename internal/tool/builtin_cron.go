package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// CronJob is a single cron job entry.
type CronJob struct {
	ID       string `json:"id"`
	Schedule string `json:"schedule"` // cron expression (e.g. "0 9 * * *" for daily 9am)
	Name     string `json:"name"`    // optional description
}

// cronStore persists jobs to a JSON file.
type cronStore struct {
	path string
	mu   sync.Mutex
}

func newCronStore(path string) *cronStore {
	return &cronStore{path: path}
}

func (s *cronStore) load() ([]CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var jobs []CronJob
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *cronStore) save(jobs []CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// CronListTool lists all cron jobs.
type CronListTool struct{ store *cronStore }

func (t *CronListTool) Name() string        { return "cron_list" }
func (t *CronListTool) Description() string { return "List all scheduled cron jobs (id, schedule, name)." }
func (t *CronListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

func (t *CronListTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	jobs, err := t.store.load()
	if err != nil {
		return "", err
	}
	if len(jobs) == 0 {
		return "No cron jobs.", nil
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	var out string
	for _, j := range jobs {
		out += fmt.Sprintf("%s  %s  %s\n", j.ID, j.Schedule, j.Name)
	}
	return strings.TrimSuffix(out, "\n"), nil
}

// CronAddTool adds a cron job.
type CronAddTool struct{ store *cronStore }

func (t *CronAddTool) Name() string        { return "cron_add" }
func (t *CronAddTool) Description() string { return "Add a scheduled cron job. Schedule is a cron expression (e.g. '0 9 * * *' for daily at 9am). ID is optional; if omitted a unique id is generated." }
func (t *CronAddTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Unique job id (optional, auto-generated if omitted)"},
			"schedule": {"type": "string", "description": "Cron expression (e.g. 0 9 * * * for daily 9am)"},
			"name": {"type": "string", "description": "Optional human-readable name"}
		},
		"required": ["schedule"]
	}`)
}

func (t *CronAddTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID       string `json:"id"`
		Schedule string `json:"schedule"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Schedule == "" {
		return "", fmt.Errorf("schedule is required")
	}
	jobs, err := t.store.load()
	if err != nil {
		return "", err
	}
	existingIDs := make(map[string]bool)
	for _, j := range jobs {
		existingIDs[j.ID] = true
	}
	id := p.ID
	if id == "" {
		for i := 0; ; i++ {
			id = fmt.Sprintf("job_%d", i)
			if !existingIDs[id] {
				break
			}
		}
	}
	if existingIDs[id] {
		return "", fmt.Errorf("cron job id already exists: %s", id)
	}
	jobs = append(jobs, CronJob{ID: id, Schedule: p.Schedule, Name: p.Name})
	if err := t.store.save(jobs); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added cron job %s (schedule: %s)", id, p.Schedule), nil
}

// CronRemoveTool removes a cron job by id.
type CronRemoveTool struct{ store *cronStore }

func (t *CronRemoveTool) Name() string        { return "cron_remove" }
func (t *CronRemoveTool) Description() string { return "Remove a cron job by id. Use cron_list to see ids." }
func (t *CronRemoveTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Job id to remove"}
		},
		"required": ["id"]
	}`)
}

func (t *CronRemoveTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	jobs, err := t.store.load()
	if err != nil {
		return "", err
	}
	var newJobs []CronJob
	for _, j := range jobs {
		if j.ID != p.ID {
			newJobs = append(newJobs, j)
		}
	}
	if len(newJobs) == len(jobs) {
		return "", fmt.Errorf("cron job not found: %s", p.ID)
	}
	if err := t.store.save(newJobs); err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed cron job %s", p.ID), nil
}

// RegisterCronTools registers cron_list, cron_add, cron_remove. storePath is the path to jobs.json (e.g. {home}/data/cron/jobs.json).
func RegisterCronTools(r *Registry, storePath string) {
	store := newCronStore(storePath)
	r.Register(&CronListTool{store: store})
	r.Register(&CronAddTool{store: store})
	r.Register(&CronRemoveTool{store: store})
}
