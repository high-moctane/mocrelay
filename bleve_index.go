//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	"github.com/blevesearch/bleve/v2/mapping"
)

// SearchIndex defines the interface for full-text search indexing.
type SearchIndex interface {
	// Index adds or updates an event in the search index.
	Index(ctx context.Context, event *Event) error

	// Search returns event IDs matching the query, ordered by relevance.
	// Results are limited to the specified count.
	Search(ctx context.Context, query string, limit int64) (eventIDs []string, err error)

	// Delete removes an event from the search index.
	Delete(ctx context.Context, eventID string) error

	// Close releases resources.
	Close() error
}

// BleveIndex implements SearchIndex using Bleve with CJK analyzer.
type BleveIndex struct {
	index bleve.Index
}

// BleveIndexOptions configures BleveIndex behavior.
type BleveIndexOptions struct {
	// Path to store the index. If empty, uses in-memory index.
	Path string
}

// NewBleveIndex creates a new Bleve-based search index.
func NewBleveIndex(opts *BleveIndexOptions) (*BleveIndex, error) {
	if opts == nil {
		opts = &BleveIndexOptions{}
	}

	indexMapping := buildIndexMapping()

	var index bleve.Index
	var err error

	if opts.Path == "" {
		// In-memory index (for testing)
		index, err = bleve.NewMemOnly(indexMapping)
	} else {
		// Try to open existing index, create if not exists
		index, err = bleve.Open(opts.Path)
		if err == bleve.ErrorIndexPathDoesNotExist {
			index, err = bleve.New(opts.Path, indexMapping)
		}
	}

	if err != nil {
		return nil, err
	}

	return &BleveIndex{index: index}, nil
}

// buildIndexMapping creates the Bleve index mapping for Nostr events.
func buildIndexMapping() *mapping.IndexMappingImpl {
	indexMapping := bleve.NewIndexMapping()

	// Event document mapping
	eventMapping := bleve.NewDocumentMapping()

	// content field: CJK analyzer for Japanese/Chinese/Korean support
	contentField := bleve.NewTextFieldMapping()
	contentField.Analyzer = cjk.AnalyzerName
	eventMapping.AddFieldMappingsAt("content", contentField)

	// id field: stored but not indexed (we search by content, not by ID)
	idField := bleve.NewTextFieldMapping()
	idField.Index = false
	idField.Store = true
	eventMapping.AddFieldMappingsAt("id", idField)

	// pubkey field: keyword (exact match, not analyzed)
	pubkeyField := bleve.NewKeywordFieldMapping()
	eventMapping.AddFieldMappingsAt("pubkey", pubkeyField)

	// kind field: numeric
	kindField := bleve.NewNumericFieldMapping()
	eventMapping.AddFieldMappingsAt("kind", kindField)

	// created_at field: numeric (for sorting)
	createdAtField := bleve.NewNumericFieldMapping()
	eventMapping.AddFieldMappingsAt("created_at", createdAtField)

	indexMapping.AddDocumentMapping("event", eventMapping)
	indexMapping.DefaultMapping = eventMapping

	return indexMapping
}

// bleveEvent is the document structure for indexing.
type bleveEvent struct {
	ID        string `json:"id"`
	Pubkey    string `json:"pubkey"`
	Kind      int64  `json:"kind"`
	CreatedAt int64  `json:"created_at"`
	Content   string `json:"content"`
}

// Index implements SearchIndex.Index.
func (b *BleveIndex) Index(ctx context.Context, event *Event) error {
	if event == nil {
		return nil
	}

	doc := bleveEvent{
		ID:        event.ID,
		Pubkey:    event.Pubkey,
		Kind:      event.Kind,
		CreatedAt: event.CreatedAt.Unix(),
		Content:   event.Content,
	}

	return b.index.Index(event.ID, doc)
}

// Search implements SearchIndex.Search.
func (b *BleveIndex) Search(ctx context.Context, query string, limit int64) ([]string, error) {
	if query == "" {
		return nil, nil
	}

	// Create a match query on the content field
	matchQuery := bleve.NewMatchQuery(query)
	matchQuery.SetField("content")

	searchRequest := bleve.NewSearchRequest(matchQuery)
	if limit > 0 {
		searchRequest.Size = int(limit)
	}
	// Sort by score (relevance) by default

	result, err := b.index.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	eventIDs := make([]string, 0, len(result.Hits))
	for _, hit := range result.Hits {
		eventIDs = append(eventIDs, hit.ID)
	}

	return eventIDs, nil
}

// Delete implements SearchIndex.Delete.
func (b *BleveIndex) Delete(ctx context.Context, eventID string) error {
	return b.index.Delete(eventID)
}

// Close implements SearchIndex.Close.
func (b *BleveIndex) Close() error {
	return b.index.Close()
}

// DocCount returns the number of documents in the index.
// Useful for testing and monitoring.
func (b *BleveIndex) DocCount() (uint64, error) {
	return b.index.DocCount()
}
