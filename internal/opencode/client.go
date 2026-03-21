package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Session struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Message struct {
	Info  MessageInfo `json:"info"`
	Parts []Part      `json:"parts"`
}

type MessageInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	Role      string `json:"role"`
}

type Part struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
}

type SendMessageRequest struct {
	Parts []Part `json:"parts"`
	Model string `json:"model,omitempty"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/global/health")
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Healthy bool `json:"healthy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("health check: decoding response: %w", err)
	}

	if !result.Healthy {
		return fmt.Errorf("health check: server reports unhealthy")
	}

	return nil
}

func (c *Client) CreateSession(title string) (*Session, error) {
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return nil, fmt.Errorf("create session: marshaling request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/session", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create session: status %d: %s", resp.StatusCode, string(respBody))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("create session: decoding response: %w", err)
	}

	return &session, nil
}

func (c *Client) SendMessage(sessionID, prompt, model string) (*Message, error) {
	reqBody := SendMessageRequest{
		Parts: []Part{{Type: "text", Content: prompt}},
		Model: model,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("send message: marshaling request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/session/"+sessionID+"/message",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("send message request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("send message: status %d: %s", resp.StatusCode, string(respBody))
	}

	var msg Message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("send message: decoding response: %w", err)
	}

	return &msg, nil
}

func (c *Client) SendMessageAsync(sessionID, prompt, model string) error {
	reqBody := SendMessageRequest{
		Parts: []Part{{Type: "text", Content: prompt}},
		Model: model,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("send message async: marshaling request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/session/"+sessionID+"/prompt_async",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("send message async request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message async: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) AbortSession(sessionID string) error {
	resp, err := c.httpClient.Post(
		c.baseURL+"/session/"+sessionID+"/abort",
		"application/json",
		nil,
	)
	if err != nil {
		return fmt.Errorf("abort session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("abort session: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) DeleteSession(sessionID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/session/"+sessionID, nil)
	if err != nil {
		return fmt.Errorf("delete session: creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete session: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) GetMessages(sessionID string) ([]Message, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/session/" + sessionID + "/message")
	if err != nil {
		return nil, fmt.Errorf("get messages request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get messages: status %d: %s", resp.StatusCode, string(respBody))
	}

	var messages []Message
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("get messages: decoding response: %w", err)
	}

	return messages, nil
}
