package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// VideoLinker is called when chain events are detected.
type VideoLinker interface {
	MatchAndLinkChainVideo(suiObjectID, title string, price uint64, creator, previewBlobID, fullBlobID string, blobURLFn func(string) string)
	DeleteBySuiObjectID(suiObjectID string) bool
}

// Watcher polls the Sui chain for VideoCreated events and links them to local
// video entries via the VideoLinker interface.
type Watcher struct {
	rpcURL    string
	packageID string
	linker    VideoLinker
	blobURLFn func(string) string
	interval  time.Duration
	client    *http.Client
}

func New(rpcURL, packageID string, linker VideoLinker, blobURLFn func(string) string, interval time.Duration) *Watcher {
	return &Watcher{
		rpcURL:    rpcURL,
		packageID: packageID,
		linker:    linker,
		blobURLFn: blobURLFn,
		interval:  interval,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	slog.Info("watcher: starting event poller", "interval", w.interval, "package", w.packageID)

	var cursor *eventCursor
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("watcher: shutting down")
			return
		case <-ticker.C:
			nextCursor, err := w.poll(ctx, cursor)
			if err != nil {
				slog.Error("watcher: poll failed", "error", err)
				continue
			}
			if nextCursor != nil {
				cursor = nextCursor
			}
		}
	}
}

type eventCursor struct {
	TxDigest string `json:"txDigest"`
	EventSeq string `json:"eventSeq"`
}

func (w *Watcher) poll(ctx context.Context, cursor *eventCursor) (*eventCursor, error) {
	// Query all events from the gating module (VideoCreated + VideoDeleted).
	filter := map[string]any{
		"MoveEventModule": map[string]any{
			"package": w.packageID,
			"module":  "gating",
		},
	}

	params := []any{filter, cursor, 50, false} // limit 50, ascending

	resp, err := w.rpcCall(ctx, "suix_queryEvents", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []struct {
			ID         eventCursor    `json:"id"`
			Type       string         `json:"type"`
			ParsedJSON map[string]any `json:"parsedJson"`
		} `json:"data"`
		NextCursor  *eventCursor `json:"nextCursor"`
		HasNextPage bool         `json:"hasNextPage"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse queryEvents: %w", err)
	}

	deletedType := fmt.Sprintf("%s::gating::VideoDeleted", w.packageID)
	for _, evt := range result.Data {
		if evt.Type == deletedType {
			w.handleDeleteEvent(evt.ParsedJSON)
		} else {
			w.handleEvent(evt.ParsedJSON)
		}
	}

	if result.HasNextPage && result.NextCursor != nil {
		// More pages — recurse to drain them within the same poll cycle.
		return w.poll(ctx, result.NextCursor)
	}

	return result.NextCursor, nil
}

func (w *Watcher) handleEvent(fields map[string]any) {
	suiObjectID, _ := fields["video_id"].(string)
	if suiObjectID == "" {
		slog.Warn("watcher: event missing video_id", "fields", fields)
		return
	}

	var price uint64
	if v, ok := fields["price"].(string); ok {
		fmt.Sscanf(v, "%d", &price)
	} else if v, ok := fields["price"].(float64); ok {
		price = uint64(v)
	}

	title, _ := fields["title"].(string)
	creator, _ := fields["creator"].(string)
	previewBlobID, _ := fields["preview_blob_id"].(string)
	fullBlobID, _ := fields["full_blob_id"].(string)

	slog.Info("watcher: detected VideoCreated event",
		"sui_object_id", suiObjectID,
		"title", title,
		"preview_blob_id", previewBlobID,
		"price", price,
	)

	w.linker.MatchAndLinkChainVideo(suiObjectID, title, price, creator, previewBlobID, fullBlobID, w.blobURLFn)
}

func (w *Watcher) handleDeleteEvent(fields map[string]any) {
	suiObjectID, _ := fields["video_id"].(string)
	if suiObjectID == "" {
		slog.Warn("watcher: VideoDeleted event missing video_id", "fields", fields)
		return
	}

	slog.Info("watcher: detected VideoDeleted event", "sui_object_id", suiObjectID)
	w.linker.DeleteBySuiObjectID(suiObjectID)
}

// rpcCall makes a JSON-RPC 2.0 call to the Sui node.
func (w *Watcher) rpcCall(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.rpcURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create rpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc call %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read rpc response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc call %s: status %d: %s", method, resp.StatusCode, string(respBody))
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
