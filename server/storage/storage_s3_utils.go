package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

var noEscape [256]bool

func init() {
	for i := 0; i < len(noEscape); i++ {
		// AWS expects every character except these to be escaped
		noEscape[i] = (i >= 'A' && i <= 'Z') ||
			(i >= 'a' && i <= 'z') ||
			(i >= '0' && i <= '9') ||
			i == '-' ||
			i == '.' ||
			i == '_' ||
			i == '~'
	}
}

// escapePath escapes part of a URL path in Amazon style.
func escapePath(path string) string {
	var buf bytes.Buffer
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' || noEscape[c] {
			buf.WriteByte(c)
		} else {
			fmt.Fprintf(&buf, "%%%02X", c)
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
