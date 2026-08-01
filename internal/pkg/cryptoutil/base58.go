package cryptoutil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// EncodeBase58 encodes a byte slice into a Base58btc encoded string.
func EncodeBase58(src []byte) string {
	x := new(big.Int).SetBytes(src)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	var result []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// Add leading zeroes (1 in base58 represents 0x00 byte)
	for _, b := range src {
		if b != 0x00 {
			break
		}
		result = append(result, base58Alphabet[0])
	}

	// Reverse the result slice
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// DecodeBase58 decodes a Base58btc encoded string back to a byte slice.
func DecodeBase58(src string) ([]byte, error) {
	result := big.NewInt(0)
	base := big.NewInt(58)

	for i := 0; i < len(src); i++ {
		char := src[i]
		idx := bytes.IndexByte([]byte(base58Alphabet), char)
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
		if src[i] != base58Alphabet[0] {
			break
		}
		numZeroes++
	}

	res := make([]byte, numZeroes+len(decoded))
	copy(res[numZeroes:], decoded)
	return res, nil
}

// ToDigestMultibase converts a raw SHA-256 byte slice to a base58btc encoded multihash digest string starting with "z"
func ToDigestMultibase(sha256Bytes []byte) string {
	// Multihash prefix: 0x12 (SHA-256) and 0x20 (length 32)
	multihash := make([]byte, 0, 2+len(sha256Bytes))
	multihash = append(multihash, 0x12, 0x20)
	multihash = append(multihash, sha256Bytes...)
	return "z" + EncodeBase58(multihash)
}

// ToDigestMultibaseFromHex converts a hex-encoded SHA-256 string to a multibase base58btc digest string starting with "z"
func ToDigestMultibaseFromHex(sha256Hex string) (string, error) {
	bytes, err := hex.DecodeString(sha256Hex)
	if err != nil {
		return "", err
	}
	return ToDigestMultibase(bytes), nil
}
