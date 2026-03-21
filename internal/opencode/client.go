package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ModelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type PermissionRule struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
}

type SendMessageRequest struct {
	Parts  []Part    `json:"parts"`
	Model  *ModelRef `json:"model,omitempty"`
	System string    `json:"system,omitempty"`
}

const systemPromptNoQuestions = "You are running in a fully automated pipeline with NO human operator. " +
	"NEVER ask questions, request clarification, or wait for input - nobody will answer and the pipeline will hang forever. " +
	"Make your best judgment and produce output immediately."

var allowAllPermissions = []PermissionRule{
	{Permission: "*", Pattern: "*", Action: "allow"},
}

var knownProviders = map[string]string{
	"claude":   "anthropic",
	"gpt":      "openai",
	"o1":       "openai",
	"o3":       "openai",
	"o4":       "openai",
	"gemini":   "google",
	"llama":    "groq",
	"mistral":  "mistral",
	"deepseek": "deepseek",
}

func ParseModelRef(llm string) ModelRef {
	if strings.Contains(llm, "/") {
		parts := strings.SplitN(llm, "/", 2)
		return ModelRef{ProviderID: parts[0], ModelID: parts[1]}
	}

	for prefix, provider := range knownProviders {
		if strings.HasPrefix(llm, prefix) {
			return ModelRef{ProviderID: provider, ModelID: llm}
		}
	}

	return ModelRef{ProviderID: "anthropic", ModelID: llm}
}

type Client struct {
	baseURL    string
	directory  string
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

func (c *Client) SetDirectory(dir string) {
	c.directory = dir
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
	body, err := json.Marshal(map[string]any{
		"title":      title,
		"permission": allowAllPermissions,
	})
	if err != nil {
		return nil, fmt.Errorf("create session: marshaling request: %w", err)
	}

	sessionURL := c.baseURL + "/session"
	if c.directory != "" {
		sessionURL += "?directory=" + url.QueryEscape(c.directory)
	}

	resp, err := c.httpClient.Post(sessionURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create session: status %d: %s", resp.StatusCode, formatAPIError(respBody))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("create session: decoding response: %w", err)
	}

	return &session, nil
}

func (c *Client) InitSession(sessionID string, model ModelRef) error {
	reqBody := map[string]string{
		"providerID": model.ProviderID,
		"modelID":    model.ModelID,
		"messageID":  "msg-init-" + sessionID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("init session: marshaling request: %w", err)
	}

	initURL := c.baseURL + "/session/" + sessionID + "/init"
	if c.directory != "" {
		initURL += "?directory=" + url.QueryEscape(c.directory)
	}

	resp, err := c.httpClient.Post(initURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("init session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("init session: status %d: %s", resp.StatusCode, formatAPIError(respBody))
	}

	return nil
}

func (c *Client) SendMessage(sessionID, prompt string, model ModelRef, output io.Writer) (*Message, error) {
	return c.SendMessageStream(context.Background(), sessionID, prompt, model, output)
}

func (c *Client) SendMessageAsync(sessionID, prompt string, model ModelRef) error {
	reqBody := SendMessageRequest{
		Parts:  []Part{{Type: "text", Text: prompt}},
		Model:  &model,
		System: systemPromptNoQuestions,
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message async: status %d: %s", resp.StatusCode, formatAPIError(respBody))
	}

	return nil
}

type sseEvent struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

type deltaProperties struct {
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	PartID    string `json:"partID"`
	Field     string `json:"field"`
	Delta     string `json:"delta"`
}

type partUpdatedProperties struct {
	Part struct {
		ID        string `json:"id"`
		SessionID string `json:"sessionID"`
		MessageID string `json:"messageID"`
		Type      string `json:"type"`
		Text      string `json:"text"`
	} `json:"part"`
}

type messageUpdatedProperties struct {
	Info struct {
		ID        string `json:"id"`
		SessionID string `json:"sessionID"`
		Role      string `json:"role"`
	} `json:"info"`
}

type sessionStatusProperties struct {
	SessionID string `json:"sessionID"`
	Status    struct {
		Type string `json:"type"`
	} `json:"status"`
}

type sessionIdleProperties struct {
	SessionID string `json:"sessionID"`
}

func (c *Client) SendMessageStream(ctx context.Context, sessionID, prompt string, model ModelRef, output io.Writer) (*Message, error) {
	sseReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/event", nil)
	if err != nil {
		return nil, fmt.Errorf("creating SSE request: %w", err)
	}
	sseReq.Header.Set("Accept", "text/event-stream")

	sseResp, err := c.httpClient.Do(sseReq)
	if err != nil {
		return nil, fmt.Errorf("connecting to SSE: %w", err)
	}

	connected := make(chan struct{})
	done := make(chan struct{})
	var fullText strings.Builder
	var streamErr error
	var assistantMsgID string
	seenParts := make(map[string]int)

	go func() {
		defer close(done)
		defer sseResp.Body.Close()

		scanner := bufio.NewScanner(sseResp.Body)
		scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

		connectedSent := false
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := line[6:]

			var evt sseEvent
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				continue
			}

			if evt.Type == "server.connected" && !connectedSent {
				connectedSent = true
				close(connected)
				continue
			}

			switch evt.Type {
			case "message.part.delta":
				var props deltaProperties
				if err := json.Unmarshal(evt.Properties, &props); err != nil {
					continue
				}
				if props.SessionID != sessionID || props.Field != "text" {
					continue
				}
				fullText.WriteString(props.Delta)
				seenParts[props.PartID] = fullText.Len()
				if output != nil {
					fmt.Fprint(output, props.Delta)
				}

			case "message.part.updated":
				var props partUpdatedProperties
				if err := json.Unmarshal(evt.Properties, &props); err != nil {
					continue
				}
				if props.Part.SessionID != sessionID || props.Part.Type != "text" {
					continue
				}
				prevLen, seen := seenParts[props.Part.ID]
				if !seen && props.Part.Text != "" {
					fullText.WriteString(props.Part.Text)
					seenParts[props.Part.ID] = fullText.Len()
					if output != nil {
						fmt.Fprint(output, props.Part.Text)
					}
				} else if seen && len(props.Part.Text) > prevLen {
					newText := props.Part.Text[prevLen:]
					fullText.WriteString(newText)
					seenParts[props.Part.ID] = fullText.Len()
					if output != nil {
						fmt.Fprint(output, newText)
					}
				}

			case "message.updated":
				var props messageUpdatedProperties
				if err := json.Unmarshal(evt.Properties, &props); err != nil {
					continue
				}
				if props.Info.SessionID == sessionID && props.Info.Role == "assistant" {
					assistantMsgID = props.Info.ID
				}

			case "session.idle":
				var props sessionIdleProperties
				if err := json.Unmarshal(evt.Properties, &props); err != nil {
					continue
				}
				if props.SessionID == sessionID {
					if output != nil {
						fmt.Fprintln(output)
					}
					return
				}

			case "session.status":
				var props sessionStatusProperties
				if err := json.Unmarshal(evt.Properties, &props); err != nil {
					continue
				}
				if props.SessionID == sessionID && props.Status.Type == "idle" {
					if output != nil {
						fmt.Fprintln(output)
					}
					return
				}
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			streamErr = fmt.Errorf("reading SSE stream: %w", err)
		}
	}()

	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		sseResp.Body.Close()
		return nil, fmt.Errorf("timeout waiting for SSE connection")
	case <-ctx.Done():
		sseResp.Body.Close()
		return nil, ctx.Err()
	}

	if err := c.SendMessageAsync(sessionID, prompt, model); err != nil {
		sseResp.Body.Close()
		return nil, err
	}

	select {
	case <-done:
	case <-ctx.Done():
		sseResp.Body.Close()
		<-done
		return nil, ctx.Err()
	}

	if streamErr != nil {
		return nil, streamErr
	}

	msg := &Message{
		Info: MessageInfo{
			ID:        assistantMsgID,
			SessionID: sessionID,
			Role:      "assistant",
		},
		Parts: []Part{
			{Type: "text", Text: fullText.String()},
		},
	}

	return msg, nil
}

