package node

import (
	"sync"

	meshcore "github.com/meshcore-go/meshcore-go"
)

type secretCache struct {
	mu      sync.RWMutex
	self    meshcore.LocalIdentity
	secrets map[[meshcore.PubKeySize]byte][]byte
}

func newSecretCache(self meshcore.LocalIdentity) *secretCache {
	return &secretCache{
		self:    self,
		secrets: make(map[[meshcore.PubKeySize]byte][]byte),
	}
}

func (sc *secretCache) reset(self meshcore.LocalIdentity) {
	sc.mu.Lock()
	sc.self = self
	sc.secrets = make(map[[meshcore.PubKeySize]byte][]byte)
	sc.mu.Unlock()
}

func (sc *secretCache) get(peer meshcore.Identity) ([]byte, error) {
	key := peer.PublicKey()

	sc.mu.RLock()
	if s, ok := sc.secrets[key]; ok {
		sc.mu.RUnlock()
		out := make([]byte, len(s))
		copy(out, s)
		return out, nil
	}
	sc.mu.RUnlock()

	secret, err := sc.self.SharedSecret(peer)
	if err != nil {
		return nil, err
	}

	sc.mu.Lock()
	sc.secrets[key] = secret
	sc.mu.Unlock()

	out := make([]byte, len(secret))
	copy(out, secret)
	return out, nil
}
