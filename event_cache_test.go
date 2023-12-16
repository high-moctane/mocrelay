package mocrelay

import (
	"fmt"
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func TestEventCache_Add(t *testing.T) {
	regularEvents := func() []*Event {
		var ret []*Event
		for i := 0; i < 10; i++ {
			ret = append(ret, &Event{
				ID:        fmt.Sprintf("event-%d", i),
				Pubkey:    "regular",
				Kind:      1,
				CreatedAt: int64(i),
			})
		}
		return ret
	}()
	replaceableEvents := func() map[string]map[int64][]*Event {
		pubkeys := []string{"pubkey01", "pubkey02", "pubkey03"}
		kinds := []int64{10000, 10001, 10002}
		ret := make(map[string]map[int64][]*Event)
		for _, pubkey := range pubkeys {
			ret[pubkey] = make(map[int64][]*Event)
			for _, kind := range kinds {
				for i := 0; i < 10; i++ {
					ret[pubkey][kind] = append(ret[pubkey][kind], &Event{
						ID:        fmt.Sprintf("event-%s-%d-%d", pubkey, kind, i),
						Pubkey:    pubkey,
						Kind:      kind,
						CreatedAt: int64(i),
					})
				}
			}
		}
		return ret
	}()
	paramReplaceableEvents := func() map[string]map[int64]map[string][]*Event {
		pubkeys := []string{"pubkey01", "pubkey02", "pubkey03"}
		kinds := []int64{30000, 30001, 30002}
		params := []string{"param01", "param02", "param03"}
		ret := make(map[string]map[int64]map[string][]*Event)
		for _, pubkey := range pubkeys {
			ret[pubkey] = make(map[int64]map[string][]*Event)
			for _, kind := range kinds {
				ret[pubkey][kind] = make(map[string][]*Event)
				for _, param := range params {
					for i := 0; i < 10; i++ {
						ret[pubkey][kind][param] = append(ret[pubkey][kind][param], &Event{
							ID:        fmt.Sprintf("event-%s-%d-%s-%d", pubkey, kind, param, i),
							Pubkey:    pubkey,
							Kind:      kind,
							CreatedAt: int64(i),
							Tags: []Tag{
								{"d", param},
							},
						})
					}
				}
			}
		}
		return ret
	}()

	type input struct {
		event *Event
		added bool
	}

	tests := []struct {
		name    string
		cap     int
		in      []*input
		filters []*ReqFilter
		found   []*Event
		len     int
	}{
		{
			name:    "empty",
			cap:     5,
			in:      []*input{},
			filters: []*ReqFilter{{}},
			found:   nil,
			len:     0,
		},
		{
			name: "one",
			cap:  5,
			in: []*input{
				{event: regularEvents[0], added: true},
			},
			filters: []*ReqFilter{{}},
			found:   []*Event{regularEvents[0]},
			len:     1,
		},
		{
			name: "full",
			cap:  5,
			in: []*input{
				{event: regularEvents[0], added: true},
				{event: regularEvents[1], added: true},
				{event: regularEvents[2], added: true},
				{event: regularEvents[3], added: true},
				{event: regularEvents[4], added: true},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				regularEvents[4],
				regularEvents[3],
				regularEvents[2],
				regularEvents[1],
				regularEvents[0],
			},
			len: 5,
		},
		{
			name: "full and more",
			cap:  5,
			in: []*input{
				{event: regularEvents[0], added: true},
				{event: regularEvents[1], added: true},
				{event: regularEvents[2], added: true},
				{event: regularEvents[3], added: true},
				{event: regularEvents[4], added: true},
				{event: regularEvents[5], added: true},
				{event: regularEvents[6], added: true},
				{event: regularEvents[7], added: true},
				{event: regularEvents[8], added: true},
				{event: regularEvents[9], added: true},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				regularEvents[9],
				regularEvents[8],
				regularEvents[7],
				regularEvents[6],
				regularEvents[5],
			},
			len: 5,
		},
		{
			name: "full and more random order",
			cap:  5,
			in: []*input{
				{event: regularEvents[7], added: true},
				{event: regularEvents[3], added: true},
				{event: regularEvents[9], added: true},
				{event: regularEvents[1], added: true},
				{event: regularEvents[5], added: true},
				{event: regularEvents[0], added: true},
				{event: regularEvents[4], added: true},
				{event: regularEvents[6], added: true},
				{event: regularEvents[8], added: true},
				{event: regularEvents[2], added: true},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				regularEvents[9],
				regularEvents[8],
				regularEvents[7],
				regularEvents[6],
				regularEvents[5],
			},
			len: 5,
		},
		{
			name: "full and more random order duplicate",
			cap:  5,
			in: []*input{
				{event: regularEvents[7], added: true},
				{event: regularEvents[3], added: true},
				{event: regularEvents[7], added: false},
				{event: regularEvents[9], added: true},
				{event: regularEvents[1], added: true},
				{event: regularEvents[3], added: false},
				{event: regularEvents[5], added: true},
				{event: regularEvents[0], added: true},
				{event: regularEvents[4], added: true},
				{event: regularEvents[6], added: true},
				{event: regularEvents[8], added: true},
				{event: regularEvents[2], added: true},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				regularEvents[9],
				regularEvents[8],
				regularEvents[7],
				regularEvents[6],
				regularEvents[5],
			},
			len: 5,
		},
		{
			name: "full replaceable",
			cap:  5,
			in: []*input{
				{event: replaceableEvents["pubkey01"][10000][0], added: true},
				{event: replaceableEvents["pubkey01"][10000][1], added: true},
				{event: replaceableEvents["pubkey01"][10000][2], added: true},
				{event: replaceableEvents["pubkey01"][10000][3], added: true},
				{event: replaceableEvents["pubkey01"][10000][4], added: true},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				replaceableEvents["pubkey01"][10000][4],
			},
			len: 1,
		},
		{
			name: "full and more replaceable",
			cap:  5,
			in: []*input{
				{event: replaceableEvents["pubkey01"][10000][0], added: true},
				{event: replaceableEvents["pubkey01"][10000][1], added: true},
				{event: replaceableEvents["pubkey01"][10000][2], added: true},
				{event: replaceableEvents["pubkey01"][10000][3], added: true},
				{event: replaceableEvents["pubkey01"][10000][4], added: true},
				{event: replaceableEvents["pubkey01"][10000][5], added: true},
				{event: replaceableEvents["pubkey01"][10000][6], added: true},
				{event: replaceableEvents["pubkey01"][10000][7], added: true},
				{event: replaceableEvents["pubkey01"][10000][8], added: true},
				{event: replaceableEvents["pubkey01"][10000][9], added: true},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				replaceableEvents["pubkey01"][10000][9],
			},
			len: 1,
		},
		{
			name: "full and more random order replaceable",
			cap:  5,
			in: []*input{
				{event: replaceableEvents["pubkey01"][10000][3], added: true},
				{event: replaceableEvents["pubkey01"][10000][1], added: false},
				{event: replaceableEvents["pubkey01"][10000][5], added: true},
				{event: replaceableEvents["pubkey01"][10000][7], added: true},
				{event: replaceableEvents["pubkey01"][10000][0], added: false},
				{event: replaceableEvents["pubkey01"][10000][4], added: false},
				{event: replaceableEvents["pubkey01"][10000][6], added: false},
				{event: replaceableEvents["pubkey01"][10000][9], added: true},
				{event: replaceableEvents["pubkey01"][10000][8], added: false},
				{event: replaceableEvents["pubkey01"][10000][2], added: false},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				replaceableEvents["pubkey01"][10000][9],
			},
			len: 1,
		},
		{
			name: "full and more random order random kind replaceable",
			cap:  5,
			in: []*input{
				{event: replaceableEvents["pubkey01"][10000][3], added: true},
				{event: replaceableEvents["pubkey01"][10001][3], added: true},
				{event: replaceableEvents["pubkey01"][10002][3], added: true},
				{event: replaceableEvents["pubkey01"][10000][1], added: false},
				{event: replaceableEvents["pubkey01"][10001][1], added: false},
				{event: replaceableEvents["pubkey01"][10002][1], added: false},
				{event: replaceableEvents["pubkey01"][10000][5], added: true},
				{event: replaceableEvents["pubkey01"][10001][5], added: true},
				{event: replaceableEvents["pubkey01"][10002][5], added: true},
				{event: replaceableEvents["pubkey01"][10000][7], added: true},
				{event: replaceableEvents["pubkey01"][10001][7], added: true},
				{event: replaceableEvents["pubkey01"][10002][7], added: true},
				{event: replaceableEvents["pubkey01"][10000][0], added: false},
				{event: replaceableEvents["pubkey01"][10001][0], added: false},
				{event: replaceableEvents["pubkey01"][10002][0], added: false},
				{event: replaceableEvents["pubkey01"][10000][4], added: false},
				{event: replaceableEvents["pubkey01"][10001][4], added: false},
				{event: replaceableEvents["pubkey01"][10002][4], added: false},
				{event: replaceableEvents["pubkey01"][10000][6], added: false},
				{event: replaceableEvents["pubkey01"][10001][6], added: false},
				{event: replaceableEvents["pubkey01"][10002][6], added: false},
				{event: replaceableEvents["pubkey01"][10000][9], added: true},
				{event: replaceableEvents["pubkey01"][10001][9], added: true},
				{event: replaceableEvents["pubkey01"][10002][9], added: true},
				{event: replaceableEvents["pubkey01"][10000][8], added: false},
				{event: replaceableEvents["pubkey01"][10001][8], added: false},
				{event: replaceableEvents["pubkey01"][10002][8], added: false},
				{event: replaceableEvents["pubkey01"][10000][2], added: false},
				{event: replaceableEvents["pubkey01"][10001][2], added: false},
				{event: replaceableEvents["pubkey01"][10002][2], added: false},
			},
			filters: []*ReqFilter{{}},
			found: []*Event{
				replaceableEvents["pubkey01"][10002][9],
				replaceableEvents["pubkey01"][10001][9],
				replaceableEvents["pubkey01"][10000][9],
			},
			len: 3,
		},
		{
			name: "full and more random order random kind parametrized replaceable",
			cap:  30,
			in: func() []*input {
				var ret []*input
				for _, pubkey := range []string{"pubkey01", "pubkey02", "pubkey03"} {
					for _, kind := range []int64{30000, 30001, 30002} {
						for _, param := range []string{"param01", "param02", "param03"} {
							ret = append(ret, []*input{
								{
									event: paramReplaceableEvents[pubkey][kind][param][3],
									added: true,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][1],
									added: false,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][5],
									added: true,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][7],
									added: true,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][0],
									added: false,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][4],
									added: false,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][6],
									added: false,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][9],
									added: true,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][8],
									added: false,
								},
								{
									event: paramReplaceableEvents[pubkey][kind][param][2],
									added: false,
								},
							}...)
						}
					}
				}
				return ret
			}(),
			filters: []*ReqFilter{{}},
			found: []*Event{
				paramReplaceableEvents["pubkey03"][30002]["param03"][9],
				paramReplaceableEvents["pubkey03"][30002]["param02"][9],
				paramReplaceableEvents["pubkey03"][30002]["param01"][9],
				paramReplaceableEvents["pubkey03"][30001]["param03"][9],
				paramReplaceableEvents["pubkey03"][30001]["param02"][9],
				paramReplaceableEvents["pubkey03"][30001]["param01"][9],
				paramReplaceableEvents["pubkey03"][30000]["param03"][9],
				paramReplaceableEvents["pubkey03"][30000]["param02"][9],
				paramReplaceableEvents["pubkey03"][30000]["param01"][9],
				paramReplaceableEvents["pubkey02"][30002]["param03"][9],
				paramReplaceableEvents["pubkey02"][30002]["param02"][9],
				paramReplaceableEvents["pubkey02"][30002]["param01"][9],
				paramReplaceableEvents["pubkey02"][30001]["param03"][9],
				paramReplaceableEvents["pubkey02"][30001]["param02"][9],
				paramReplaceableEvents["pubkey02"][30001]["param01"][9],
				paramReplaceableEvents["pubkey02"][30000]["param03"][9],
				paramReplaceableEvents["pubkey02"][30000]["param02"][9],
				paramReplaceableEvents["pubkey02"][30000]["param01"][9],
				paramReplaceableEvents["pubkey01"][30002]["param03"][9],
				paramReplaceableEvents["pubkey01"][30002]["param02"][9],
				paramReplaceableEvents["pubkey01"][30002]["param01"][9],
				paramReplaceableEvents["pubkey01"][30001]["param03"][9],
				paramReplaceableEvents["pubkey01"][30001]["param02"][9],
				paramReplaceableEvents["pubkey01"][30001]["param01"][9],
				paramReplaceableEvents["pubkey01"][30000]["param03"][9],
				paramReplaceableEvents["pubkey01"][30000]["param02"][9],
				paramReplaceableEvents["pubkey01"][30000]["param01"][9],
			},
			len: 27,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewEventCache(tt.cap)
			for _, in := range tt.in {
				gotAdded := c.Add(in.event)
				assert.Equal(t, in.added, gotAdded, c.getEventKey(in.event))
			}

			found := c.Find(tt.filters)
			assert.Equal(t, tt.found, found)

			assert.Equal(t, tt.len, c.Len())
		})
	}
}

