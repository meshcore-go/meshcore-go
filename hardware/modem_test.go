package hardware

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockTransport is a minimal Transport for testing KissModem dispatch logic.
type mockTransport struct {
	mu       sync.Mutex
	sent     [][]byte
	frameH   func(*KissFrame)
	errorH   func(error)
	connectF func(ctx context.Context) error
}

func newMockTransport() *mockTransport {
	return &mockTransport{}
}

func (m *mockTransport) Connect(ctx context.Context) error {
	if m.connectF != nil {
		return m.connectF(ctx)
	}
	return nil
}

func (m *mockTransport) Close() error { return nil }

func (m *mockTransport) Send(data []byte) error {
	m.mu.Lock()
	m.sent = append(m.sent, data)
	m.mu.Unlock()
	return nil
}

func (m *mockTransport) SetFrameHandler(h func(*KissFrame)) { m.frameH = h }
func (m *mockTransport) SetErrorHandler(h func(error))      { m.errorH = h }

// sentFrames returns copies of all raw bytes sent through the transport.
func (m *mockTransport) sentFrames() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.sent))
	copy(out, m.sent)
	return out
}

// injectFrame simulates the transport delivering a decoded KISS frame.
func (m *mockTransport) injectFrame(f *KissFrame) {
	if m.frameH != nil {
		m.frameH(f)
	}
}

// --- helpers ---

func makeDataFrame(data []byte) *KissFrame {
	return &KissFrame{Port: 0, Command: KISS_CMD_DATA, Data: data}
}

func makeRxMetaFrame(snr, rssi int8) *KissFrame {
	return &KissFrame{
		Port:    0,
		Command: KISS_CMD_SETHARDWARE,
		Data:    []byte{HW_RESP_RX_META, byte(snr), byte(rssi)},
	}
}

// --- Tests ---

func TestModem_SignalReportDisabled_ImmediateDispatch(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt) // no WithSignalReport

	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		received = append(received, f)
	})

	mt.injectFrame(makeDataFrame([]byte{0x01}))
	mt.injectFrame(makeDataFrame([]byte{0x02}))
	modem.Flush()

	if len(received) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(received))
	}
	if received[0].SNR != 0 || received[0].RSSI != 0 {
		t.Error("expected zero SNR/RSSI when signal report disabled")
	}
}

func TestModem_SignalReportEnabled_DataThenMeta(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		received = append(received, f)
	})

	// Inject a data frame — should be queued, not dispatched.
	mt.injectFrame(makeDataFrame([]byte{0xAA}))
	modem.Flush()
	if len(received) != 0 {
		t.Fatalf("data frame should be queued, got %d dispatched", len(received))
	}

	// Inject RX_META — should enrich and dispatch the queued frame.
	mt.injectFrame(makeRxMetaFrame(-6, -80))
	modem.Flush()
	if len(received) != 1 {
		t.Fatalf("expected 1 dispatched frame after meta, got %d", len(received))
	}
	if received[0].SNR != -6 {
		t.Errorf("SNR = %d, want -6", received[0].SNR)
	}
	if received[0].RSSI != -80 {
		t.Errorf("RSSI = %d, want -80", received[0].RSSI)
	}
	if len(received[0].Data) != 1 || received[0].Data[0] != 0xAA {
		t.Errorf("data = %X, want AA", received[0].Data)
	}
}

