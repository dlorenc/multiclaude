// Example webhook receiver for custom integrations
// This demonstrates how to build a service that receives multiclaude notifications
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Event represents a notification event from multiclaude
type Event struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Priority       string            `json:"priority"`
	Timestamp      time.Time         `json:"timestamp"`
	RepoName       string            `json:"repo_name"`
	AgentName      string            `json:"agent_name,omitempty"`
	AgentType      string            `json:"agent_type,omitempty"`
	Title          string            `json:"title"`
	Message        string            `json:"message"`
	Context        map[string]string `json:"context,omitempty"`
	ActionRequired bool              `json:"action_required"`
	ResponseID     string            `json:"response_id,omitempty"`
}

// WebhookPayload is the structure received from multiclaude
type WebhookPayload struct {
	Version   string    `json:"version"`
	Event     *Event    `json:"event"`
	Signature string    `json:"signature,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

var webhookSecret = os.Getenv("WEBHOOK_SECRET")

func main() {
	http.HandleFunc("/api/multiclaude/events", handleWebhook)
	http.HandleFunc("/health", handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting webhook receiver on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature if secret is configured
	if webhookSecret != "" {
		signature := r.Header.Get("X-Multiclaude-Signature")
		if !verifySignature(signature, body) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Process the event
	processEvent(payload.Event)

	// Respond with success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func verifySignature(signature string, body []byte) bool {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

func processEvent(event *Event) {
	// This is where you'd implement your custom logic
	// Examples:
	// - Store in database
	// - Send to another notification service
	// - Trigger an automation
	// - Update a dashboard

	log.Printf("Received event: type=%s, repo=%s, agent=%s, title=%s",
		event.Type, event.RepoName, event.AgentName, event.Title)

	switch event.Type {
	case "agent.question":
		// Handle questions - maybe send to a response queue
		log.Printf("Agent %s is asking: %s", event.AgentName, event.Message)
		if event.ActionRequired {
			log.Printf("Response ID: %s", event.ResponseID)
		}

	case "agent.completed":
		// Handle completions - maybe update a task tracker
		log.Printf("Agent %s completed their task", event.AgentName)

	case "agent.error":
		// Handle errors - maybe page an on-call engineer
		log.Printf("ALERT: Agent %s encountered an error: %s", event.AgentName, event.Message)

	case "agent.stuck":
		// Handle stuck agents - maybe auto-restart or notify
		log.Printf("WARNING: Agent %s appears stuck", event.AgentName)

	case "pr.created":
		// Handle PR creation - maybe run additional checks
		if prURL, ok := event.Context["pr_url"]; ok {
			log.Printf("PR created: %s", prURL)
		}

	case "ci.failed":
		// Handle CI failures - maybe auto-fix or notify
		log.Printf("CI failed for agent %s", event.AgentName)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Example: Sending a response back to multiclaude
// This can be used when running in interactive mode
func sendResponse(multiclaude_url, responseID, message string) error {
	payload := map[string]string{
		"response_id": responseID,
		"message":     message,
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post(multiclaude_url+"/api/v1/respond", "application/json",
		io.NopCloser(jsonReader(data)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response failed with status %d", resp.StatusCode)
	}
	return nil
}

func jsonReader(data []byte) io.Reader {
	return io.NopCloser(io.Reader(nil))
}
