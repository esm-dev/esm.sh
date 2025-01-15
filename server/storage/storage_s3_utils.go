package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// escapePath escapes part of a URL path in Amazon style.
func escapePath(path string) string {
	var buf strings.Builder
	for _, c := range path {
		if c == '/' || c == '-' || c == '_' || c == '.' || c == '~' {
			buf.WriteRune(c)
		} else if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			buf.WriteRune(c)
		} else if c == '+' {
			buf.WriteString("%20")
		} else {
			l := utf8.RuneLen(c)
			if l < 0 {
				// if utf8 cannot convert return the same string as is
				return path
			}
			u := make([]byte, l)
			utf8.EncodeRune(u, c)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				buf.WriteString("%" + strings.ToUpper(hex))
			}
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
