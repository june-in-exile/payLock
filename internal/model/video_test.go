package model

import (
	"testing"
)

func TestVideoStore_CreateAndGet(t *testing.T) {
	store := NewVideoStore()

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
	store := NewVideoStore()

	_, ok := store.Get("nonexistent")
	if ok {
		t.Fatal("expected video not to exist")
	}
}

func TestVideoStore_SetReady(t *testing.T) {
	store := NewVideoStore()
	store.Create("test-1", "My Video", 0, "")

	store.SetReady("test-1", "previewBlob", "https://agg/v1/blobs/previewBlob", "fullBlob", "https://agg/v1/blobs/fullBlob")

	v, _ := store.Get("test-1")
	if v.Status != StatusReady {
		t.Errorf("expected status ready, got %s", v.Status)
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
	store := NewVideoStore()
	store.Create("test-1", "My Video", 0, "")

	ok := store.SetSuiObjectID("test-1", "0xABC123")
	if !ok {
		t.Fatal("expected SetSuiObjectID to return true")
	}

	v, _ := store.Get("test-1")
	if v.SuiObjectID != "0xABC123" {
		t.Errorf("expected sui_object_id 0xABC123, got %s", v.SuiObjectID)
	}
}

func TestVideoStore_SetSuiObjectID_NotFound(t *testing.T) {
	store := NewVideoStore()

	ok := store.SetSuiObjectID("nonexistent", "0xABC")
	if ok {
		t.Fatal("expected SetSuiObjectID to return false for nonexistent")
	}
}

func TestVideoStore_SetFailed(t *testing.T) {
	store := NewVideoStore()
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
	store := NewVideoStore()
	store.Create("test-1", "My Video", 0, "")

	v1, _ := store.Get("test-1")
	v1.Status = StatusReady // mutate the copy

	v2, _ := store.Get("test-1")
	if v2.Status != StatusProcessing {
		t.Error("mutation of returned copy should not affect store")
	}
}

func TestVideoStore_List(t *testing.T) {
	store := NewVideoStore()
	store.Create("a", "Title A", 0, "")
	store.Create("b", "Title B", 0, "")

	list := store.List()
	if len(list) != 2 {
		t.Errorf("expected 2 videos, got %d", len(list))
	}
}

func TestVideoStore_Delete(t *testing.T) {
	store := NewVideoStore()
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
