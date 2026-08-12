package operationid

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

func Fingerprint(kind, bindingRef string, preGeneration, targetGeneration uint64) string {
	digest := sha256.New()
	writeString(digest, kind)
	writeString(digest, bindingRef)
	var generations [16]byte
	binary.BigEndian.PutUint64(generations[:8], preGeneration)
	binary.BigEndian.PutUint64(generations[8:], targetGeneration)
	_, _ = digest.Write(generations[:])
	return hex.EncodeToString(digest.Sum(nil))
}

func AuthorizationFenceFingerprint(kind, bindingRef, consumer string, generation, minimumGrantVersion, minimumOwnerEpoch, minimumConsumerEpoch, minimumEntryEpoch uint64) string {
	digest := sha256.New()
	writeString(digest, kind)
	writeString(digest, bindingRef)
	writeString(digest, consumer)
	for _, value := range []uint64{generation, minimumGrantVersion, minimumOwnerEpoch, minimumConsumerEpoch, minimumEntryEpoch} {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], value)
		_, _ = digest.Write(encoded[:])
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func NewID() (string, error) {
	var value [18]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "op_" + base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func DeterministicID(fingerprint string) string {
	digest := sha256.Sum256([]byte("paigram-platform-operation:" + fingerprint))
	return "op_" + base64.RawURLEncoding.EncodeToString(digest[:18])
}

func writeString(digest hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write([]byte(value))
}
