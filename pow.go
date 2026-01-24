//go:build goexperiment.jsonv2

package mocrelay

// CountLeadingZeroBits counts the number of leading zero bits in a hex string.
// This is used for NIP-13 Proof of Work validation.
//
// Example:
//
//	CountLeadingZeroBits("000000000e9d97a1ab09fc381030b346cdd7a142ad57e6df0b46dc9bef6c7e2d") // returns 36
//	CountLeadingZeroBits("002f...") // returns 10 (0000 0000 0010 1111...)
func CountLeadingZeroBits(hex string) int {
	count := 0

	for i := 0; i < len(hex); i++ {
		nibble := hexToNibble(hex[i])
		if nibble < 0 {
			// Invalid hex character, stop counting
			break
		}

		if nibble == 0 {
			count += 4
		} else {
			// Count leading zeros in this nibble (0-3)
			count += clz4(nibble)
			break
		}
	}

	return count
}

// hexToNibble converts a hex character to its numeric value (0-15).
// Returns -1 for invalid characters.
func hexToNibble(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c - 'a' + 10)
	case 'A' <= c && c <= 'F':
		return int(c - 'A' + 10)
	default:
		return -1
	}
}

// clz4 counts leading zeros in a 4-bit nibble (0-15).
// Returns 0-3 (not 4, since nibble=0 is handled separately).
func clz4(nibble int) int {
	// nibble: 1=3, 2-3=2, 4-7=1, 8-15=0
	switch {
	case nibble >= 8:
		return 0
	case nibble >= 4:
		return 1
	case nibble >= 2:
		return 2
	default:
		return 3
	}
}
