package base58

import (
	"bytes"
	"fmt"
	"math/big"
)

const Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// Encode encodes a byte slice into a Base58btc encoded string.
func Encode(src []byte) string {
	x := new(big.Int).SetBytes(src)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	var result []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append(result, Alphabet[mod.Int64()])
	}

	// Add leading zeroes (1 in base58 represents 0x00 byte)
	for _, b := range src {
		if b != 0x00 {
			break
		}
		result = append(result, Alphabet[0])
	}

	// Reverse the result slice
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// Decode decodes a Base58btc encoded string back to a byte slice.
func Decode(src string) ([]byte, error) {
	result := big.NewInt(0)
	base := big.NewInt(58)

	for i := 0; i < len(src); i++ {
		char := src[i]
		idx := bytes.IndexByte([]byte(Alphabet), char)
		if idx == -1 {
			return nil, fmt.Errorf("invalid base58 character: %q", char)
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}

	decoded := result.Bytes()

	// Add leading zeroes
	var numZeroes int
	for i := 0; i < len(src); i++ {
		if src[i] != Alphabet[0] {
			break
		}
		numZeroes++
	}

	res := make([]byte, numZeroes+len(decoded))
	copy(res[numZeroes:], decoded)
	return res, nil
}
