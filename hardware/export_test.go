package hardware

// Flush blocks until all enqueued frames have been dispatched (test-only).
func (m *KissModem) Flush() {
	done := make(chan struct{})
	select {
	case m.flush <- done:
		<-done
	case <-m.done:
	}
}
