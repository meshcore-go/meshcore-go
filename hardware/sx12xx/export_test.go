package sx12xx

import "time"

func withNoiseTimings(interval, round time.Duration) ModemOption {
	return func(c *modemConfig) {
		c.noiseInterval = interval
		c.noiseRound = round
	}
}

func withRecvWatchdog(timeout, poll time.Duration) ModemOption {
	return func(c *modemConfig) {
		c.recvTimeout = timeout
		c.recvPoll = poll
	}
}

func withDropReporting(d time.Duration) ModemOption {
	return func(c *modemConfig) { c.dropInterval = d }
}
