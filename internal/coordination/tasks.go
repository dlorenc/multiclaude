package coordination

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskManager manages distributable tasks.
type TaskManager struct {
	config *Config

	tasks map[string]*Task
	mu    sync.RWMutex
}

// NewTaskManager creates a new task manager.
func NewTaskManager(config *Config) *TaskManager {
	return &TaskManager{
		config: config,
		tasks:  make(map[string]*Task),
	}
}

// Create creates a new task.
func (tm *TaskManager) Create(req *CreateTaskRequest) (*Task, error) {
	if req.Repo == "" {
		return nil, fmt.Errorf("repo is required")
	}
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	priority := req.Priority
	if priority == "" {
		priority = tm.config.DefaultPriority
	}

	task := &Task{
		ID:          fmt.Sprintf("task-%s", uuid.New().String()[:8]),
		Repo:        req.Repo,
		Description: req.Description,
		Priority:    priority,
		Labels:      req.Labels,
		Status:      TaskStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	tm.tasks[task.ID] = task

	return task, nil
}

// Get retrieves a task by ID.
func (tm *TaskManager) Get(taskID string) (*Task, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	// Return a copy
	taskCopy := *task
	return &taskCopy, nil
}

// List returns all tasks, optionally filtered by status.
func (tm *TaskManager) List(status TaskStatus) []Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]Task, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		if status == "" || task.Status == status {
			tasks = append(tasks, *task)
		}
	}

	// Sort by priority (critical > high > medium > low) then by creation time
	sort.Slice(tasks, func(i, j int) bool {
		pi := priorityValue(tasks[i].Priority)
		pj := priorityValue(tasks[j].Priority)
		if pi != pj {
			return pi > pj
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})

	return tasks
}

// GetPending returns pending tasks matching the given labels.
func (tm *TaskManager) GetPending(labels map[string]string) []Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]Task, 0)
	for _, task := range tm.tasks {
		if task.Status != TaskStatusPending {
			continue
		}
		if matchesLabels(task.Labels, labels) {
			tasks = append(tasks, *task)
		}
	}

	// Sort by priority then creation time
	sort.Slice(tasks, func(i, j int) bool {
		pi := priorityValue(tasks[i].Priority)
		pj := priorityValue(tasks[j].Priority)
		if pi != pj {
			return pi > pj
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})

	return tasks
}

// Claim attempts to claim a task for a worker.
func (tm *TaskManager) Claim(taskID, nodeID, workerName string) (*Task, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	if task.Status != TaskStatusPending {
		if task.ClaimedBy != "" {
			return nil, fmt.Errorf("task already claimed by %s", task.ClaimedBy)
		}
		return nil, fmt.Errorf("task is not pending (status: %s)", task.Status)
	}

	// Claim the task
	task.Status = TaskStatusClaimed
	task.ClaimedBy = fmt.Sprintf("%s@%s", workerName, nodeID)
	task.ClaimedAt = time.Now()
	task.UpdatedAt = time.Now()

	taskCopy := *task
	return &taskCopy, nil
}

// Update updates a task's status.
func (tm *TaskManager) Update(taskID string, req *TaskUpdateRequest) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %q not found", taskID)
	}

	// Validate state transition
	if !validStateTransition(task.Status, req.Status) {
		return fmt.Errorf("invalid state transition from %s to %s", task.Status, req.Status)
	}

	task.Status = req.Status
	task.UpdatedAt = time.Now()

	if req.Result != nil {
		task.Result = req.Result
	}

	return nil
}

// Release releases a claimed task back to pending.
func (tm *TaskManager) Release(taskID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %q not found", taskID)
	}

	if task.Status != TaskStatusClaimed && task.Status != TaskStatusRunning {
		return fmt.Errorf("task is not claimed (status: %s)", task.Status)
	}

	task.Status = TaskStatusPending
	task.ClaimedBy = ""
	task.ClaimedAt = time.Time{}
	task.UpdatedAt = time.Now()

	return nil
}

// Delete removes a task.
func (tm *TaskManager) Delete(taskID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tasks[taskID]; !exists {
		return fmt.Errorf("task %q not found", taskID)
	}

	delete(tm.tasks, taskID)
	return nil
}

// StartCleanup starts the background cleanup goroutine.
func (tm *TaskManager) StartCleanup(ctx context.Context, claimTimeout time.Duration) {
	ticker := time.NewTicker(claimTimeout / 3)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tm.releaseExpiredClaims(claimTimeout)
		case <-ctx.Done():
			return
		}
	}
}

// releaseExpiredClaims releases tasks that have been claimed for too long.
func (tm *TaskManager) releaseExpiredClaims(timeout time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cutoff := time.Now().Add(-timeout)

	for _, task := range tm.tasks {
		if task.Status == TaskStatusClaimed && !task.ClaimedAt.IsZero() {
			if task.ClaimedAt.Before(cutoff) {
				task.Status = TaskStatusOrphaned
				task.UpdatedAt = time.Now()
			}
		}
	}
}

// GetStats returns task statistics.
func (tm *TaskManager) GetStats() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	stats := map[string]int{
		"total":     0,
		"pending":   0,
		"claimed":   0,
		"running":   0,
		"completed": 0,
		"failed":    0,
		"orphaned":  0,
	}

	for _, task := range tm.tasks {
		stats["total"]++
		switch task.Status {
		case TaskStatusPending:
			stats["pending"]++
		case TaskStatusClaimed:
			stats["claimed"]++
		case TaskStatusRunning:
			stats["running"]++
		case TaskStatusCompleted:
			stats["completed"]++
		case TaskStatusFailed:
			stats["failed"]++
		case TaskStatusOrphaned:
			stats["orphaned"]++
		}
	}

	return map[string]interface{}{
		"counts": stats,
	}
}

// priorityValue converts priority to a numeric value for sorting.
func priorityValue(p Priority) int {
	switch p {
	case PriorityCritical:
		return 4
	case PriorityHigh:
		return 3
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 1
	default:
		return 2 // Default to medium
	}
}

// validStateTransition checks if a state transition is valid.
func validStateTransition(from, to TaskStatus) bool {
	validTransitions := map[TaskStatus][]TaskStatus{
		TaskStatusPending:   {TaskStatusClaimed},
		TaskStatusClaimed:   {TaskStatusRunning, TaskStatusPending, TaskStatusOrphaned},
		TaskStatusRunning:   {TaskStatusCompleted, TaskStatusFailed, TaskStatusOrphaned},
		TaskStatusOrphaned:  {TaskStatusPending, TaskStatusClaimed},
		TaskStatusCompleted: {}, // Terminal
		TaskStatusFailed:    {TaskStatusPending}, // Can retry
	}

	valid, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, v := range valid {
		if v == to {
			return true
		}
	}

	return false
}
