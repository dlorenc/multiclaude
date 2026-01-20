package coordination

import (
	"testing"
	"time"
)

func TestNewTaskManager(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	if tm == nil {
		t.Fatal("expected task manager")
	}
	if tm.tasks == nil {
		t.Error("expected tasks map to be initialized")
	}
}

func TestTaskManager_Create(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	req := &CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test task",
		Priority:    PriorityHigh,
		Labels:      map[string]string{"type": "feature"},
	}

	task, err := tm.Create(req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if task.ID == "" {
		t.Error("expected task ID")
	}
	if task.Repo != "my-repo" {
		t.Errorf("expected repo 'my-repo', got '%s'", task.Repo)
	}
	if task.Description != "Test task" {
		t.Errorf("expected description 'Test task', got '%s'", task.Description)
	}
	if task.Priority != PriorityHigh {
		t.Errorf("expected priority high, got '%s'", task.Priority)
	}
	if task.Status != TaskStatusPending {
		t.Errorf("expected status pending, got '%s'", task.Status)
	}
}

func TestTaskManager_CreateDefaultPriority(t *testing.T) {
	config := DefaultConfig()
	config.DefaultPriority = PriorityLow
	tm := NewTaskManager(config)

	task, err := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if task.Priority != PriorityLow {
		t.Errorf("expected priority low, got '%s'", task.Priority)
	}
}

func TestTaskManager_CreateValidation(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	// Missing repo
	_, err := tm.Create(&CreateTaskRequest{
		Description: "Test",
	})
	if err == nil {
		t.Error("expected error for missing repo")
	}

	// Missing description
	_, err = tm.Create(&CreateTaskRequest{
		Repo: "my-repo",
	})
	if err == nil {
		t.Error("expected error for missing description")
	}
}

func TestTaskManager_Get(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	got, err := tm.Get(task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != task.ID {
		t.Errorf("expected ID '%s', got '%s'", task.ID, got.ID)
	}
}

func TestTaskManager_GetNotFound(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	_, err := tm.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestTaskManager_List(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	tm.Create(&CreateTaskRequest{Repo: "repo1", Description: "Task 1"})
	tm.Create(&CreateTaskRequest{Repo: "repo2", Description: "Task 2"})

	tasks := tm.List("")
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	// Filter by status
	tasks = tm.List(TaskStatusPending)
	if len(tasks) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(tasks))
	}

	tasks = tm.List(TaskStatusCompleted)
	if len(tasks) != 0 {
		t.Errorf("expected 0 completed tasks, got %d", len(tasks))
	}
}

func TestTaskManager_ListPrioritySorting(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	tm.Create(&CreateTaskRequest{Repo: "r", Description: "Low", Priority: PriorityLow})
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	tm.Create(&CreateTaskRequest{Repo: "r", Description: "Critical", Priority: PriorityCritical})
	time.Sleep(1 * time.Millisecond)
	tm.Create(&CreateTaskRequest{Repo: "r", Description: "High", Priority: PriorityHigh})

	tasks := tm.List("")

	// Should be sorted: critical, high, low
	if tasks[0].Priority != PriorityCritical {
		t.Errorf("expected first task to be critical, got '%s'", tasks[0].Priority)
	}
	if tasks[1].Priority != PriorityHigh {
		t.Errorf("expected second task to be high, got '%s'", tasks[1].Priority)
	}
	if tasks[2].Priority != PriorityLow {
		t.Errorf("expected third task to be low, got '%s'", tasks[2].Priority)
	}
}

func TestTaskManager_GetPending(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task1, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Task 1",
		Labels:      map[string]string{"type": "feature"},
	})
	tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Task 2",
		Labels:      map[string]string{"type": "bugfix"},
	})

	// Claim one task
	tm.Claim(task1.ID, "node-1", "worker-1")

	// Get all pending (should be 1)
	tasks := tm.GetPending(nil)
	if len(tasks) != 1 {
		t.Errorf("expected 1 pending task, got %d", len(tasks))
	}

	// Get by label
	tasks = tm.GetPending(map[string]string{"type": "bugfix"})
	if len(tasks) != 1 {
		t.Errorf("expected 1 bugfix task, got %d", len(tasks))
	}
}

func TestTaskManager_Claim(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	claimed, err := tm.Claim(task.ID, "node-1", "worker-1")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if claimed.Status != TaskStatusClaimed {
		t.Errorf("expected status claimed, got '%s'", claimed.Status)
	}
	if claimed.ClaimedBy != "worker-1@node-1" {
		t.Errorf("expected claimed_by 'worker-1@node-1', got '%s'", claimed.ClaimedBy)
	}
	if claimed.ClaimedAt.IsZero() {
		t.Error("expected claimed_at to be set")
	}
}

func TestTaskManager_ClaimAlreadyClaimed(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	tm.Claim(task.ID, "node-1", "worker-1")

	// Try to claim again
	_, err := tm.Claim(task.ID, "node-2", "worker-2")
	if err == nil {
		t.Error("expected error claiming already claimed task")
	}
}

