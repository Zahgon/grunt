package lodash

import (
	"testing"
)

// TestCollectionNode24 replays lodash's array, collection, object, lang, math and
// utility functions recorded from Node 24.
func TestCollectionNode24(t *testing.T) { runFixtures(t, "testdata/collection.json.gz") }
