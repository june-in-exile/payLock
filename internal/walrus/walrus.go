package walrus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PublisherResponse struct {
	NewlyCreated     *NewlyCreatedBlob     `json:"newlyCreated,omitempty"`
	AlreadyCertified *AlreadyCertifiedBlob `json:"alreadyCertified,omitempty"`
}

type NewlyCreatedBlob struct {
	BlobObject struct {
		BlobID string `json:"blobId"`
	} `json:"blobObject"`
}

type AlreadyCertifiedBlob struct {
	BlobID string `json:"blobId"`
}

type Client struct {
	publisherURL  string
	aggregatorURL string
	httpClient    *http.Client
}

func NewClient(publisherURL, aggregatorURL string) *Client {
	return &Client{
		publisherURL:  publisherURL,
		aggregatorURL: aggregatorURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *Client) Store(data []byte) (blobID string, err error) {
	req, err := http.NewRequest(http.MethodPut, c.publisherURL+"/v1/blobs", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("store blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("store blob: status %d: %s", resp.StatusCode, string(body))
	}

	var result PublisherResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if result.NewlyCreated != nil {
		return result.NewlyCreated.BlobObject.BlobID, nil
	}
	if result.AlreadyCertified != nil {
		return result.AlreadyCertified.BlobID, nil
	}
	return "", fmt.Errorf("store blob: no blob ID in response")
}

func (c *Client) Fetch(blobID string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.aggregatorURL+"/v1/blobs/"+blobID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch blob: status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
