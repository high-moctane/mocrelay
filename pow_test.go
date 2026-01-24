//go:build goexperiment.jsonv2

package mocrelay

import "testing"

func TestCountLeadingZeroBits(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want int
	}{
		{
			name: "NIP-13 example: 36 leading zeros",
			hex:  "000000000e9d97a1ab09fc381030b346cdd7a142ad57e6df0b46dc9bef6c7e2d",
			want: 36,
		},
		{
			name: "NIP-13 example: 002f has 10 leading zeros",
			hex:  "002f",
			want: 10,
		},
		{
			name: "all zeros (64 chars = 256 bits)",
			hex:  "0000000000000000000000000000000000000000000000000000000000000000",
			want: 256,
		},
		{
			name: "no leading zeros (starts with 8-f)",
			hex:  "8000000000000000000000000000000000000000000000000000000000000000",
			want: 0,
		},
		{
			name: "starts with 1 (3 leading zeros)",
			hex:  "1000000000000000000000000000000000000000000000000000000000000000",
			want: 3,
		},
		{
			name: "starts with 2 (2 leading zeros)",
			hex:  "2000000000000000000000000000000000000000000000000000000000000000",
			want: 2,
		},
		{
			name: "starts with 4 (1 leading zero)",
			hex:  "4000000000000000000000000000000000000000000000000000000000000000",
			want: 1,
		},
		{
			name: "starts with 7 (1 leading zero)",
			hex:  "7000000000000000000000000000000000000000000000000000000000000000",
			want: 1,
		},
		{
			name: "empty string",
			hex:  "",
			want: 0,
		},
		{
			name: "uppercase hex",
			hex:  "000000000E9D97A1AB09FC381030B346CDD7A142AD57E6DF0B46DC9BEF6C7E2D",
			want: 36,
		},
		{
			name: "mixed case",
			hex:  "000000000eAaBbCc",
			want: 36,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountLeadingZeroBits(tt.hex)
			if got != tt.want {
				t.Errorf("CountLeadingZeroBits(%q) = %d, want %d", tt.hex, got, tt.want)
			}
		})
	}
}

func TestHexToNibble(t *testing.T) {
	tests := []struct {
		c    byte
		want int
	}{
		{'0', 0}, {'1', 1}, {'2', 2}, {'3', 3}, {'4', 4},
		{'5', 5}, {'6', 6}, {'7', 7}, {'8', 8}, {'9', 9},
		{'a', 10}, {'b', 11}, {'c', 12}, {'d', 13}, {'e', 14}, {'f', 15},
		{'A', 10}, {'B', 11}, {'C', 12}, {'D', 13}, {'E', 14}, {'F', 15},
		{'g', -1}, {'z', -1}, {'!', -1}, {' ', -1},
	}

	for _, tt := range tests {
		got := hexToNibble(tt.c)
		if got != tt.want {
			t.Errorf("hexToNibble(%q) = %d, want %d", tt.c, got, tt.want)
		}
	}
}

func TestClz4(t *testing.T) {
	tests := []struct {
		nibble int
		want   int
	}{
		{1, 3},  // 0001 -> 3 leading zeros
		{2, 2},  // 0010 -> 2 leading zeros
		{3, 2},  // 0011 -> 2 leading zeros
		{4, 1},  // 0100 -> 1 leading zero
		{5, 1},  // 0101 -> 1 leading zero
		{6, 1},  // 0110 -> 1 leading zero
		{7, 1},  // 0111 -> 1 leading zero
		{8, 0},  // 1000 -> 0 leading zeros
		{9, 0},  // 1001 -> 0 leading zeros
		{10, 0}, // 1010 -> 0 leading zeros
		{15, 0}, // 1111 -> 0 leading zeros
	}

	for _, tt := range tests {
		got := clz4(tt.nibble)
		if got != tt.want {
			t.Errorf("clz4(%d) = %d, want %d", tt.nibble, got, tt.want)
		}
	}
}
