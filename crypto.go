package meshcore

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/curve25519"
)

const (
	cipherKeySize = 16 // AES-128
	cipherMACSize = 2  // truncated HMAC
)

// DeriveSharedSecret computes an X25519 shared secret from an Ed25519
// private key seed (32 bytes) and a peer's Ed25519 public key (32 bytes).
func DeriveSharedSecret(privateKeySeed []byte, peerPublicKey []byte) ([]byte, error) {
	if len(privateKeySeed) != 32 {
		return nil, fmt.Errorf("private key seed must be 32 bytes, got %d", len(privateKeySeed))
	}
	if len(peerPublicKey) != 32 {
		return nil, fmt.Errorf("peer public key must be 32 bytes, got %d", len(peerPublicKey))
	}

	// Convert Ed25519 private seed → X25519 private key
	x25519Private := edPrivateToX25519(privateKeySeed)

	// Convert Ed25519 public key → X25519 public key
	x25519Public, err := edPublicToX25519(peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("converting peer public key: %w", err)
	}

	// X25519 ECDH
	shared, err := curve25519.X25519(x25519Private, x25519Public)
	if err != nil {
		return nil, fmt.Errorf("computing shared secret: %w", err)
	}

	return shared, nil
}

// edPrivateToX25519 converts an Ed25519 private key seed to an X25519 private key.
// This matches the clamping done by RFC 8032 / libsodium.
func edPrivateToX25519(seed []byte) []byte {
	h := sha512.Sum512(seed)
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64
	return h[:32]
}

// edPublicToX25519 converts an Ed25519 public key to an X25519 public key
// using the birational map from Edwards to Montgomery form.
func edPublicToX25519(edPub []byte) ([]byte, error) {
	p, err := new(edwards25519.Point).SetBytes(edPub)
	if err != nil {
		return nil, err
	}
	return p.BytesMontgomery(), nil
}

// Decrypt decrypts src using AES-128 ECB with the first 16 bytes of sharedSecret.
func Decrypt(sharedSecret []byte, src []byte) ([]byte, error) {
	block, err := aes.NewCipher(sharedSecret[:cipherKeySize])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	dest := make([]byte, len(src))
	for i := 0; i+aes.BlockSize <= len(src); i += aes.BlockSize {
		block.Decrypt(dest[i:i+aes.BlockSize], src[i:i+aes.BlockSize])
	}

	return dest, nil
}

// Encrypt encrypts src using AES-128 ECB with the first 16 bytes of sharedSecret.
// Partial blocks are zero-padded to 16 bytes.
func Encrypt(sharedSecret []byte, src []byte) ([]byte, error) {
	block, err := aes.NewCipher(sharedSecret[:cipherKeySize])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	// Output is always a multiple of 16
	outLen := len(src)
	if outLen%aes.BlockSize != 0 {
		outLen += aes.BlockSize - (outLen % aes.BlockSize)
	}
	dest := make([]byte, outLen)

	i := 0
	for i+aes.BlockSize <= len(src) {
		block.Encrypt(dest[i:i+aes.BlockSize], src[i:i+aes.BlockSize])
		i += aes.BlockSize
	}
	if i < len(src) {
		var tmp [aes.BlockSize]byte
		copy(tmp[:], src[i:])
		block.Encrypt(dest[i:i+aes.BlockSize], tmp[:])
	}

	return dest, nil
}

// EncryptThenMAC encrypts src, then prepends a 2-byte HMAC-SHA256 MAC.
// Returns: [MAC (2 bytes)] [encrypted data].
func EncryptThenMAC(sharedSecret []byte, src []byte) ([]byte, error) {
	encrypted, err := Encrypt(sharedSecret, src)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, sharedSecret)
	mac.Write(encrypted)
	computed := mac.Sum(nil)

	result := make([]byte, cipherMACSize+len(encrypted))
	copy(result[:cipherMACSize], computed[:cipherMACSize])
	copy(result[cipherMACSize:], encrypted)

	return result, nil
}

// MACThenDecrypt verifies the 2-byte HMAC-SHA256 MAC, then decrypts.
// src must be: [MAC (2 bytes)] [encrypted data].
// Returns nil if the MAC is invalid.
func MACThenDecrypt(sharedSecret []byte, src []byte) ([]byte, error) {
	if len(src) <= cipherMACSize {
		return nil, fmt.Errorf("data too short: %d bytes", len(src))
	}

	mac := hmac.New(sha256.New, sharedSecret)
	mac.Write(src[cipherMACSize:])
	computed := mac.Sum(nil)

	if computed[0] != src[0] || computed[1] != src[1] {
		return nil, nil
	}

	return Decrypt(sharedSecret, src[cipherMACSize:])
}
