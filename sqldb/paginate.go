package sqldb

import (
	"context"
	"fmt"
)

// BatchQueryFunc represents a function that takes a batch of converted items
// and returns results.
type BatchQueryFunc[T any, R any] func(context.Context, []T) ([]R, error)

// ItemCallbackFunc represents a function that processes individual results.
type ItemCallbackFunc[R any] func(context.Context, R) error

// ConvertFunc represents a function that converts from input type to query type
// for the batch query.
type ConvertFunc[I any, T any] func(I) T

// BatchQueryConfig holds configuration values for calls to ExecuteBatchQuery.
type BatchQueryConfig struct {
	// MaxBatchSize is the maximum number of items included in a batch
	// query IN clauses list.
	MaxBatchSize int
}

// DefaultBatchQueryConfig returns a default configuration for batched queries.
func DefaultBatchQueryConfig() *BatchQueryConfig {
	return &BatchQueryConfig{
		MaxBatchSize: 1000,
	}
}

// ExecuteBatchQuery executes a query in batches over a slice of input items.
// It converts the input items to a query type using the provided convertFunc,
// executes the query in batches using the provided queryFunc, and applies
// the callback to each result. This is useful for queries using the
// "WHERE x IN []slice" pattern. It takes that slice, splits it into batches of
// size MaxBatchSize, and executes the query for each batch.
//
// NOTE: it is the caller's responsibility to ensure that the expected return
// results are unique across all pages. Meaning that if the input items are
// split up, a result that is returned in one page should not be expected to
// be returned in another page.
func ExecuteBatchQuery[I any, T any, R any](ctx context.Context,
	cfg *BatchQueryConfig, inputItems []I, convertFunc ConvertFunc[I, T],
	queryFunc BatchQueryFunc[T, R], callback ItemCallbackFunc[R]) error {

	if len(inputItems) == 0 {
		return nil
	}

	// Process items in pages.
	for i := 0; i < len(inputItems); i += cfg.MaxBatchSize {
		// Calculate the end index for this page.
		end := i + cfg.MaxBatchSize
		if end > len(inputItems) {
			end = len(inputItems)
		}

		// Get the page slice of input items.
		inputPage := inputItems[i:end]

		// Convert only the items needed for this page.
		convertedPage := make([]T, len(inputPage))
		for j, inputItem := range inputPage {
			convertedPage[j] = convertFunc(inputItem)
		}

		// Execute the query for this page.
		results, err := queryFunc(ctx, convertedPage)
		if err != nil {
			return fmt.Errorf("query failed for page "+
				"starting at %d: %w", i, err)
		}

		// Apply the callback to each result.
		for _, result := range results {
			if err := callback(ctx, result); err != nil {
				return fmt.Errorf("callback failed for "+
					"result: %w", err)
			}
		}
	}

	return nil
}

// PagedQueryFunc represents a function that fetches a page of results using a cursor.
// It returns the fetched items and should return an empty slice when no more results.
type PagedQueryFunc[C any, T any] func(context.Context, C, int32) ([]T, error)

// CursorExtractFunc represents a function that extracts the cursor value from an item.
// This cursor will be used for the next page fetch.
type CursorExtractFunc[T any, C any] func(T) C

// ItemProcessFunc represents a function that processes individual items.
type ItemProcessFunc[T any] func(context.Context, T) error

// PaginatedQueryConfig holds configuration values for paginated queries.
type PaginatedQueryConfig struct {
	PageSize int32
}

// DefaultPaginatedQueryConfig returns a default configuration
func DefaultPaginatedQueryConfig() *PaginatedQueryConfig {
	return &PaginatedQueryConfig{
		PageSize: 1000, // Different default than batch queries
	}
}

// ExecutePaginatedQuery executes a cursor-based paginated query.
// It continues fetching pages until no more results are returned,
// processing each item with the provided callback.
//
// Parameters:
// - ctx: context for the operation
// - cfg: configuration including page size
// - initialCursor: the starting cursor value (e.g., 0, -1, "", etc.)
// - queryFunc: function that fetches a page given cursor and limit
// - extractCursor: function that extracts cursor from an item for next page
// - processItem: function that processes each individual item
func ExecutePaginatedQuery[C any, T any](
	ctx context.Context,
	cfg *PaginatedQueryConfig,
	initialCursor C,
	queryFunc PagedQueryFunc[C, T],
	extractCursor CursorExtractFunc[T, C],
	processItem ItemProcessFunc[T],
) error {
	cursor := initialCursor

	for {
		// Fetch the next page.
		items, err := queryFunc(ctx, cursor, cfg.PageSize)
		if err != nil {
			return fmt.Errorf("failed to fetch page with "+
				"cursor %v: %w", cursor, err)
		}

		// If no items returned, we're done.
		if len(items) == 0 {
			break
		}

		// Process each item in the page.
		for _, item := range items {
			if err := processItem(ctx, item); err != nil {
				return fmt.Errorf("failed to process item: %w",
					err)
			}

			// Update cursor for next iteration.
			cursor = extractCursor(item)
		}
	}

	return nil
}
