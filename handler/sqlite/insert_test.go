package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/high-moctane/mocrelay"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func Test_insertEvents(t *testing.T) {
	type try struct {
		events []*mocrelay.Event
	}

	tests := []struct {
		name               string
		try1               try
		try2               try
		total              int64
		totalLookups       int64
		totalDeletedEvents int64
	}{
		{
			name: "insert regular event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        2,
			totalLookups: 8,
		},
		{
			name: "insert regular event: duplicate id",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert replaceable event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert replaceable event: duplicate id",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert replaceable event: same",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert replaceable event: different pubkey",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey2",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        2,
			totalLookups: 8,
		},
		{
			name: "insert replaceable event: different kind",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      10001,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        2,
			totalLookups: 8,
		},
		{
			name: "insert replaceable event: duplicate pubkey and kind but too old",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 100,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert ephemeral event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:        0,
			totalLookups: 0,
		},
		{
			name: "insert parametrized replaceable event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert parametrized replaceable event: duplicate",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert parametrized replaceable event: same",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert parametrized replaceable event: same but too old",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 100,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        1,
			totalLookups: 4,
		},
		{
			name: "insert parametrized replaceable event: different pubkey",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey2",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        2,
			totalLookups: 8,
		},
		{
			name: "insert parametrized replaceable event: different kind",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      30001,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        2,
			totalLookups: 8,
		},
		{
			name: "insert parametrized replaceable event: different tag",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id2",
						Pubkey:    "pubkey",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        2,
			totalLookups: 8,
		},
		{
			name: "all",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id2",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id3",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id4",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			try2: try{
				events: []*mocrelay.Event{
					{
						ID:        "id1",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id11",
						Pubkey:    "pubkey1",
						CreatedAt: 2,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id2",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id21",
						Pubkey:    "pubkey1",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id22",
						Pubkey:    "pubkey2",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id3",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "id4",
						Pubkey:    "pubkey1",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
					{
						ID:        "id41",
						Pubkey:    "pubkey2",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:        6,
			totalLookups: 24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite3", ":memory:?cache=shared")
			if err != nil {
				t.Fatalf("failed to open db: %v", err)
			}
			defer db.Close()

			if _, err := db.ExecContext(ctx, `pragma foreign_keys = on`); err != nil {
				t.Fatalf("failed to enable foreign keys: %v", err)
			}

			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			if tt.try1.events != nil {
				err := insertEvents(ctx, db, tt.try1.events)
				assert.NoError(t, err, "failed to insert events try1: %v", err)
			}

			if tt.try2.events != nil {
				err := insertEvents(ctx, db, tt.try2.events)
				assert.NoError(t, err, "failed to insert events try2: %v", err)
			}

			var total int64
			err = db.QueryRowContext(ctx, "select count(*) from events").Scan(&total)
			assert.NoError(t, err, "failed to get total: %v", err)
			assert.Equal(t, tt.total, total)

			var totalLookups int64
			if err := db.QueryRowContext(ctx, "select count(*) from lookups").Scan(&totalLookups); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.totalLookups, totalLookups)
		})
	}
}

func Test_insertDeletedKeys(t *testing.T) {
	tests := []struct {
		name             string
		inputs           []*mocrelay.Event
		eventsTotal      int64
		deletedKeysTotal int64
	}{
		{
			name: "one kind5",
			inputs: []*mocrelay.Event{
				{
					ID:        "id1",
					Pubkey:    "pubkey",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id2"},
						{"e", "id3"},
					},
				},
			},
			eventsTotal:      1,
			deletedKeysTotal: 2,
		},
		{
			name: "two kind5",
			inputs: []*mocrelay.Event{
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id2"},
						{"a", "30000:pubkey:value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey2",
					CreatedAt: 2,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id4"},
						{"a", "30000:pubkey2:value"},
					},
				},
			},
			eventsTotal:      2,
			deletedKeysTotal: 4,
		},
		{
			name: "two kind5 with duplicate tag",
			inputs: []*mocrelay.Event{
				{
					ID:        "id1",
					Pubkey:    "pubkey",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id2"},
						{"e", "id3"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey",
					CreatedAt: 2,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id3"},
						{"e", "id4"},
					},
				},
			},
			eventsTotal:      2,
			deletedKeysTotal: 3,
		},
		{
			name: "two kind5 with duplicate tag different pubkey",
			inputs: []*mocrelay.Event{
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id2"},
						{"e", "id3"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey2",
					CreatedAt: 2,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "id3"},
						{"e", "id4"},
					},
				},
			},
			eventsTotal:      2,
			deletedKeysTotal: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite3", ":memory:?cache=shared")
			if err != nil {
				t.Fatalf("failed to open db: %v", err)
			}
			defer db.Close()

			if _, err := db.ExecContext(ctx, `pragma foreign_keys = on`); err != nil {
				t.Fatalf("failed to enable foreign keys: %v", err)
			}

			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			if err := insertEvents(ctx, db, tt.inputs); err != nil {
				t.Fatalf("failed to insert events: %v", err)
			}

			var eventsTotal int64
			if err := db.QueryRowContext(ctx, "select count(*) from events").Scan(&eventsTotal); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.eventsTotal, eventsTotal)

			var deletedKeysTotal int64
			if err := db.QueryRowContext(ctx, "select count(*) from deleted_keys").Scan(&deletedKeysTotal); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.deletedKeysTotal, deletedKeysTotal)
		})
	}
}
