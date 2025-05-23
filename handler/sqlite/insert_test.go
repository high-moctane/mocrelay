package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"

	"github.com/high-moctane/mocrelay"
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
		totalTags          int64
		totalDeletedEvents int64
	}{
		{
			name: "insert regular event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     2,
			totalTags: 2,
		},
		{
			name: "insert regular event: duplicate id",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa11",
						Pubkey:    "bb",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert replaceable event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert replaceable event: duplicate id",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa11",
						Pubkey:    "bb",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert replaceable event: same",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert replaceable event: different pubkey",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb11",
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
						ID:        "aa22",
						Pubkey:    "bb22",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     2,
			totalTags: 2,
		},
		{
			name: "insert replaceable event: different kind",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      10001,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     2,
			totalTags: 2,
		},
		{
			name: "insert replaceable event: duplicate pubkey and kind but too old",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert ephemeral event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
				},
			},
			total:     0,
			totalTags: 0,
		},
		{
			name: "insert parametrized replaceable event",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert parametrized replaceable event: duplicate",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa11",
						Pubkey:    "bb",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert parametrized replaceable event: same",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert parametrized replaceable event: same but too old",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     1,
			totalTags: 1,
		},
		{
			name: "insert parametrized replaceable event: different pubkey",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb11",
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
						ID:        "aa22",
						Pubkey:    "bb22",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     2,
			totalTags: 2,
		},
		{
			name: "insert parametrized replaceable event: different kind",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      30001,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     2,
			totalTags: 2,
		},
		{
			name: "insert parametrized replaceable event: different tag",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb",
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
						ID:        "aa22",
						Pubkey:    "bb",
						CreatedAt: 2,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     2,
			totalTags: 2,
		},
		{
			name: "all",
			try1: try{
				events: []*mocrelay.Event{
					{
						ID:        "aa11",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa22",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa33",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa44",
						Pubkey:    "bb11",
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
						ID:        "aa11",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa1111",
						Pubkey:    "bb11",
						CreatedAt: 2,
						Kind:      1,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa22",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa2211",
						Pubkey:    "bb11",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa2222",
						Pubkey:    "bb22",
						CreatedAt: 2,
						Kind:      10000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa33",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      20000,
						Tags: []mocrelay.Tag{
							{"e", "value"},
						},
					},
					{
						ID:        "aa44",
						Pubkey:    "bb11",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
					{
						ID:        "aa4411",
						Pubkey:    "bb22",
						CreatedAt: 1,
						Kind:      30000,
						Tags: []mocrelay.Tag{
							{"d", "value"},
						},
					},
				},
			},
			total:     6,
			totalTags: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite3", ":memory:?cache=shared&foreign_keys=on")
			if err != nil {
				t.Fatalf("failed to open db: %v", err)
			}
			defer db.Close()

			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			if tt.try1.events != nil {
				err := insertEvents(ctx, db, 0, tt.try1.events)
				assert.NoError(t, err, "failed to insert events try1: %v", err)
			}

			if tt.try2.events != nil {
				err := insertEvents(ctx, db, 0, tt.try2.events)
				assert.NoError(t, err, "failed to insert events try2: %v", err)
			}

			var total int64
			err = db.QueryRowContext(ctx, "select count(*) from events").Scan(&total)
			assert.NoError(t, err, "failed to get total: %v", err)
			assert.Equal(t, tt.total, total, "total")

			var totalPayloads int64
			err = db.QueryRowContext(ctx, "select count(*) from event_payloads").
				Scan(&totalPayloads)
			assert.NoError(t, err, "failed to get total: %v", err)
			assert.Equal(t, tt.total, totalPayloads, "total payloads")

			var totalTags int64
			if err := db.QueryRowContext(ctx, "select count(*) from event_tags").Scan(&totalTags); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.totalTags, totalTags, "total tags")
		})
	}
}

func Test_insertDeletedKeysIds(t *testing.T) {
	tests := []struct {
		name             string
		inputs           []*mocrelay.Event
		eventsTotal      int64
		deletedKeysTotal int64
		deletedIDsTotal  int64
	}{
		{
			name: "one kind5",
			inputs: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa22"},
						{"e", "aa33"},
					},
				},
			},
			eventsTotal:      1,
			deletedKeysTotal: 0,
			deletedIDsTotal:  2,
		},
		{
			name: "two kind5",
			inputs: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa22"},
						{"a", "30000:bb11:value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 2,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa44"},
						{"a", "30000:bb22:value"},
					},
				},
			},
			eventsTotal:      2,
			deletedKeysTotal: 2,
			deletedIDsTotal:  2,
		},
		{
			name: "two kind5 with duplicate tag",
			inputs: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa22"},
						{"e", "aa33"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb",
					CreatedAt: 2,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa33"},
						{"e", "aa44"},
					},
				},
			},
			eventsTotal:      2,
			deletedKeysTotal: 0,
			deletedIDsTotal:  3,
		},
		{
			name: "two kind5 with duplicate tag different pubkey",
			inputs: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa22"},
						{"e", "aa33"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 2,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa33"},
						{"e", "aa44"},
					},
				},
			},
			eventsTotal:      2,
			deletedKeysTotal: 0,
			deletedIDsTotal:  4,
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

			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			if err := insertEvents(ctx, db, 0, tt.inputs); err != nil {
				t.Fatalf("failed to insert events: %v", err)
			}

			var eventsTotal int64
			if err := db.QueryRowContext(ctx, "select count(*) from events").Scan(&eventsTotal); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.eventsTotal, eventsTotal)

			var deletedKeysTotal int64
			if err := db.QueryRowContext(ctx, "select count(*) from deleted_event_keys").Scan(&deletedKeysTotal); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.deletedKeysTotal, deletedKeysTotal, "deleted keys total")

			var deletedIDsTotal int64
			if err := db.QueryRowContext(ctx, "select count(*) from deleted_event_ids").Scan(&deletedIDsTotal); err != nil {
				t.Fatalf("failed to get total: %v", err)
			}
			assert.Equal(t, tt.deletedIDsTotal, deletedIDsTotal, "deleted ids total")
		})
	}
}
