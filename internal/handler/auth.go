package handler

import "github.com/anthropics/paylock/internal/model"

const creatorHeader = "X-Creator"

// verifyCreator checks that the given address matches the video's creator.
// Returns true if authorized. If the video has no creator set, always returns true
// for backwards compatibility.
func verifyCreator(video *model.Video, addr string) bool {
	if video.Creator == "" {
		return true
	}
	return addr == video.Creator
}
