package indexer

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

// ChainVideo represents a Video object read from the Sui chain.
type ChainVideo struct {
	ObjectID         string
	Title            string
	Price            uint64
	Creator          string
	ThumbnailBlobID  string
	PreviewBlobID    string
	FullBlobID       string
}

// Indexer scans the Sui chain for Video objects created by the gating package.
type Indexer struct {
	rpcURL    string
	packageID string
	client    *http.Client
}

func New(rpcURL, packageID string) *Indexer {
	return &Indexer{
		rpcURL:    rpcURL,
		packageID: packageID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchAll discovers all Video objects created via the gating::create_video function
// and returns their on-chain state. It paginates through transaction blocks, then
// fetches each created Video object.
func (idx *Indexer) FetchAll(ctx context.Context) ([]ChainVideo, error) {
	objectIDs, err := idx.discoverVideoObjectIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover video objects: %w", err)
	}

	if len(objectIDs) == 0 {
		slog.Info("indexer: no video objects found on chain")
		return nil, nil
	}

	slog.Info("indexer: discovered video objects", "count", len(objectIDs))

	videos, err := idx.fetchVideoObjects(ctx, objectIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch video objects: %w", err)
	}

	slog.Info("indexer: fetched video data from chain", "count", len(videos))
	return videos, nil
}

// discoverVideoObjectIDs finds all object IDs created by create_video transactions.
func (idx *Indexer) discoverVideoObjectIDs(ctx context.Context) ([]string, error) {
	var allIDs []string
	var cursor *string

	for {
		txDigests, nextCursor, err := idx.queryCreateVideoTxs(ctx, cursor)
		if err != nil {
			return nil, err
		}

		for _, digest := range txDigests {
			ids, err := idx.extractCreatedObjectIDs(ctx, digest)
			if err != nil {
				slog.Warn("indexer: failed to extract objects from tx", "digest", digest, "error", err)
				continue
			}
			allIDs = append(allIDs, ids...)
		}

		if nextCursor == nil || len(txDigests) == 0 {
			break
		}
		cursor = nextCursor
	}

	return allIDs, nil
}

// queryCreateVideoTxs queries transaction blocks that called create_video.
func (idx *Indexer) queryCreateVideoTxs(ctx context.Context, cursor *string) (digests []string, nextCursor *string, err error) {
	filter := map[string]any{
		"MoveFunction": map[string]any{
			"package":  idx.packageID,
			"module":   "gating",
			"function": "create_video",
		},
	}

	params := []any{filter, cursor, 50, true} // limit 50, descending

	resp, err := idx.rpcCall(ctx, "suix_queryTransactionBlocks", params)
	if err != nil {
		return nil, nil, err
	}

	var result struct {
		Data []struct {
			Digest string `json:"digest"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
		HasNextPage bool   `json:"hasNextPage"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, nil, fmt.Errorf("parse queryTransactionBlocks: %w", err)
	}

	for _, d := range result.Data {
		digests = append(digests, d.Digest)
	}

	if result.HasNextPage {
		return digests, result.NextCursor, nil
	}
	return digests, nil, nil
}

// extractCreatedObjectIDs gets the created object IDs from a transaction.
func (idx *Indexer) extractCreatedObjectIDs(ctx context.Context, digest string) ([]string, error) {
	params := []any{
		digest,
		map[string]bool{
			"showEffects": true,
		},
	}

	resp, err := idx.rpcCall(ctx, "sui_getTransactionBlock", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Effects struct {
			Created []struct {
				Reference struct {
					ObjectID string `json:"objectId"`
				} `json:"reference"`
			} `json:"created"`
		} `json:"effects"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse tx effects: %w", err)
	}

	var ids []string
	for _, c := range result.Effects.Created {
		ids = append(ids, c.Reference.ObjectID)
	}
	return ids, nil
}

// fetchVideoObjects fetches object data in batches using sui_multiGetObjects.
func (idx *Indexer) fetchVideoObjects(ctx context.Context, objectIDs []string) ([]ChainVideo, error) {
	var videos []ChainVideo
	batchSize := 50

	for i := 0; i < len(objectIDs); i += batchSize {
		end := i + batchSize
		if end > len(objectIDs) {
			end = len(objectIDs)
		}
		batch := objectIDs[i:end]

		params := []any{
			batch,
			map[string]bool{
				"showContent": true,
				"showType":    true,
			},
		}

		resp, err := idx.rpcCall(ctx, "sui_multiGetObjects", params)
		if err != nil {
			return nil, err
		}

		var objects []struct {
			Data *struct {
				ObjectID string `json:"objectId"`
				Type     string `json:"type"`
				Content  *struct {
					DataType string         `json:"dataType"`
					Type     string         `json:"type"`
					Fields   map[string]any `json:"fields"`
				} `json:"content"`
			} `json:"data"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(resp, &objects); err != nil {
			return nil, fmt.Errorf("parse multiGetObjects: %w", err)
		}

		videoType := fmt.Sprintf("%s::gating::Video", idx.packageID)
		for _, obj := range objects {
			if obj.Data == nil || obj.Data.Content == nil {
				continue
			}
			if obj.Data.Content.Type != videoType {
				continue
			}

			cv := ChainVideo{
				ObjectID: obj.Data.ObjectID,
			}

			fields := obj.Data.Content.Fields
			if v, ok := fields["price"].(string); ok {
				fmt.Sscanf(v, "%d", &cv.Price)
			} else if v, ok := fields["price"].(float64); ok {
				cv.Price = uint64(v)
			}
			if v, ok := fields["title"].(string); ok {
				cv.Title = v
			}
			if v, ok := fields["creator"].(string); ok {
				cv.Creator = v
			}
			if v, ok := fields["thumbnail_blob_id"].(string); ok {
				cv.ThumbnailBlobID = v
			}
			if v, ok := fields["preview_blob_id"].(string); ok {
				cv.PreviewBlobID = v
			}
			if v, ok := fields["full_blob_id"].(string); ok {
				cv.FullBlobID = v
			}

			videos = append(videos, cv)
		}
	}

	return videos, nil
}

// rpcCall makes a JSON-RPC 2.0 call to the Sui node.
func (idx *Indexer) rpcCall(ctx context.Context, method string, params []any) (json.RawMessage, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, idx.rpcURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create rpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := idx.client.Do(req)
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
