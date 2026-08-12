package usecase

import (
	"crypto/sha256"
	"encoding/base64"
)

func FormatAccountKey(accountID string) string {
	return opaqueReference("acct", "mihomo\x00"+accountID)
}

func FormatProfileRef(accountKey, gameBiz, region, playerID string) string {
	return opaqueReference("prof", accountKey+"\x00"+gameBiz+"\x00"+region+"\x00"+playerID)
}

func FormatDeviceRef(accountKey, deviceID string) string {
	return opaqueReference("dev", accountKey+"\x00"+deviceID)
}

func opaqueReference(prefix, canonical string) string {
	digest := sha256.Sum256([]byte(canonical))
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(digest[:24])
}
