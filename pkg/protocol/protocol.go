package protocol

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/allani-ca/jetweir/pkg/types"
)

// ParseSubscriptionParams parses official Jetstream v2 subscription parameters from query string.
func ParseSubscriptionParams(query url.Values) types.SubscriptionFilters {
	filters := types.SubscriptionFilters{
		Cursor:              -1,
		MaxMessageSizeBytes: 100 * 1024,
		Compress:            false,
	}

	if collections := query.Get("wantedCollections"); collections != "" {
		filters.WantedCollections = strings.Split(collections, ",")
	}

	if dids := query.Get("wantedDids"); dids != "" {
		filters.WantedDIDs = strings.Split(dids, ",")
	}

	if cursor := query.Get("cursor"); cursor != "" {
		if c, err := strconv.ParseInt(cursor, 10, 64); err == nil {
			filters.Cursor = c
		}
	}

	if maxSize := query.Get("maxMessageSizeBytes"); maxSize != "" {
		if size, err := strconv.Atoi(maxSize); err == nil {
			filters.MaxMessageSizeBytes = size
		}
	}

	if compress := query.Get("compress"); compress != "" {
		filters.Compress = compress == "true" || compress == "1"
	}

	return filters
}

// FilterMatches determines if an event matches the given filters.
func FilterMatches(event *types.JetstreamEvent, filters types.SubscriptionFilters) bool {
	// If no filters specified, match all events
	if len(filters.WantedCollections) == 0 && len(filters.WantedDIDs) == 0 {
		return true
	}

	// Check DID filter
	if len(filters.WantedDIDs) > 0 {
		if !contains(filters.WantedDIDs, event.Did) {
			return false
		}
	}

	// Check collection filter (from operation field)
	if len(filters.WantedCollections) > 0 {
		collection := extractCollection(event.Operation)
		if collection != "" && !contains(filters.WantedCollections, collection) {
			return false
		}
	}

	return true
}

// FilterHash creates a canonical hash for a subscription filter to enable deduplication.
func FilterHash(filters types.SubscriptionFilters) string {
	// Sort and deduplicate collections
	collections := dedupeAndSort(filters.WantedCollections)
	dids := dedupeAndSort(filters.WantedDIDs)

	hash := struct {
		Collections []string
		DIDs        []string
		Compress    bool
	}{
		Collections: collections,
		DIDs:        dids,
		Compress:    filters.Compress,
	}

	b, _ := json.Marshal(hash)
	return string(b)
}

// IsSubsetFilter checks if filterA is a subset of filterB (filterB is broader).
func IsSubsetFilter(filterA, filterB types.SubscriptionFilters) bool {
	// If B has no collection filter, it matches all collections
	if len(filterB.WantedCollections) == 0 || containsAll(filterB.WantedCollections, filterA.WantedCollections) {
		// Same logic for DIDs
		if len(filterB.WantedDIDs) == 0 || containsAll(filterB.WantedDIDs, filterA.WantedDIDs) {
			return true
		}
	}
	return false
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func containsAll(container, items []string) bool {
	for _, item := range items {
		if !contains(container, item) {
			return false
		}
	}
	return true
}

func dedupeAndSort(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	// Simple sort for consistency
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i] > result[j] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func extractCollection(operation string) string {
	// Operation format: "app.bsky.feed.post#create" or similar
	// Extract the collection part (before #)
	if idx := strings.Index(operation, "#"); idx != -1 {
		return operation[:idx]
	}
	return operation
}

// MarshalEvent converts an event to JSON bytes.
func MarshalEvent(event *types.JetstreamEvent) ([]byte, error) {
	return json.Marshal(event)
}

// UnmarshalEvent converts JSON bytes to an event.
func UnmarshalEvent(data []byte) (*types.JetstreamEvent, error) {
	var event types.JetstreamEvent
	err := json.Unmarshal(data, &event)
	return &event, err
}