func TestModem_SignalReportEnabled_StaleFlush(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var mu sync.Mutex
	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	})

	// Inject first data frame — queued.
	mt.injectFrame(makeDataFrame([]byte{0x01}))

	// Inject second data frame — first should flush with zero SNR/RSSI.
	mt.injectFrame(makeDataFrame([]byte{0x02}))
	modem.Flush()

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 stale-flushed frame, got %d", count)
	}

	mu.Lock()
	stale := received[0]
	mu.Unlock()
	if stale.SNR != 0 || stale.RSSI != 0 {
		t.Errorf("stale frame should have zero SNR/RSSI, got SNR=%d RSSI=%d", stale.SNR, stale.RSSI)
	}
	if stale.Data[0] != 0x01 {
		t.Errorf("stale frame data = %X, want 01", stale.Data)
	}

	// Now deliver meta for the second frame.
	mt.injectFrame(makeRxMetaFrame(5, -50))
	modem.Flush()

	mu.Lock()
	count = len(received)
	mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 total frames, got %d", count)
	}

	mu.Lock()
	enriched := received[1]
	mu.Unlock()
	if enriched.SNR != 5 || enriched.RSSI != -50 {
		t.Errorf("enriched frame SNR=%d RSSI=%d, want 5/-50", enriched.SNR, enriched.RSSI)
	}
	if enriched.Data[0] != 0x02 {
		t.Errorf("enriched frame data = %X, want 02", enriched.Data)
	}
}

func TestModem_SignalReportEnabled_Timeout(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))
	defer modem.Close()

	var mu sync.Mutex
	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	})

	// Inject data frame — queued.
	mt.injectFrame(makeDataFrame([]byte{0xFF}))

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 0 {
		t.Fatalf("frame should be pending, got %d dispatched", count)
	}

	// Wait for the timeout (1s) + margin.
	time.Sleep(rxMetaTimeout + 200*time.Millisecond)

	mu.Lock()
	count = len(received)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 timeout-flushed frame, got %d", count)
	}

	mu.Lock()
	flushed := received[0]
	mu.Unlock()
	if flushed.SNR != 0 || flushed.RSSI != 0 {
		t.Errorf("timeout-flushed frame should have zero SNR/RSSI, got SNR=%d RSSI=%d", flushed.SNR, flushed.RSSI)
	}
	if flushed.Data[0] != 0xFF {
		t.Errorf("data = %X, want FF", flushed.Data)
	}
}

func TestModem_SignalReportEnabled_MetaWithoutPending(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		received = append(received, f)
	})

	// Inject RX_META with no pending data frame — should not crash,
	// and the HW frame should still be dispatched to frame handler.
	mt.injectFrame(makeRxMetaFrame(-10, -90))

	// The HW frame itself goes through dispatchFrame → frameHandler.
	// But no data frame should be dispatched with enrichment.
	for _, f := range received {
		if f.Command == KISS_CMD_DATA {
			t.Error("no data frame should be dispatched when meta arrives without pending")
		}
	}
}

func TestModem_SignalReportEnabled_NonDataNonMetaImmediate(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		received = append(received, f)
	})

	// Non-data, non-RX_META HW frame should dispatch immediately even with signal report on.
	hwFrame := &KissFrame{
		Port:    0,
		Command: KISS_CMD_SETHARDWARE,
		Data:    []byte{HW_RESP_TX_DONE, 0x01},
	}
	mt.injectFrame(hwFrame)
	modem.Flush()

	if len(received) != 1 {
		t.Fatalf("expected 1 immediate frame, got %d", len(received))
	}
	if received[0].Command != KISS_CMD_SETHARDWARE {
		t.Errorf("command = 0x%02X, want SETHARDWARE", received[0].Command)
	}
}

func TestModem_ConnectSendsSignalReport(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	ctx := context.Background()
	if err := modem.Connect(ctx); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	sent := mt.sentFrames()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent frame on connect, got %d", len(sent))
	}

	// Decode the sent frame and verify it's SET_SIGNAL_REPORT with 0x01.
	frame, err := DecodeFrame(sent[0])
	if err != nil {
		t.Fatalf("DecodeFrame error: %v", err)
	}
	if frame.Command != KISS_CMD_SETHARDWARE {
		t.Errorf("command = 0x%02X, want SETHARDWARE", frame.Command)
	}
	if len(frame.Data) < 2 {
		t.Fatalf("frame data too short: %X", frame.Data)
	}
	if frame.Data[0] != HW_CMD_SET_SIGNAL_REPORT {
		t.Errorf("sub-command = 0x%02X, want SET_SIGNAL_REPORT (0x%02X)", frame.Data[0], HW_CMD_SET_SIGNAL_REPORT)
	}
	if frame.Data[1] != 0x01 {
		t.Errorf("signal report value = 0x%02X, want 0x01 (enabled)", frame.Data[1])
	}
}

