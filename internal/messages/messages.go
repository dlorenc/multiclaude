package messages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle state of a message
type Status string

const (
	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusRead      Status = "read"
	StatusAcked     Status = "acked"
)

// Message represents a message between agents
type Message struct {
	ID        string     `json:"id"`
	From      string     `json:"from"`
	To        string     `json:"to"`
	Timestamp time.Time  `json:"timestamp"`
	Body      string     `json:"body"`
	Status    Status     `json:"status"`
	AckedAt   *time.Time `json:"acked_at,omitempty"`
}

// Manager handles message filesystem operations
type Manager struct {
	messagesRoot string
}

// NewManager creates a new message manager
func NewManager(messagesRoot string) *Manager {
	return &Manager{messagesRoot: messagesRoot}
}

// Send creates a new message file
func (m *Manager) Send(repoName, from, to, body string) (*Message, error) {
	msg := &Message{
		ID:        fmt.Sprintf("msg-%s", uuid.New().String()[:13]),
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Body:      body,
		Status:    StatusPending,
	}

	if err := m.write(repoName, to, msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// List returns all messages for an agent
func (m *Manager) List(repoName, agentName string) ([]*Message, error) {
	dir := m.agentDir(repoName, agentName)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Message{}, nil
		}
		return nil, fmt.Errorf("failed to read messages directory: %w", err)
	}

	var messages []*Message
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		msg, err := m.read(repoName, agentName, entry.Name())
		if err != nil {
			// Skip invalid messages
			continue
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// Get retrieves a specific message by ID
func (m *Manager) Get(repoName, agentName, messageID string) (*Message, error) {
	filename := messageID + ".json"
	return m.read(repoName, agentName, filename)
}

// UpdateStatus updates the status of a message
func (m *Manager) UpdateStatus(repoName, agentName, messageID string, status Status) error {
	msg, err := m.Get(repoName, agentName, messageID)
	if err != nil {
		return err
	}

	msg.Status = status
	if status == StatusAcked {
		now := time.Now()
		msg.AckedAt = &now
	}

	return m.write(repoName, agentName, msg)
}

// Ack marks a message as acknowledged
func (m *Manager) Ack(repoName, agentName, messageID string) error {
	return m.UpdateStatus(repoName, agentName, messageID, StatusAcked)
}

// Delete removes a message file
func (m *Manager) Delete(repoName, agentName, messageID string) error {
	path := filepath.Join(m.agentDir(repoName, agentName), messageID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	return nil
}

// DeleteAcked removes all acknowledged messages for an agent
func (m *Manager) DeleteAcked(repoName, agentName string) (int, error) {
	messages, err := m.List(repoName, agentName)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, msg := range messages {
		if msg.Status == StatusAcked {
			if err := m.Delete(repoName, agentName, msg.ID); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// ListUnread returns all unread messages for an agent
func (m *Manager) ListUnread(repoName, agentName string) ([]*Message, error) {
	messages, err := m.List(repoName, agentName)
	if err != nil {
		return nil, err
	}

	var unread []*Message
	for _, msg := range messages {
		if msg.Status == StatusPending || msg.Status == StatusDelivered {
			unread = append(unread, msg)
		}
	}

	return unread, nil
}

// agentDir returns the directory path for an agent's messages
func (m *Manager) agentDir(repoName, agentName string) string {
	return filepath.Join(m.messagesRoot, repoName, agentName)
}

// ensureAgentDir ensures the agent's message directory exists
func (m *Manager) ensureAgentDir(repoName, agentName string) error {
	dir := m.agentDir(repoName, agentName)
	return os.MkdirAll(dir, 0755)
}

// write writes a message to disk
func (m *Manager) write(repoName, agentName string, msg *Message) error {
	if err := m.ensureAgentDir(repoName, agentName); err != nil {
		return err
	}

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	path := filepath.Join(m.agentDir(repoName, agentName), msg.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write message file: %w", err)
	}

	return nil
}

// read reads a message from disk
func (m *Manager) read(repoName, agentName, filename string) (*Message, error) {
	path := filepath.Join(m.agentDir(repoName, agentName), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read message file: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &msg, nil
}

// CleanupOrphaned removes message directories for non-existent agents
func (m *Manager) CleanupOrphaned(repoName string, validAgents []string) (int, error) {
	repoDir := filepath.Join(m.messagesRoot, repoName)

	entries, err := os.ReadDir(repoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read repo messages dir: %w", err)
	}

	validAgentMap := make(map[string]bool)
	for _, agent := range validAgents {
		validAgentMap[agent] = true
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if !validAgentMap[entry.Name()] {
			// This is an orphaned agent directory
			path := filepath.Join(repoDir, entry.Name())
			if err := os.RemoveAll(path); err == nil {
				count++
			}
		}
	}

	return count, nil
}
