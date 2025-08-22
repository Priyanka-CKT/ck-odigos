package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// AgentStatusRequest represents the request body for agent status operations
type AgentStatusRequest struct {
	Status string `json:"status"`
	PodID  string `json:"podId"`
}

// AgentStatusResponse represents the response from agent status API
type AgentStatusResponse struct {
	Status string `json:"status"`
}

// buildAgentStatusURL constructs the URL for agent status API calls
func buildAgentStatusURL(serviceName string) string {
	nexusEndpoint := os.Getenv("CK_NEXUS_ENDPOINT")
	if nexusEndpoint == "" {
		return ""
	}
	return fmt.Sprintf("%s/api/agent-status/%s", nexusEndpoint, serviceName)
}

// GetAgentStatus retrieves the current status of an agent service
func GetAgentStatus(ctx context.Context, serviceName string) (string, error) {
	url := buildAgentStatusURL(serviceName)
	if url == "" {
		return "unknown", fmt.Errorf("CK_NEXUS_ENDPOINT not configured")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "unknown", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "unknown", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "unknown", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "unknown", fmt.Errorf("failed to read response: %w", err)
	}

	var response AgentStatusResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "unknown", fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Status, nil
}

// DisableAgentStatus sends a disable request to the agent status API
func DisableAgentStatus(ctx context.Context, serviceName string, podID *string) (bool, error) {
	url := buildAgentStatusURL(serviceName)
	if url == "" {
		return false, fmt.Errorf("CK_NEXUS_ENDPOINT not configured")
	}

	podIDValue := "string"
	if podID != nil && *podID != "" {
		podIDValue = *podID
	}

	requestBody := AgentStatusRequest{
		Status: "disabled",
		PodID:  podIDValue,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return false, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// If response is 2xx (success), return true without parsing response body
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}

	// For non-2xx responses, return false with error
	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// EnableAgentStatus sends an enable request to the agent status API
func EnableAgentStatus(ctx context.Context, serviceName string, podID *string) (bool, error) {
	url := buildAgentStatusURL(serviceName)
	if url == "" {
		return false, fmt.Errorf("CK_NEXUS_ENDPOINT not configured")
	}

	podIDValue := "string"
	if podID != nil && *podID != "" {
		podIDValue = *podID
	}

	requestBody := AgentStatusRequest{
		Status: "enabled",
		PodID:  podIDValue,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return false, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// If response is 2xx (success), return true without parsing response body
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}

	// For non-2xx responses, return false with error
	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}