func TestTaskManager_ClaimNotFound(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	_, err := tm.Claim("nonexistent", "node-1", "worker-1")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestTaskManager_Update(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	// Claim first
	tm.Claim(task.ID, "node-1", "worker-1")

	// Update to running
	err := tm.Update(task.ID, &TaskUpdateRequest{
		Status: TaskStatusRunning,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := tm.Get(task.ID)
	if got.Status != TaskStatusRunning {
		t.Errorf("expected status running, got '%s'", got.Status)
	}
}

func TestTaskManager_UpdateComplete(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	tm.Claim(task.ID, "node-1", "worker-1")
	tm.Update(task.ID, &TaskUpdateRequest{Status: TaskStatusRunning})

	result := &TaskResult{
		Success:     true,
		PRNumber:    42,
		PRURL:       "https://github.com/owner/repo/pull/42",
		Duration:    5 * time.Minute,
		CompletedAt: time.Now(),
	}

	err := tm.Update(task.ID, &TaskUpdateRequest{
		Status: TaskStatusCompleted,
		Result: result,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := tm.Get(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Errorf("expected status completed, got '%s'", got.Status)
	}
	if got.Result == nil {
		t.Error("expected result to be set")
	}
	if got.Result.PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", got.Result.PRNumber)
	}
}

func TestTaskManager_UpdateInvalidTransition(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	// Try to go directly from pending to completed
	err := tm.Update(task.ID, &TaskUpdateRequest{
		Status: TaskStatusCompleted,
	})
	if err == nil {
		t.Error("expected error for invalid state transition")
	}
}

func TestTaskManager_Release(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	tm.Claim(task.ID, "node-1", "worker-1")

	err := tm.Release(task.ID)
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	got, _ := tm.Get(task.ID)
	if got.Status != TaskStatusPending {
		t.Errorf("expected status pending, got '%s'", got.Status)
	}
	if got.ClaimedBy != "" {
		t.Errorf("expected claimed_by to be empty, got '%s'", got.ClaimedBy)
	}
}

func TestTaskManager_ReleaseNotClaimed(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	err := tm.Release(task.ID)
	if err == nil {
		t.Error("expected error releasing unclaimed task")
	}
}

func TestTaskManager_Delete(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	err := tm.Delete(task.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = tm.Get(task.ID)
	if err == nil {
		t.Error("expected error getting deleted task")
	}
}

func TestTaskManager_GetStats(t *testing.T) {
	config := DefaultConfig()
	tm := NewTaskManager(config)

	// Create various tasks
	task1, _ := tm.Create(&CreateTaskRequest{Repo: "r", Description: "1"})
	task2, _ := tm.Create(&CreateTaskRequest{Repo: "r", Description: "2"})
	task3, _ := tm.Create(&CreateTaskRequest{Repo: "r", Description: "3"})

	// Claim and complete some
	tm.Claim(task1.ID, "n", "w")
	tm.Update(task1.ID, &TaskUpdateRequest{Status: TaskStatusRunning})
	tm.Update(task1.ID, &TaskUpdateRequest{Status: TaskStatusCompleted})

	tm.Claim(task2.ID, "n", "w")

	stats := tm.GetStats()
	counts := stats["counts"].(map[string]int)

	if counts["total"] != 3 {
		t.Errorf("expected total 3, got %d", counts["total"])
	}
	if counts["pending"] != 1 {
		t.Errorf("expected pending 1, got %d", counts["pending"])
	}
	if counts["claimed"] != 1 {
		t.Errorf("expected claimed 1, got %d", counts["claimed"])
	}
	if counts["completed"] != 1 {
		t.Errorf("expected completed 1, got %d", counts["completed"])
	}

	_ = task3 // unused but needed for test
}

func TestPriorityValue(t *testing.T) {
	tests := []struct {
		priority Priority
		want     int
	}{
		{PriorityCritical, 4},
		{PriorityHigh, 3},
		{PriorityMedium, 2},
		{PriorityLow, 1},
		{"unknown", 2}, // Default to medium
	}

	for _, tt := range tests {
		t.Run(string(tt.priority), func(t *testing.T) {
			got := priorityValue(tt.priority)
			if got != tt.want {
				t.Errorf("priorityValue(%s) = %d, want %d", tt.priority, got, tt.want)
			}
		})
	}
}

func TestValidStateTransition(t *testing.T) {
	tests := []struct {
		from, to TaskStatus
		want     bool
	}{
		{TaskStatusPending, TaskStatusClaimed, true},
		{TaskStatusPending, TaskStatusCompleted, false},
		{TaskStatusClaimed, TaskStatusRunning, true},
		{TaskStatusClaimed, TaskStatusPending, true},
		{TaskStatusRunning, TaskStatusCompleted, true},
		{TaskStatusRunning, TaskStatusFailed, true},
		{TaskStatusCompleted, TaskStatusPending, false},
		{TaskStatusFailed, TaskStatusPending, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := validStateTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("validStateTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTaskManager_ReleaseExpiredClaims(t *testing.T) {
	config := DefaultConfig()
	config.ClaimTimeout = 100 * time.Millisecond
	tm := NewTaskManager(config)

	task, _ := tm.Create(&CreateTaskRequest{
		Repo:        "my-repo",
		Description: "Test",
	})

	tm.Claim(task.ID, "node-1", "worker-1")

	// Force the ClaimedAt to be in the past
	tm.mu.Lock()
	tm.tasks[task.ID].ClaimedAt = time.Now().Add(-200 * time.Millisecond)
	tm.mu.Unlock()

	// Run the cleanup
	tm.releaseExpiredClaims(config.ClaimTimeout)

	got, _ := tm.Get(task.ID)
	if got.Status != TaskStatusOrphaned {
		t.Errorf("expected status orphaned, got '%s'", got.Status)
	}
}
