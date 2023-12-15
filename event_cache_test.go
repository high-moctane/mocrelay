package mocrelay

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventCache(t *testing.T) {
	reg := []*Event{
		{ID: "reg0", Pubkey: "reg0", Kind: 1, CreatedAt: 0},
		{ID: "reg1", Pubkey: "reg1", Kind: 1, CreatedAt: 1},
		{ID: "reg2", Pubkey: "reg2", Kind: 1, CreatedAt: 2},
		{ID: "reg3", Pubkey: "reg3", Kind: 1, CreatedAt: 3},
		{ID: "reg4", Pubkey: "reg4", Kind: 1, CreatedAt: 4},
		{ID: "reg5", Pubkey: "reg5", Kind: 1, CreatedAt: 5},
	}
	rep := []*Event{
		{ID: "rep0", Pubkey: "rep0", Kind: 0, CreatedAt: 0},
		{ID: "rep1", Pubkey: "rep0", Kind: 0, CreatedAt: 1},
		{ID: "rep2", Pubkey: "rep0", Kind: 10000, CreatedAt: 2},
		{ID: "rep3", Pubkey: "rep0", Kind: 10000, CreatedAt: 3},
		{ID: "rep4", Pubkey: "rep1", Kind: 0, CreatedAt: 4},
		{ID: "rep5", Pubkey: "rep1", Kind: 0, CreatedAt: 5},
		{ID: "rep6", Pubkey: "rep0", Kind: 0, CreatedAt: 6},
		{ID: "rep7", Pubkey: "rep0", Kind: 0, CreatedAt: 7},
	}
	prep := []*Event{
		{ID: "prep0", Pubkey: "prep0", Kind: 30000, CreatedAt: 0, Tags: []Tag{{"d", "tag0"}}},
		{ID: "prep1", Pubkey: "prep0", Kind: 30000, CreatedAt: 1, Tags: []Tag{{"d", "tag1"}}},
		{ID: "prep2", Pubkey: "prep0", Kind: 30000, CreatedAt: 2, Tags: []Tag{{"d", "tag0"}}},
		{ID: "prep3", Pubkey: "prep0", Kind: 30000, CreatedAt: 3, Tags: []Tag{{"d", "tag1"}}},
		{ID: "prep4", Pubkey: "prep1", Kind: 30000, CreatedAt: 4, Tags: []Tag{{"d", "tag0"}}},
		{ID: "prep5", Pubkey: "prep1", Kind: 30000, CreatedAt: 5, Tags: []Tag{{"d", "tag1"}}},
		{ID: "prep6", Pubkey: "prep1", Kind: 30000, CreatedAt: 6, Tags: []Tag{{"d", "tag0"}}},
		{ID: "prep7", Pubkey: "prep1", Kind: 30000, CreatedAt: 7, Tags: []Tag{{"d", "tag1"}}},
	}

	tests := []struct {
		name string
		cap  int
		in   []*Event
		want []*Event
	}{
		{
			"empty",
			3,
			nil,
			nil,
		},
		{
			"two",
			3,
			[]*Event{reg[0], reg[1]},
			[]*Event{reg[1], reg[0]},
		},
		{
			"many",
			3,
			[]*Event{reg[0], reg[1], reg[2], reg[3], reg[4]},
			[]*Event{reg[4], reg[3], reg[2]},
		},
		{
			"two: reverse",
			3,
			[]*Event{reg[1], reg[0]},
			[]*Event{reg[1], reg[0]},
		},
		{
			"random",
			3,
			[]*Event{reg[3], reg[2], reg[1], reg[4], reg[0]},
			[]*Event{reg[4], reg[3], reg[2]},
		},
		{
			"duplicate",
			3,
			[]*Event{reg[1], reg[1], reg[0], reg[1]},
			[]*Event{reg[1], reg[0]},
		},
		{
			"replaceable",
			3,
			[]*Event{rep[0], rep[1], rep[2]},
			[]*Event{rep[2], rep[1]},
		},
		{
			"replaceable: reverse",
			3,
			[]*Event{rep[1], rep[0], rep[2]},
			[]*Event{rep[2], rep[1]},
		},
		{
			"replaceable: different pubkey",
			3,
			[]*Event{rep[0], rep[4], rep[5]},
			[]*Event{rep[5], rep[0]},
		},
		{
			"param replaceable",
			5,
			[]*Event{prep[0], prep[1], prep[2], prep[3]},
			[]*Event{prep[3], prep[2]},
		},
		{
			"param replaceable: different pubkey",
			6,
			[]*Event{prep[0], prep[1], prep[2], prep[3], prep[4], prep[5], prep[6], prep[7]},
			[]*Event{prep[7], prep[6], prep[3], prep[2]},
		},
		{
			"param replaceable: different pubkey",
			6,
			[]*Event{prep[0], prep[1], prep[2], prep[3], prep[4], prep[5], prep[6], prep[7]},
			[]*Event{prep[7], prep[6], prep[3], prep[2]},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewEventCache(tt.cap)
			for _, e := range tt.in {
				c.Add(e)
			}
			got := c.Find([]*ReqFilter{{}})
			assert.Equal(t, tt.want, got)
		})
	}
}

func Benchmark_eventCache_Add(b *testing.B) {
	c := NewEventCache(10000)

	genKey := func() string {
		buf := make([]byte, 32)
		rand.Read(buf)
		return hex.EncodeToString(buf)
	}

	b.ResetTimer()
	b.RunParallel(func(b *testing.PB) {
		for i := 0; b.Next(); i++ {
			ev := Event{
				ID:        genKey(),
				Kind:      1,
				CreatedAt: int64(i + 4 - i%5),
			}

			c.Add(&ev)
		}
	})
}
