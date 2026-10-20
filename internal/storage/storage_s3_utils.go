package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// escapePath escapes part of a URL path in Amazon style.
func escapePath(path string) string {
	const upperhex = "0123456789ABCDEF"
	var buf strings.Builder
	buf.Grow(len(path))
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' || c == '-' || c == '_' || c == '.' || c == '~' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			buf.WriteByte(c)
		} else if c == '+' {
			buf.WriteString("%20")
		} else {
			buf.WriteByte('%')
			buf.WriteByte(upperhex[c>>4])
			buf.WriteByte(upperhex[c&15])
		}
	}
	return buf.String()
}

// sha256Sum returns the SHA-256 checksum of the given data.
func sha256Sum(stringToSum string) []byte {
	hash := sha256.New()
	hash.Write([]byte(stringToSum))
	return hash.Sum(nil)
}

// hmacSum signs the given string with the provided key using HMAC-SHA256.
func hmacSum(key []byte, stringToSign string) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write([]byte(stringToSign))
	return hash.Sum(nil)
}

// toHex returns the hexadecimal representation of the given data.
func toHex(data []byte) string {
	return hex.EncodeToString(data)
}
