package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Job represents a scheduled task.
type Job struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // cron expression
	AgentID  string `json:"agentId"`
	Message  string `json:"message"`
	Enabled  bool   `json:"enabled"`
}

// RunRecord tracks a cron job execution.
type RunRecord struct {
	JobID     string    `json:"jobId"`
	StartedAt time.Time `json:"startedAt"`
	Duration  string    `json:"duration"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// TriggerFunc is called when a cron job fires.
type TriggerFunc func(ctx context.Context, agentID, message string) error

// Scheduler manages cron jobs and triggers agent runs.
type Scheduler struct {
	mu       sync.RWMutex
	cron     *cron.Cron
	jobs     map[string]*Job
	entryMap map[string]cron.EntryID // jobID → cron entry
	trigger  TriggerFunc
	dataDir  string
	runs     []RunRecord
}

func NewScheduler(dataDir string, trigger TriggerFunc) *Scheduler {
	return &Scheduler{
		cron:     cron.New(cron.WithSeconds()),
		jobs:     make(map[string]*Job),
		entryMap: make(map[string]cron.EntryID),
		trigger:  trigger,
		dataDir:  dataDir,
	}
}

// Load reads saved jobs from disk.
func (s *Scheduler) Load() error {
	path := filepath.Join(s.dataDir, "jobs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		s.jobs[job.ID] = job
		if job.Enabled {
			s.scheduleJob(job)
		}
	}
	return nil
}

// Save persists jobs to disk.
func (s *Scheduler) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	os.MkdirAll(s.dataDir, 0755)
	jobs := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "jobs.json"), data, 0644)
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
	slog.Info("cron scheduler started", "jobs", len(s.jobs))
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// Add creates a new cron job.
func (s *Scheduler) Add(name, schedule, agentID, message string) (*Job, error) {
	// Validate schedule
	if _, err := cron.ParseStandard(schedule); err != nil {
		// Try with seconds
		if _, err2 := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(schedule); err2 != nil {
			return nil, fmt.Errorf("invalid cron schedule: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	job := &Job{
		ID:       fmt.Sprintf("cron_%d", time.Now().UnixMilli()),
		Name:     name,
		Schedule: schedule,
		AgentID:  agentID,
		Message:  message,
		Enabled:  true,
	}
	s.jobs[job.ID] = job
	s.scheduleJob(job)

	if err := s.Save(); err != nil {
		slog.Warn("failed to save cron jobs", "error", err)
	}
	return job, nil
}

// Remove deletes a cron job.
func (s *Scheduler) Remove(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entryMap[jobID]; ok {
		s.cron.Remove(entryID)
		delete(s.entryMap, jobID)
	}
	delete(s.jobs, jobID)
	return s.Save()
}

// List returns all jobs.
func (s *Scheduler) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

// RunNow immediately triggers a job.
func (s *Scheduler) RunNow(jobID string) error {
	s.mu.RLock()
	job, ok := s.jobs[jobID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	go s.executeJob(job)
	return nil
}

func (s *Scheduler) scheduleJob(job *Job) {
	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		s.executeJob(job)
	})
	if err != nil {
		slog.Error("failed to schedule job", "job", job.Name, "error", err)
		return
	}
	s.entryMap[job.ID] = entryID
}

func (s *Scheduler) executeJob(job *Job) {
	start := time.Now()
	slog.Info("cron job executing", "job", job.Name, "agent", job.AgentID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := s.trigger(ctx, job.AgentID, job.Message)
	duration := time.Since(start)

	record := RunRecord{
		JobID:     job.ID,
		StartedAt: start,
		Duration:  duration.String(),
		Success:   err == nil,
	}
	if err != nil {
		record.Error = err.Error()
		slog.Error("cron job failed", "job", job.Name, "error", err, "duration", duration)
	} else {
		slog.Info("cron job completed", "job", job.Name, "duration", duration)
	}

	s.mu.Lock()
	s.runs = append(s.runs, record)
	if len(s.runs) > 1000 {
		s.runs = s.runs[len(s.runs)-500:]
	}
	s.mu.Unlock()
}
