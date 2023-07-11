package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseClientMsgJSON(t *testing.T) {
	type args struct {
		json string
	}
	type want struct {
		res ClientMsgJSON
		err error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			"ok: event msg",
			args{
				`["EVENT",{"kind":1,"pubkey":"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e","created_at":1688314821,"tags":[],"content":"ぽわ〜","id":"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844","sig":"fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21"}]`,
			},
			want{
				&ClientEventMsgJSON{
					EventJSON: &EventJSON{
						ID:        "00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1688314821,
						Kind:      1,
						Tags:      [][]string{},
						Content:   "ぽわ〜",
						Sig:       "fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21",
					},
				},
				nil,
			},
		},
		{
			"ng: too long event msg",
			args{
				`["EVENT","invalid",{"kind":1,"pubkey":"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e","created_at":1688314821,"tags":[],"content":"ぽわ〜","id":"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844","sig":"fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21"}]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: too short event msg",
			args{
				`["EVENT"]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: invalid event msg",
			args{
				`["EVENT",0]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ok: req msg",
			args{
				`["REQ","id1",{"ids":["id1","id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":11,"until":111,"limit":1},{"authors":["pub2","pub22"]},{}]`,
			},
			want{
				&ClientReqMsgJSON{
					SubscriptionID: "id1",
					FilterJSONs: []*FilterJSON{
						{
							IDs:     func() *[]string { v := []string{"id1", "id11"}; return &v }(),
							Authors: func() *[]string { v := []string{"pub1", "pub11"}; return &v }(),
							Kinds:   func() *[]int { v := []int{1, 11}; return &v }(),
							Etags:   func() *[]string { v := []string{"etag1", "etag11"}; return &v }(),
							Ptags:   func() *[]string { v := []string{"ptag1", "ptag11"}; return &v }(),
							Since:   func() *int { v := 11; return &v }(),
							Until:   func() *int { v := 111; return &v }(),
							Limit:   func() *int { v := 1; return &v }(),
						},
						{
							Authors: func() *[]string { v := []string{"pub2", "pub22"}; return &v }(),
						},
						{},
					},
				},
				nil,
			},
		},
		{
			"ng: req msg without SubscriptionID",
			args{
				`["REQ",{"ids":["id1","id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":11,"until":111,"limit":1},{"authors":["pub2","pub22"]},{}]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: req msg without any filter",
			args{
				`["REQ","id1"]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: req msg with invalid filter",
			args{
				`["REQ",{"ids":["id1","id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":"today","until":111,"limit":1},{"authors":["pub2","pub22"]},{}]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: too short req msg",
			args{
				`["REQ"]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ok: close msg",
			args{
				`["CLOSE", "id1"]`,
			},
			want{
				&ClientCloseMsgJSON{"id1"},
				nil,
			},
		},
		{
			"ng: too long close msg",
			args{
				`["CLOSE", "id1", "id2"]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: too short close msg",
			args{
				`["CLOSE"]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: invalid type close msg",
			args{
				`["CLOSE", 30]`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: not a json",
			args{
				`moctane`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: json object",
			args{
				`{type: "EVENT"}`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: broken json",
			args{
				`["REQ","id1",{"ids":["id1","id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":11,"until":111,"limit":1},{"authors":["pub2","pub22"]},{}`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseClientMsgJSON(tt.args.json)
			if tt.want.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.want.res, res)
		})
	}
}

func TestParseEventJSON(t *testing.T) {
	type args struct {
		json string
	}
	type want struct {
		event *EventJSON
		err   error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			"ok: full",
			args{
				`{
					"id": "a3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					"pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					"created_at": 1688314828,
					"kind": 1,
					"tags": [
						[
							"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root"
						],
						[
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"
						]
					],
					"content": "ぽわ〜",
					"sig": "5acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631"
				}`,
			},
			want{
				&EventJSON{
					ID:        "a3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "5acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
				nil,
			},
		},
		{
			"ok: partial",
			args{
				`{
					"kind": 1,
					"pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					"created_at": 1688314821,
					"tags": [],
					"content": "ぽわ〜",
					"id": "00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
					"sig": "fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21"
				}`,
			},
			want{
				&EventJSON{
					ID:        "00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314821,
					Kind:      1,
					Tags:      [][]string{},
					Content:   "ぽわ〜",
					Sig:       "fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21",
				},
				nil,
			},
		},
		{
			"ng: invalid json",
			args{
				`{,
					"kind": 1,
					"pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					"created_at": 1688314821,
					"tags": [],
					"content": "ぽわ〜",
					"id": "00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
					"sig": "fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21"
				}`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: invalid type",
			args{
				`{
					"kind": 1,
					"pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					"created_at": 1688314821,
					"tags": [],
					"content": 15,
					"id": "00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
					"sig": "fe5ee79be79cac4f3485d2287dc2843f06eae5a8ff1f062561503751c35041164e55e2bce744276260dff5392d3b4c1ca5ec54698acd04fa40334e2caab22a21"
				}`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: empty string",
			args{
				``,
			},
			want{
				nil,
				assert.AnError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseEventJSON(tt.args.json)
			if tt.want.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.want.event, res)
		})
	}
}

func TestEventJSON_Verify(t *testing.T) {
	type args struct {
		event *EventJSON
	}
	type want struct {
		ok  bool
		err error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			"ok",
			args{
				&EventJSON{
					ID:        "a3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "5acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
			},
			want{
				true,
				nil,
			},
		},
		{
			"ng: incorrect id",
			args{
				&EventJSON{
					ID:        "b3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "5acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
			},
			want{
				false,
				nil,
			},
		},
		{
			"ng: incorrect sig",
			args{
				&EventJSON{
					ID:        "a3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "6acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
			},
			want{
				false,
				nil,
			},
		},
		{
			"ng: incorrect id and sig",
			args{
				&EventJSON{
					ID:        "b3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "6acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
			},
			want{
				false,
				nil,
			},
		},
		{
			"ng: non hex id",
			args{
				&EventJSON{
					ID:        "Z3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "5acc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
			},
			want{
				false,
				assert.AnError,
			},
		},
		{
			"ng: non hex sig",
			args{
				&EventJSON{
					ID:        "a3cc6206905faa34cdfb4849ecaceb4bd62a0d99d953f06fbc01d98d87ea8626",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1688314828,
					Kind:      1,
					Tags: [][]string{
						{"e",
							"00a6cc6a39f88a7ee8d3ed162532b9598b2c680ac5eda764d9c5716df894e844",
							"",
							"root",
						},
						{
							"p",
							"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						},
					},
					Content: "ぽわ〜",
					Sig:     "Zacc0bf523404dd5141a1cedff73448084dcda73089d1b3aee4bccacd545c44072a7d39e7726dced09de4023716a0a7a23ca57aad88db976a1541209c316e631",
				},
			},
			want{
				false,
				assert.AnError,
			},
		},
		{
			"ng: nil",
			args{
				nil,
			},
			want{
				false,
				assert.AnError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := tt.args.event.Verify()
			if tt.want.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.want.ok, ok)
		})
	}
}

func TestParseFilterJSON(t *testing.T) {
	type args struct {
		json string
	}
	type want struct {
		filter *FilterJSON
		err    error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			"ok: full",
			args{
				`{"ids":["id1","id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":11,"until":111,"limit":1}`,
			},
			want{
				&FilterJSON{
					IDs:     func() *[]string { v := []string{"id1", "id11"}; return &v }(),
					Authors: func() *[]string { v := []string{"pub1", "pub11"}; return &v }(),
					Kinds:   func() *[]int { v := []int{1, 11}; return &v }(),
					Etags:   func() *[]string { v := []string{"etag1", "etag11"}; return &v }(),
					Ptags:   func() *[]string { v := []string{"ptag1", "ptag11"}; return &v }(),
					Since:   func() *int { v := 11; return &v }(),
					Until:   func() *int { v := 111; return &v }(),
					Limit:   func() *int { v := 1; return &v }(),
				},
				nil,
			},
		},
		{
			"ok: partial",
			args{
				`{"authors":["pub1","pub11"],"limit":100}`,
			},
			want{
				&FilterJSON{
					Authors: func() *[]string { v := []string{"pub1", "pub11"}; return &v }(),
					Limit:   func() *int { v := 100; return &v }(),
				},
				nil,
			},
		},
		{
			"ok: empty object",
			args{
				`{}`,
			},
			want{
				&FilterJSON{},
				nil,
			},
		},
		{
			"ng: invalid json",
			args{
				`{ids":["id1","id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":11,"until":111,"limit":1}`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: invalid type",
			args{
				`{"ids":[3,"id11"],"authors":["pub1","pub11"],"kinds":[1,11],"#e":["etag1","etag11"],"#p":["ptag1","ptag11"],"since":11,"until":111,"limit":1}`,
			},
			want{
				nil,
				assert.AnError,
			},
		},
		{
			"ng: empty string",
			args{
				``,
			},
			want{
				nil,
				assert.AnError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseFilterJSON(tt.args.json)
			if tt.want.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.want.filter, res)
		})
	}
}
