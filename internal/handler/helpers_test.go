package handler

import (
	"testing"

	"github.com/anthropics/orca/internal/model"
)

func mustNewVideoStore(t *testing.T) *model.VideoStore {
	t.Helper()
	store, err := model.NewVideoStore("")
	if err != nil {
		t.Fatal(err)
	}
	return store
}
