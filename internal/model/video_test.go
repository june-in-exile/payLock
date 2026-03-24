package model

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *VideoStore {
	t.Helper()
	store, err := NewVideoStore("")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestVideoStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)

	store.Create("test-1", "My Video", 100_000_000, "0xCAFE")

	v, ok := store.Get("test-1")
	if !ok {
		t.Fatal("expected video to exist")
	}
	if v.ID != "test-1" {
		t.Errorf("expected id test-1, got %s", v.ID)
	}
	if v.Title != "My Video" {
		t.Errorf("expected title My Video, got %s", v.Title)
	}
	if v.Status != StatusProcessing {
		t.Errorf("expected status processing, got %s", v.Status)
	}
	if v.Price != 100_000_000 {
		t.Errorf("expected price 100000000, got %d", v.Price)
	}
	if v.Creator != "0xCAFE" {
		t.Errorf("expected creator 0xCAFE, got %s", v.Creator)
	}
}

func TestVideoStore_GetNotFound(t *testing.T) {
	store := newTestStore(t)

	_, ok := store.Get("nonexistent")
	if ok {
		t.Fatal("expected video not to exist")
	}
}

func TestVideoStore_SetReady(t *testing.T) {
	store := newTestStore(t)
	store.Create("test-1", "My Video", 0, "")

	store.SetReady("test-1", "thumbBlob", "https://agg/v1/blobs/thumbBlob", "previewBlob", "https://agg/v1/blobs/previewBlob", "fullBlob", "https://agg/v1/blobs/fullBlob")

	v, _ := store.Get("test-1")
	if v.Status != StatusReady {
		t.Errorf("expected status ready, got %s", v.Status)
	}
	if v.ThumbnailBlobID != "thumbBlob" {
		t.Errorf("expected thumbnail_blob_id thumbBlob, got %s", v.ThumbnailBlobID)
	}
	if v.ThumbnailBlobURL != "https://agg/v1/blobs/thumbBlob" {
		t.Errorf("expected thumbnail_blob_url, got %s", v.ThumbnailBlobURL)
	}
	if v.PreviewBlobID != "previewBlob" {
		t.Errorf("expected preview_blob_id previewBlob, got %s", v.PreviewBlobID)
	}
	if v.PreviewBlobURL != "https://agg/v1/blobs/previewBlob" {
		t.Errorf("expected preview_blob_url, got %s", v.PreviewBlobURL)
	}
	if v.FullBlobID != "fullBlob" {
		t.Errorf("expected full_blob_id fullBlob, got %s", v.FullBlobID)
	}
	if v.FullBlobURL != "https://agg/v1/blobs/fullBlob" {
		t.Errorf("expected full_blob_url, got %s", v.FullBlobURL)
	}
}

func TestVideoStore_SetSuiObjectID(t *testing.T) {
	store := newTestStore(t)
	store.Create("test-1", "My Video", 0, "")

	ok := store.SetSuiObjectID("test-1", "0xABC123", "", "")
	if !ok {
		t.Fatal("expected SetSuiObjectID to return true")
	}

	v, _ := store.Get("test-1")
	if v.SuiObjectID != "0xABC123" {
		t.Errorf("expected sui_object_id 0xABC123, got %s", v.SuiObjectID)
	}
}

func TestVideoStore_SetSuiObjectID_NotFound(t *testing.T) {
	store := newTestStore(t)

	ok := store.SetSuiObjectID("nonexistent", "0xABC", "", "")
	if ok {
		t.Fatal("expected SetSuiObjectID to return false for nonexistent")
	}
}

func TestVideoStore_SetFailed(t *testing.T) {
	store := newTestStore(t)
	store.Create("test-1", "My Video", 0, "")

	store.SetFailed("test-1", "something went wrong")

	v, _ := store.Get("test-1")
	if v.Status != StatusFailed {
		t.Errorf("expected status failed, got %s", v.Status)
	}
	if v.Error != "something went wrong" {
		t.Errorf("expected error message, got %s", v.Error)
	}
}

func TestVideoStore_GetReturnsImmutableCopy(t *testing.T) {
	store := newTestStore(t)
	store.Create("test-1", "My Video", 0, "")

	v1, _ := store.Get("test-1")
	v1.Status = StatusReady // mutate the copy

	v2, _ := store.Get("test-1")
	if v2.Status != StatusProcessing {
		t.Error("mutation of returned copy should not affect store")
	}
}

func TestVideoStore_List(t *testing.T) {
	store := newTestStore(t)
	store.Create("a", "Title A", 0, "")
	store.Create("b", "Title B", 0, "")

	list := store.List()
	if len(list) != 2 {
		t.Errorf("expected 2 videos, got %d", len(list))
	}
}

func TestVideoStore_Delete(t *testing.T) {
	store := newTestStore(t)
	store.Create("test-1", "My Video", 0, "")

	if !store.Delete("test-1") {
		t.Fatal("expected delete to return true")
	}

	_, ok := store.Get("test-1")
	if ok {
		t.Fatal("expected video to be deleted")
	}

	if store.Delete("test-1") {
		t.Fatal("expected delete of nonexistent to return false")
	}
}

func TestVideoStore_Persistence(t *testing.T) {
	dir := t.TempDir()

	store1, err := NewVideoStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store1.Create("v1", "Video One", 0, "")
	store1.SetReady("v1", "tBlob", "tURL", "pBlob", "pURL", "fBlob", "fURL")
	store1.Create("v2", "Video Two", 500, "0xABC")

	// Load a new store from the same directory — should recover data
	store2, err := NewVideoStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	list := store2.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 videos after reload, got %d", len(list))
	}

	v1, ok := store2.Get("v1")
	if !ok {
		t.Fatal("expected v1 to exist after reload")
	}
	if v1.Status != StatusReady {
		t.Errorf("expected status ready, got %s", v1.Status)
	}
	if v1.PreviewBlobID != "pBlob" {
		t.Errorf("expected preview_blob_id pBlob, got %s", v1.PreviewBlobID)
	}
	if v1.FullBlobID != "fBlob" {
		t.Errorf("expected full_blob_id fBlob, got %s", v1.FullBlobID)
	}

	v2, ok := store2.Get("v2")
	if !ok {
		t.Fatal("expected v2 to exist after reload")
	}
	if v2.Price != 500 {
		t.Errorf("expected price 500, got %d", v2.Price)
	}
	if v2.Creator != "0xABC" {
		t.Errorf("expected creator 0xABC, got %s", v2.Creator)
	}
}

func TestVideoStore_DeletePersists(t *testing.T) {
	dir := t.TempDir()

	store1, err := NewVideoStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store1.Create("v1", "Video One", 0, "")
	store1.Create("v2", "Video Two", 0, "")
	store1.Delete("v1")

	store2, err := NewVideoStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := store2.Get("v1"); ok {
		t.Fatal("expected v1 to be deleted after reload")
	}
	if _, ok := store2.Get("v2"); !ok {
		t.Fatal("expected v2 to exist after reload")
	}
}

func TestVideoStore_PersistenceFileLocation(t *testing.T) {
	dir := t.TempDir()

	store, err := NewVideoStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store.Create("v1", "Test", 0, "")

	path := filepath.Join(dir, "videos.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected %s to exist", path)
	}
}
