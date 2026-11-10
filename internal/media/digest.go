package media

import (
	"crypto/sha256"
	"encoding/hex"
)

func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func CombineDigest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