func TestModem_ConnectDoesNotSendSignalReport_WhenDisabled(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt) // no WithSignalReport

	ctx := context.Background()
	if err := modem.Connect(ctx); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	sent := mt.sentFrames()
	if len(sent) != 0 {
		t.Fatalf("expected no sent frames on connect when signal report disabled, got %d", len(sent))
	}
}

func TestModem_SetSignalReport(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt)

	if err := modem.SetSignalReport(true); err != nil {
		t.Fatalf("SetSignalReport(true) error: %v", err)
	}
	if err := modem.SetSignalReport(false); err != nil {
		t.Fatalf("SetSignalReport(false) error: %v", err)
	}

	sent := mt.sentFrames()
	if len(sent) != 2 {
		t.Fatalf("expected 2 sent frames, got %d", len(sent))
	}

	// First: enable (0x01)
	frame, _ := DecodeFrame(sent[0])
	if frame.Data[0] != HW_CMD_SET_SIGNAL_REPORT || frame.Data[1] != 0x01 {
		t.Errorf("enable frame: subcmd=0x%02X val=0x%02X, want 0x19/0x01", frame.Data[0], frame.Data[1])
	}

	// Second: disable (0x00)
	frame, _ = DecodeFrame(sent[1])
	if frame.Data[0] != HW_CMD_SET_SIGNAL_REPORT || frame.Data[1] != 0x00 {
		t.Errorf("disable frame: subcmd=0x%02X val=0x%02X, want 0x19/0x00", frame.Data[0], frame.Data[1])
	}
}

func TestModem_DataHandler(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt) // signal report disabled

	var dataReceived [][]byte
	modem.SetDataHandler(func(data []byte, _ int8, _ int8) {
		dataReceived = append(dataReceived, data)
	})

	mt.injectFrame(makeDataFrame([]byte{0xDE, 0xAD}))
	modem.Flush()

	if len(dataReceived) != 1 {
		t.Fatalf("expected 1 data callback, got %d", len(dataReceived))
	}
	if dataReceived[0][0] != 0xDE || dataReceived[0][1] != 0xAD {
		t.Errorf("data = %X, want DEAD", dataReceived[0])
	}
}

func TestModem_HwResponseHandler(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt)

	var hwCalls []byte
	modem.OnHwResponse(HwResp(HW_CMD_PING), func(subCmd byte, data []byte) {
		hwCalls = append(hwCalls, subCmd)
	})

	// Inject a PING response HW frame.
	pingResp := &KissFrame{
		Port:    0,
		Command: KISS_CMD_SETHARDWARE,
		Data:    []byte{HwResp(HW_CMD_PING), 0x01},
	}
	mt.injectFrame(pingResp)
	modem.Flush()

	if len(hwCalls) != 1 {
		t.Fatalf("expected 1 hw callback, got %d", len(hwCalls))
	}
	if hwCalls[0] != HwResp(HW_CMD_PING) {
		t.Errorf("sub-command = 0x%02X, want 0x%02X", hwCalls[0], HwResp(HW_CMD_PING))
	}
}

func TestModem_SignalReportEnabled_RxMetaAlsoFiresHwHandler(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var hwCalls int
	modem.OnHwResponse(HW_RESP_RX_META, func(subCmd byte, data []byte) {
		hwCalls++
	})

	// Queue a data frame, then deliver meta.
	mt.injectFrame(makeDataFrame([]byte{0x01}))
	mt.injectFrame(makeRxMetaFrame(-3, -70))
	modem.Flush()

	if hwCalls != 1 {
		t.Errorf("expected RX_META hw handler called once, got %d", hwCalls)
	}
}

