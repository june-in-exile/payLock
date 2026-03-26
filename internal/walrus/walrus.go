package walrus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"syscall"
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
			Timeout: 10 * time.Minute,
		},
	}
}

const maxRetries = 3

// Store uploads data to Walrus and returns the blob ID.
// Retries up to maxRetries times with exponential backoff on transient errors.
func (c *Client) Store(ctx context.Context, data []byte, epochs int) (string, error) {
	var lastErr error
	for attempt := range maxRetries {
		blobID, err := c.storeOnce(ctx, data, epochs)
		if err == nil {
			return blobID, nil
		}
		lastErr = err
		if !isTransient(err) {
			return "", err
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("store blob: %w", ctx.Err())
		}
		backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("store blob: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return "", fmt.Errorf("store blob after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) storeOnce(ctx context.Context, data []byte, epochs int) (string, error) {
	url := fmt.Sprintf("%s/v1/blobs?epochs=%d", c.publisherURL, epochs)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, io.NopCloser(
		io.NewSectionReader(readerAt(data), 0, int64(len(data))),
	))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("store blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return "", &serverError{status: resp.StatusCode, body: string(body)}
	}
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

type serverError struct {
	status int
	body   string
}

func (e *serverError) Error() string {
	return fmt.Sprintf("store blob: status %d: %s", e.status, e.body)
}

// isTransient returns true for network errors and 5xx server errors that are worth retrying.
func isTransient(err error) bool {
	var se *serverError
	if errors.As(err, &se) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return false
}

// BlobURL returns the aggregator URL for a given blob ID.
func (c *Client) BlobURL(blobID string) string {
	return c.aggregatorURL + "/v1/blobs/" + blobID
}

// readerAt wraps a byte slice to implement io.ReaderAt.
type readerAt []byte

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r)) {
		return 0, io.EOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
