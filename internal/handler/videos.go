package handler

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/anthropics/paylock/internal/model"
)

const (
	defaultPerPage = 20
	maxPerPage     = 100
)

type Videos struct {
	videos *model.VideoStore
}

func NewVideos(videos *model.VideoStore) *Videos {
	return &Videos{videos: videos}
}

func (h *Videos) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	list := h.videos.List()

	if creator := r.URL.Query().Get("creator"); creator != "" {
		filtered := make([]model.Video, 0, len(list))
		for _, v := range list {
			if v.Creator == creator {
				filtered = append(filtered, v)
			}
		}
		list = filtered
	}

	// Sort by created_at descending (newest first)
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt > list[j].CreatedAt
	})

	total := len(list)
	page := parseIntQuery(r, "page", 1)
	perPage := parseIntQuery(r, "per_page", defaultPerPage)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	list = list[start:end]

	writeJSON(w, http.StatusOK, map[string]any{
		"videos":   list,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func parseIntQuery(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
