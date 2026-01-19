package pi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type ImageRequest struct {
	URL      string `json:"url"`
	MimeType string `json:"mimeType"`
}

type ImageResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) SendImage(url, mimeType string) (*ImageResponse, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("pi URL not configured")
	}

	payload := ImageRequest{
		URL:      url,
		MimeType: mimeType,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(c.baseURL, "/") + "/api/get-img"
	log.Printf("Sending image to Pi: %s", endpoint)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			log.Printf("Pi request timed out (request likely still processed)")
			return &ImageResponse{Status: "pending", Message: "Request sent, awaiting display"}, nil
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result ImageResponse
	if respErr := json.NewDecoder(resp.Body).Decode(&result); respErr != nil {
		return nil, fmt.Errorf("failed to decode response: %w", respErr)
	}

	if resp.StatusCode >= 400 {
		return &result, fmt.Errorf("pi error: %s", result.Error)
	}

	return &result, nil
}

func (c *Client) ClearImages() error {
	if c.baseURL == "" {
		return fmt.Errorf("pi URL not configured")
	}

	clearEndpoint := strings.TrimSuffix(c.baseURL, "/") + "/api/clear-dir"

	req, err := http.NewRequest(http.MethodDelete, clearEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("pi returned status %d", resp.StatusCode)
	}

	return nil
}
