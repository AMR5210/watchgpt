package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/metrics"
	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
	"github.com/sony/gobreaker/v2"
)

const (
	openaiURL = "https://api.openai.com/v1/chat/completions"
	model     = "gpt-4o-mini"
)

type OpenAIClient struct {
	apiKey     string
	httpClient *http.Client
	cb         *gobreaker.CircuitBreaker[*http.Response]
}

func NewOpenAIClient(apiKey string) *OpenAIClient {
	cb := gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        "openai",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker state change",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
			stateMap := map[gobreaker.State]float64{
				gobreaker.StateClosed:   0,
				gobreaker.StateHalfOpen: 1,
				gobreaker.StateOpen:     2,
			}
			metrics.CircuitBreakerState.Set(stateMap[to])
		},
	})

	return &OpenAIClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		cb:         cb,
	}
}

type chatRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SSE chunk for streaming
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type ChatMessage struct {
	Role    string
	Content string
	Image   string
}

func (c *OpenAIClient) Analyze(ctx context.Context, base64Image, prompt string) (string, error) {
	metrics.ImageSizeBytes.Observe(float64(len(base64Image)))

	dataURL := "data:image/jpeg;base64," + base64Image
	content := []contentPart{
		{Type: "text", Text: prompt},
		{Type: "image_url", ImageURL: &imageURL{URL: dataURL, Detail: "low"}},
	}

	reqBody := chatRequest{
		Model:     model,
		MaxTokens: 300,
		Messages:  []chatMessage{{Role: "user", Content: content}},
	}

	return c.doRequest(ctx, reqBody)
}

func (c *OpenAIClient) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	metrics.ActiveConversations.Inc()
	defer metrics.ActiveConversations.Dec()

	openaiMessages := c.buildMessages(messages)
	reqBody := chatRequest{
		Model:     model,
		MaxTokens: 500,
		Messages:  openaiMessages,
	}

	return c.doRequest(ctx, reqBody)
}

// ChatStream streams tokens to the provided flush function
func (c *OpenAIClient) ChatStream(ctx context.Context, messages []ChatMessage, onToken func(string)) error {
	metrics.ActiveConversations.Inc()
	defer metrics.ActiveConversations.Dec()

	openaiMessages := c.buildMessages(messages)
	reqBody := chatRequest{
		Model:     model,
		MaxTokens: 500,
		Messages:  openaiMessages,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	setRequestIDHeader(ctx, req)

	start := time.Now()
	resp, err := c.doHTTP(req)
	if err != nil {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBytes[:min(len(respBytes), 500)]))
	}

	// Read SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onToken(chunk.Choices[0].Delta.Content)
		}
	}

	duration := time.Since(start).Seconds()
	metrics.OpenAIRequestDuration.Observe(duration)
	metrics.OpenAIRequestsTotal.WithLabelValues("success").Inc()
	return nil
}

func (c *OpenAIClient) buildMessages(messages []ChatMessage) []chatMessage {
	var openaiMessages []chatMessage
	for _, msg := range messages {
		if msg.Image != "" && msg.Role == "user" {
			metrics.ImageSizeBytes.Observe(float64(len(msg.Image)))
			dataURL := "data:image/jpeg;base64," + msg.Image
			content := []contentPart{
				{Type: "text", Text: msg.Content},
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL, Detail: "low"}},
			}
			openaiMessages = append(openaiMessages, chatMessage{Role: msg.Role, Content: content})
		} else {
			openaiMessages = append(openaiMessages, chatMessage{Role: msg.Role, Content: msg.Content})
		}
	}
	return openaiMessages
}

// doHTTP wraps HTTP calls with the circuit breaker.
// Server errors (5xx) trip the breaker; client errors (4xx) pass through.
func (c *OpenAIClient) doHTTP(req *http.Request) (*http.Response, error) {
	return c.cb.Execute(func() (*http.Response, error) {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("openai server error %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
		}
		return resp, nil
	})
}

// IsCircuitOpen returns true if the error is due to the circuit breaker being open.
func IsCircuitOpen(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

func (c *OpenAIClient) doRequest(ctx context.Context, reqBody chatRequest) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	setRequestIDHeader(ctx, req)

	start := time.Now()
	resp, err := c.doHTTP(req)
	duration := time.Since(start).Seconds()
	metrics.OpenAIRequestDuration.Observe(duration)

	if err != nil {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBytes[:min(len(respBytes), 500)]))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("openai error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		metrics.OpenAIRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("no choices in response")
	}

	metrics.OpenAIRequestsTotal.WithLabelValues("success").Inc()
	return chatResp.Choices[0].Message.Content, nil
}

func setRequestIDHeader(ctx context.Context, req *http.Request) {
	if requestID := requestctx.RequestID(ctx); requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
}