func formatAPIError(body []byte) string {
	var apiErr struct {
		Error []struct {
			Message string `json:"message"`
			Path    []any  `json:"path"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && len(apiErr.Error) > 0 {
		var msgs []string
		for _, e := range apiErr.Error {
			msgs = append(msgs, e.Message)
		}
		return strings.Join(msgs, "; ")
	}
	return string(body)
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

type ProviderModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Name       string `json:"name"`
}

type Provider struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name"`
	Models map[string]ProviderModel `json:"models"`
}

type ProvidersResponse struct {
	All       []Provider `json:"all"`
	Connected []string   `json:"connected"`
}

func (c *Client) ListProviders() (*ProvidersResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/provider")
	if err != nil {
		return nil, fmt.Errorf("list providers request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list providers: status %d: %s", resp.StatusCode, formatAPIError(respBody))
	}

	var result ProvidersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("list providers: decoding response: %w", err)
	}

	return &result, nil
}

func (c *Client) ValidateModels(models []ModelRef) error {
	providers, err := c.ListProviders()
	if err != nil {
		return fmt.Errorf("fetching providers: %w", err)
	}

	connectedSet := make(map[string]bool, len(providers.Connected))
	for _, id := range providers.Connected {
		connectedSet[id] = true
	}

	available := make(map[string]map[string]bool)
	for _, p := range providers.All {
		if !connectedSet[p.ID] {
			continue
		}
		modelSet := make(map[string]bool, len(p.Models))
		for modelKey := range p.Models {
			modelSet[modelKey] = true
		}
		available[p.ID] = modelSet
	}

	var errs []string
	for _, m := range models {
		providerModels, ok := available[m.ProviderID]
		if !ok {
			known := make([]string, 0, len(available))
			for pid := range available {
				known = append(known, pid)
			}
			errs = append(errs, fmt.Sprintf(
				"provider %q not connected (connected: %v)", m.ProviderID, known))
			continue
		}
		if !providerModels[m.ModelID] {
			errs = append(errs, fmt.Sprintf(
				"model %q not found in provider %q", m.ModelID, m.ProviderID))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid models in config:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
