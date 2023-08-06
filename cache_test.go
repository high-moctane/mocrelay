package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventCache(t *testing.T) {
	type args struct {
		size       int
		defaultFil Filters
		fil        Filters
		input      []*Event
	}
	type want struct {
		saveds []bool
		output []*Event
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			"empty",
			args{
				3,
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				nil,
			},
			want{
				[]bool{},
				nil,
			},
		},
		{
			"less",
			args{
				3,
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				[]*Event{
					{EventJSON: &EventJSON{ID: "1"}},
					{EventJSON: &EventJSON{ID: "2"}},
					{EventJSON: &EventJSON{ID: "3"}},
				},
			},
			want{
				[]bool{true, true, true},
				[]*Event{
					{EventJSON: &EventJSON{ID: "3"}},
					{EventJSON: &EventJSON{ID: "2"}},
					{EventJSON: &EventJSON{ID: "1"}},
				},
			},
		},
		{
			"more",
			args{
				3,
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				[]*Event{
					{EventJSON: &EventJSON{ID: "1"}},
					{EventJSON: &EventJSON{ID: "2"}},
					{EventJSON: &EventJSON{ID: "3"}},
					{EventJSON: &EventJSON{ID: "4"}},
					{EventJSON: &EventJSON{ID: "5"}},
				},
			},
			want{
				[]bool{true, true, true, true, true},
				[]*Event{
					{EventJSON: &EventJSON{ID: "5"}},
					{EventJSON: &EventJSON{ID: "4"}},
					{EventJSON: &EventJSON{ID: "3"}},
				},
			},
		},
		{
			"not unique",
			args{
				3,
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				Filters{&Filter{FilterJSON: &FilterJSON{}}},
				[]*Event{
					{EventJSON: &EventJSON{ID: "1"}},
					{EventJSON: &EventJSON{ID: "1"}},
					{EventJSON: &EventJSON{ID: "1"}},
					{EventJSON: &EventJSON{ID: "1"}},
					{EventJSON: &EventJSON{ID: "1"}},
				},
			},
			want{
				[]bool{true, false, false, false, false},
				[]*Event{
					{EventJSON: &EventJSON{ID: "1"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewEventCache(tt.args.size, tt.args.defaultFil)

			for i, e := range tt.args.input {
				saved := cache.Save(e)
				assert.Equal(t, tt.want.saveds[i], saved)
			}

			events := cache.FindAll(tt.args.fil)
			assert.Equal(t, tt.want.output, events)
		})
	}
}
