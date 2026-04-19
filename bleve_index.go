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
}

// BleveIndex implements SearchIndex using Bleve with CJK analyzer.
//
// The [bleve.Index] is owned by the caller: BleveIndex does not open or
// close it. Callers should create the index with [BuildIndexMapping] (or
// a customized mapping built on top of it) so the document schema matches
// what Index / Search / Delete expect, then close the underlying index
// when done. BleveIndex itself has no Close method.
type BleveIndex struct {
	index bleve.Index
}

// BleveIndexOptions is reserved for future BleveIndex configuration.
// It is currently empty but exists so that adding options later is not a
// breaking API change. Pass nil for now.
type BleveIndexOptions struct{}

// NewBleveIndex wraps an already-open [bleve.Index] as a [SearchIndex].
//
// The caller retains ownership of idx: they are responsible for opening it
// (typically with [BuildIndexMapping]) and for closing it when done.
// BleveIndex itself has no Close method.
//
// opts can be nil; there are currently no configurable options.
func NewBleveIndex(idx bleve.Index, opts *BleveIndexOptions) *BleveIndex {
	_ = opts // reserved for future use
	return &BleveIndex{index: idx}
}

// BuildIndexMapping returns the Bleve [mapping.IndexMappingImpl] that
// BleveIndex expects. Use it when opening a [bleve.Index] to pass to
// [NewBleveIndex]:
//
//	idx, _ := bleve.NewMemOnly(mocrelay.BuildIndexMapping())
//	searchIndex := mocrelay.NewBleveIndex(idx, nil)
//
// It configures:
//   - "content" as a text field analyzed with the CJK analyzer (bigram
//     tokenization, covers Japanese / Chinese / Korean).
//   - "id" as stored-only (indexed=false) so hits can be mapped back to
//     event IDs without bloating the index.
//   - "pubkey" as a keyword (exact match) field.
//   - "kind" and "created_at" as numeric fields.
//
// Callers who need a customized mapping can build on top of the returned
// value, but Index / Search / Delete rely on the document shape above.
func BuildIndexMapping() *mapping.IndexMappingImpl {
	return buildIndexMapping()
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

func (b *BleveIndex) docCount() (uint64, error) {
	return b.index.DocCount()
}
