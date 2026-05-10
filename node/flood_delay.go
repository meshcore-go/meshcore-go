package node

import (
	"math/rand/v2"
	"time"
)

func FloodRetransmitDelay(estAirtimeMs uint32) time.Duration {
	t := float64(estAirtimeMs) * 52.0 / 50.0 / 2.0
	n := rand.IntN(6)
	return time.Duration(float64(n)*t) * time.Millisecond
}
