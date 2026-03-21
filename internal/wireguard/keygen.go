//go:build windows

package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// KeyPair contiene el par de claves Curve25519 para WireGuard
type KeyPair struct {
	PrivateKey string // base64
	PublicKey  string // base64
}

// GenerateKeyPair genera un nuevo par de claves WireGuard (Curve25519)
func GenerateKeyPair() (KeyPair, error) {
	// Generar clave privada de 32 bytes aleatorios
	privateKeyBytes := make([]byte, 32)
	if _, err := rand.Read(privateKeyBytes); err != nil {
		return KeyPair{}, fmt.Errorf("error generando clave privada: %w", err)
	}

	// Clamp según especificación de Curve25519
	privateKeyBytes[0] &= 248
	privateKeyBytes[31] = (privateKeyBytes[31] & 127) | 64

	// Derivar clave pública
	publicKeyBytes, err := curve25519.X25519(privateKeyBytes, curve25519.Basepoint)
	if err != nil {
		return KeyPair{}, fmt.Errorf("error derivando clave pública: %w", err)
	}

	return KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(privateKeyBytes),
		PublicKey:  base64.StdEncoding.EncodeToString(publicKeyBytes),
	}, nil
}