func TestEventCache_getEventCache(t *testing.T) {
	tests := []struct {
		name string
		in   *Event
		want string
	}{
		{
			name: "regular",
			in: &Event{
				ID:        "event-1",
				Pubkey:    "regular",
				Kind:      1,
				CreatedAt: 1,
				Tags:      []Tag{},
			},
			want: "event-1",
		},
		{
			name: "replaceable",
			in: &Event{
				ID:        "event-1",
				Pubkey:    "replaceable",
				Kind:      10000,
				CreatedAt: 1,
				Tags:      []Tag{},
			},
			want: "replaceable:10000",
		},
		{
			name: "parametrized replaceable",
			in: &Event{
				ID:        "event-1",
				Pubkey:    "param-replaceable",
				Kind:      30000,
				CreatedAt: 1,
				Tags: []Tag{
					{"d", "param"},
				},
			},
			want: "param-replaceable:30000:param",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewEventCache(5)
			got := c.getEventKey(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEventCache_AddKind5(t *testing.T) {
	type input struct {
		event *Event
		added bool
	}

	tests := []struct {
		name       string
		cap        int
		in         []*input
		filters    []*ReqFilter
		want       []*Event
		len        int
		deletedLen int
	}{
		{
			name: "kind5 after event",
			cap:  5,
			in: []*input{
				{
					event: &Event{
						ID:        "event-1",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 1,
					},
					added: true,
				},
				{
					event: &Event{
						ID:     "kind5",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "event-1"},
						},
						CreatedAt: 2,
					},
					added: true,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:     "kind5",
					Pubkey: "pubkey01",
					Kind:   5,
					Tags: []Tag{
						{"e", "event-1"},
					},
					CreatedAt: 2,
				},
			},
			len:        1,
			deletedLen: 1,
		},
		{
			name: "kind5 before event",
			cap:  5,
			in: []*input{
				{
					event: &Event{
						ID:     "kind5",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "event-1"},
						},
						CreatedAt: 2,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-1",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 1,
					},
					added: false,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:     "kind5",
					Pubkey: "pubkey01",
					Kind:   5,
					Tags: []Tag{
						{"e", "event-1"},
					},
					CreatedAt: 2,
				},
			},
			len:        1,
			deletedLen: 1,
		},
		{
			name: "kind5 for parametrized replaceable event",
			cap:  5,
			in: []*input{
				{
					event: &Event{
						ID:     "kind5",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"a", "pubkey01:30000:param"},
						},
						CreatedAt: 2,
					},
					added: true,
				},
				{
					event: &Event{
						ID:     "event-1",
						Pubkey: "pubkey01",
						Kind:   30000,
						Tags: []Tag{
							{"d", "param"},
						},
						CreatedAt: 1,
					},
					added: false,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:     "kind5",
					Pubkey: "pubkey01",
					Kind:   5,
					Tags: []Tag{
						{"a", "pubkey01:30000:param"},
					},
					CreatedAt: 2,
				},
			},
			len:        1,
			deletedLen: 1,
		},
		{
			name: "delete oneself",
			cap:  5,
			in: []*input{
				{
					event: &Event{
						ID:     "kind5",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "kind5"},
						},
						CreatedAt: 1,
					},
					added: true,
				},
			},
			filters:    []*ReqFilter{{}},
			want:       nil,
			len:        0,
			deletedLen: 0,
		},
		{
			name: "kind5 for kind5",
			cap:  5,
			in: []*input{
				{
					event: &Event{
						ID:     "kind5",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "kind5"},
						},
						CreatedAt: 1,
					},
					added: true,
				},
				{
					event: &Event{
						ID:     "kind5-2",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "kind5"},
						},
						CreatedAt: 2,
					},
					added: true,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:     "kind5-2",
					Pubkey: "pubkey01",
					Kind:   5,
					Tags: []Tag{
						{"e", "kind5"},
					},
					CreatedAt: 2,
				},
			},
			len:        1,
			deletedLen: 1,
		},
		{
			name: "kind5 with different pubkey",
			cap:  5,
			in: []*input{
				{
					event: &Event{
						ID:        "event-1",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 1,
					},
					added: true,
				},
				{
					event: &Event{
						ID:     "kind5-3",
						Pubkey: "pubkey02",
						Kind:   5,
						Tags: []Tag{
							{"e", "event-1"},
						},
						CreatedAt: 2,
					},
					added: true,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:     "kind5-3",
					Pubkey: "pubkey02",
					Kind:   5,
					Tags: []Tag{
						{"e", "event-1"},
					},
					CreatedAt: 2,
				},
				{
					ID:        "event-1",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 1,
				},
			},
			len:        2,
			deletedLen: 1,
		},
		{
			name: "many events",
			cap:  3,
			in: []*input{
				{
					event: &Event{
						ID:     "kind5-1",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "event-1"},
						},
						CreatedAt: 1,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-2",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 2,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-3",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 3,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-4",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 4,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-1",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 5,
					},
					added: true,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:        "event-1",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 5,
				},
				{
					ID:        "event-4",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 4,
				},
				{
					ID:        "event-3",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 3,
				},
			},
			len:        3,
			deletedLen: 0,
		},
		{
			name: "many events with multiple same kind5",
			cap:  3,
			in: []*input{
				{
					event: &Event{
						ID:     "kind5-1",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "event-1"},
						},
						CreatedAt: 1,
					},
					added: true,
				},
				{
					event: &Event{
						ID:     "kind5-2",
						Pubkey: "pubkey01",
						Kind:   5,
						Tags: []Tag{
							{"e", "event-1"},
						},
						CreatedAt: 2,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-2",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 3,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-3",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 4,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-1",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 10,
					},
					added: false,
				},
				{
					event: &Event{
						ID:        "event-4",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 5,
					},
					added: true,
				},
				{
					event: &Event{
						ID:        "event-1",
						Pubkey:    "pubkey01",
						Kind:      1,
						CreatedAt: 10,
					},
					added: true,
				},
			},
			filters: []*ReqFilter{{}},
			want: []*Event{
				{
					ID:        "event-1",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 10,
				},

				{
					ID:        "event-4",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 5,
				},
				{
					ID:        "event-3",
					Pubkey:    "pubkey01",
					Kind:      1,
					CreatedAt: 4,
				},
			},
			len:        3,
			deletedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewEventCache(tt.cap)
			for _, in := range tt.in {
				gotAdded := c.Add(in.event)
				assert.Equal(t, in.added, gotAdded, c.getEventKey(in.event))
			}

			found := c.Find(tt.filters)
			assert.Equal(t, tt.want, found)

			assert.Equal(t, tt.len, c.Len())
			assert.Equal(t, tt.deletedLen, len(c.deleted))
		})
	}
}
