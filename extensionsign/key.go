package extensionsign

import (
	"crypto/ed25519"
	"crypto/rand"
)

func GenerateKey() (ed25519.PrivateKey, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return private, nil
}