func TestModem_SignalReportEnabled_MetaShortPayload(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		received = append(received, f)
	})

	// Queue data frame.
	mt.injectFrame(makeDataFrame([]byte{0xBB}))

	// Inject RX_META with only 1 byte of data (missing RSSI) — should still
	// dispatch the pending frame but without enrichment.
	shortMeta := &KissFrame{
		Port:    0,
		Command: KISS_CMD_SETHARDWARE,
		Data:    []byte{HW_RESP_RX_META}, // only sub-command, no SNR/RSSI
	}
	mt.injectFrame(shortMeta)
	modem.Flush()

	// The pending frame should be dispatched (without enrichment).
	dataFrames := 0
	for _, f := range received {
		if f.Command == KISS_CMD_DATA {
			dataFrames++
			if f.SNR != 0 || f.RSSI != 0 {
				t.Errorf("short meta should not enrich, got SNR=%d RSSI=%d", f.SNR, f.RSSI)
			}
		}
	}
	if dataFrames != 1 {
		t.Errorf("expected 1 data frame dispatched, got %d", dataFrames)
	}
}

func TestModem_Close_CancelsPendingTimer(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var mu sync.Mutex
	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	})

	// Queue a data frame (starts timer).
	mt.injectFrame(makeDataFrame([]byte{0xCC}))

	// Close before timeout fires.
	modem.Close()

	// Wait past the timeout to verify the timer was cancelled.
	time.Sleep(rxMetaTimeout + 200*time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 0 {
		t.Errorf("expected no dispatched frames after Close, got %d", count)
	}
}

func TestModem_ErrorHandler(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt)

	var errReceived error
	modem.SetErrorHandler(func(err error) {
		errReceived = err
	})

	// Inject a malformed HW frame (SETHARDWARE with empty data).
	bad := &KissFrame{
		Port:    0,
		Command: KISS_CMD_SETHARDWARE,
		Data:    []byte{},
	}
	mt.injectFrame(bad)
	modem.Flush()

	if errReceived == nil {
		t.Error("expected error for malformed HW frame")
	}
}

func TestModem_MultipleDataThenMeta(t *testing.T) {
	// Simulates rapid-fire: data1, data2, data3, meta.
	// data1 flushed (stale when data2 arrives), data2 flushed (stale when data3 arrives),
	// data3 enriched by meta.
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var mu sync.Mutex
	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		mu.Lock()
		received = append(received, f)
		mu.Unlock()
	})

	mt.injectFrame(makeDataFrame([]byte{0x01}))
	mt.injectFrame(makeDataFrame([]byte{0x02}))
	mt.injectFrame(makeDataFrame([]byte{0x03}))
	mt.injectFrame(makeRxMetaFrame(10, -40))
	modem.Flush()

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(received))
	}

	// First two: stale, zero SNR/RSSI
	for i := range 2 {
		if received[i].SNR != 0 || received[i].RSSI != 0 {
			t.Errorf("frame %d: expected zero SNR/RSSI, got %d/%d", i, received[i].SNR, received[i].RSSI)
		}
	}
	// Third: enriched
	if received[2].SNR != 10 || received[2].RSSI != -40 {
		t.Errorf("frame 2: expected SNR=10 RSSI=-40, got %d/%d", received[2].SNR, received[2].RSSI)
	}
}

func TestKissModem_OutboundHandlerCalledBeforeSend(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt)

	var captured []byte
	m.AddOutboundHandler(func(data []byte) {
		captured = append([]byte{}, data...)
	})

	payload := []byte{0xDE, 0xAD}
	if err := m.SendData(payload); err != nil {
		t.Fatalf("SendData error: %v", err)
	}

	if len(captured) != 2 || captured[0] != 0xDE || captured[1] != 0xAD {
		t.Errorf("outbound handler got %X, want DEAD", captured)
	}

	sent := mt.sentFrames()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent frame, got %d", len(sent))
	}
}

func TestKissModem_MultipleOutboundHandlers(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt)

	var count int
	m.AddOutboundHandler(func([]byte) { count++ })
	m.AddOutboundHandler(func([]byte) { count++ })

	if err := m.SendData([]byte{0x01}); err != nil {
		t.Fatalf("SendData error: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 handler calls, got %d", count)
	}
}
