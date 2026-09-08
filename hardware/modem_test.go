package hardware

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	dead     chan struct{}
	closed   atomic.Bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{dead: make(chan struct{})}
}

func (m *mockTransport) Connect(ctx context.Context) error {
	if m.connectF != nil {
		return m.connectF(ctx)
	}
	return nil
}

func (m *mockTransport) Close() error { m.closed.Store(true); return nil }

func (m *mockTransport) Send(data []byte) error {
	m.mu.Lock()
	m.sent = append(m.sent, data)
	m.mu.Unlock()
	return nil
}

func (m *mockTransport) SetFrameHandler(h func(*KissFrame)) { m.frameH = h }
func (m *mockTransport) SetErrorHandler(h func(error))      { m.errorH = h }
func (m *mockTransport) Dead() <-chan struct{}              { return m.dead }

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
	if received[0].HasSignalInfo {
		t.Error("expected HasSignalInfo=false when signal report disabled")
	}
}

func TestModem_SignalReportEnabled_DataThenMeta(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))

	var received []*KissFrame
	modem.SetFrameHandler(func(f *KissFrame) {
		received = append(received, f)
	})

	mt.injectFrame(makeDataFrame([]byte{0xAA}))
	modem.Flush()
	if len(received) != 0 {
		t.Fatalf("data frame should be queued, got %d dispatched", len(received))
	}

	// SNR byte -6 is quarter-dB on the wire: -1.5 dB.
	mt.injectFrame(makeRxMetaFrame(-6, -80))
	modem.Flush()
	if len(received) != 2 {
		t.Fatalf("expected DATA and RX_META, got %d frames", len(received))
	}
	if received[0].SNR != -1.5 {
		t.Errorf("SNR = %g, want -1.5", received[0].SNR)
	}
	if received[0].RSSI != -80 {
		t.Errorf("RSSI = %d, want -80", received[0].RSSI)
	}
	if !received[0].HasSignalInfo {
		t.Error("expected HasSignalInfo=true after RX_META enrichment")
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

	mt.injectFrame(makeDataFrame([]byte{0x01}))

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
		t.Errorf("stale frame should have zero SNR/RSSI, got SNR=%g RSSI=%d", stale.SNR, stale.RSSI)
	}
	if stale.HasSignalInfo {
		t.Error("expected HasSignalInfo=false for stale-flushed frame")
	}
	if stale.Data[0] != 0x01 {
		t.Errorf("stale frame data = %X, want 01", stale.Data)
	}

	// SNR byte 5 is quarter-dB on the wire: 1.25 dB.
	mt.injectFrame(makeRxMetaFrame(5, -50))
	modem.Flush()

	mu.Lock()
	count = len(received)
	mu.Unlock()
	if count != 3 {
		t.Fatalf("expected 2 DATA frames and RX_META, got %d", count)
	}

	mu.Lock()
	enriched := received[1]
	mu.Unlock()
	if enriched.SNR != 1.25 || enriched.RSSI != -50 {
		t.Errorf("enriched frame SNR=%g RSSI=%d, want 1.25/-50", enriched.SNR, enriched.RSSI)
	}
	if !enriched.HasSignalInfo {
		t.Error("expected HasSignalInfo=true for enriched frame")
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

	mt.injectFrame(makeDataFrame([]byte{0xFF}))

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 0 {
		t.Fatalf("frame should be pending, got %d dispatched", count)
	}

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
		t.Errorf("timeout-flushed frame should have zero SNR/RSSI, got SNR=%g RSSI=%d", flushed.SNR, flushed.RSSI)
	}
	if flushed.HasSignalInfo {
		t.Error("expected HasSignalInfo=false for timeout-flushed frame")
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

	mt.injectFrame(makeRxMetaFrame(-10, -90))
	modem.Flush()

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

func TestModem_ConnectSendsSignalReportOff_WhenDisabled(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt) // no WithSignalReport

	if err := modem.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	sent := mt.sentFrames()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent frame on connect, got %d", len(sent))
	}
	frame, _ := DecodeFrame(sent[0])
	if frame.Data[0] != HW_CMD_SET_SIGNAL_REPORT || frame.Data[1] != 0x00 {
		t.Errorf("frame: subcmd=0x%02X val=0x%02X, want 0x19/0x00", frame.Data[0], frame.Data[1])
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

	frame, _ := DecodeFrame(sent[0])
	if frame.Data[0] != HW_CMD_SET_SIGNAL_REPORT || frame.Data[1] != 0x01 {
		t.Errorf("enable frame: subcmd=0x%02X val=0x%02X, want 0x19/0x01", frame.Data[0], frame.Data[1])
	}

	frame, _ = DecodeFrame(sent[1])
	if frame.Data[0] != HW_CMD_SET_SIGNAL_REPORT || frame.Data[1] != 0x00 {
		t.Errorf("disable frame: subcmd=0x%02X val=0x%02X, want 0x19/0x00", frame.Data[0], frame.Data[1])
	}
}

func TestModem_DataHandler(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt) // signal report disabled

	var dataReceived [][]byte
	modem.SetDataHandler(func(data []byte, _ float32, _ int8, _ bool) {
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

	mt.injectFrame(makeDataFrame([]byte{0xBB}))

	shortMeta := &KissFrame{
		Port:    0,
		Command: KISS_CMD_SETHARDWARE,
		Data:    []byte{HW_RESP_RX_META},
	}
	mt.injectFrame(shortMeta)
	modem.Flush()

	dataFrames := 0
	for _, f := range received {
		if f.Command == KISS_CMD_DATA {
			dataFrames++
			if f.SNR != 0 || f.RSSI != 0 {
				t.Errorf("short meta should not enrich, got SNR=%g RSSI=%d", f.SNR, f.RSSI)
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

	mt.injectFrame(makeDataFrame([]byte{0xCC}))

	modem.Close()

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
	mt.injectFrame(makeRxMetaFrame(10, -40)) // SNR byte 10 (quarter-dB) = 2.5 dB
	modem.Flush()

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 4 {
		t.Fatalf("expected 3 DATA frames and RX_META, got %d", len(received))
	}

	for i := range 2 {
		if received[i].SNR != 0 || received[i].RSSI != 0 {
			t.Errorf("frame %d: expected zero SNR/RSSI, got %g/%d", i, received[i].SNR, received[i].RSSI)
		}
	}
	if received[2].SNR != 2.5 || received[2].RSSI != -40 {
		t.Errorf("frame 2: expected SNR=2.5 RSSI=-40, got %g/%d", received[2].SNR, received[2].RSSI)
	}
}

func TestKissModem_OutboundHandlerCalledBeforeSend(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt, WithTxFlowControl(0))

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
	m := NewKissModem(mt, WithTxFlowControl(0))

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

func TestModem_HandlerWorkers(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithHandlerWorkers(4))
	defer modem.Close()

	var mu sync.Mutex
	var received []*KissFrame
	modem.SetDataHandler(func(data []byte, snr float32, rssi int8, hasSignalInfo bool) {
		mu.Lock()
		received = append(received, &KissFrame{Data: data, SNR: snr, RSSI: rssi})
		mu.Unlock()
	})

	for i := range 20 {
		mt.injectFrame(makeDataFrame([]byte{byte(i)}))
	}
	modem.Flush()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 20 {
		t.Fatalf("expected 20 frames dispatched via worker pool, got %d", count)
	}
}

func TestModem_HandlerWatchdog(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithHandlerWatchdog(10*time.Millisecond))
	defer modem.Close()

	modem.SetFrameHandler(func(f *KissFrame) {
		time.Sleep(50 * time.Millisecond)
	})

	mt.injectFrame(makeDataFrame([]byte{0x01}))
	modem.Flush()

	stats := modem.Stats()
	if stats.HandlerSlow != 1 {
		t.Errorf("expected HandlerSlow=1, got %d", stats.HandlerSlow)
	}
}

func TestModem_Stats_DroppedFrames(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithInboundBuffer(1))

	// Block the drain goroutine by setting a slow handler
	modem.SetFrameHandler(func(f *KissFrame) {
		time.Sleep(100 * time.Millisecond)
	})

	mt.injectFrame(makeDataFrame([]byte{0x01}))
	time.Sleep(10 * time.Millisecond)

	for i := range 5 {
		mt.injectFrame(makeDataFrame([]byte{byte(i + 2)}))
	}

	time.Sleep(200 * time.Millisecond)
	modem.Close()

	stats := modem.Stats()
	if stats.InboundDroppedOldest+stats.InboundDroppedNew == 0 {
		t.Error("expected at least one drop counter to be non-zero")
	}
}

func TestModem_Stats_MetaTimeout(t *testing.T) {
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

	mt.injectFrame(makeDataFrame([]byte{0xAA}))

	time.Sleep(rxMetaTimeout + 200*time.Millisecond)

	stats := modem.Stats()
	if stats.RxMetaTimeouts != 1 {
		t.Errorf("expected RxMetaTimeouts=1, got %d", stats.RxMetaTimeouts)
	}
}

func TestModem_Stats_MetaMisattributed(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithSignalReport(true))
	defer modem.Close()

	modem.SetFrameHandler(func(f *KissFrame) {})

	mt.injectFrame(makeDataFrame([]byte{0x01}))
	mt.injectFrame(makeDataFrame([]byte{0x02}))
	modem.Flush()

	stats := modem.Stats()
	if stats.RxMetaMisattributed != 1 {
		t.Errorf("expected RxMetaMisattributed=1, got %d", stats.RxMetaMisattributed)
	}
}

// sendAndAwait runs the blocking SendData in a goroutine, delivers resps once
// the TX is pending, and returns SendData's result.
func sendAndAwait(t *testing.T, m *KissModem, mt *mockTransport, resps ...*KissFrame) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- m.SendData([]byte{0x01}) }()

	deadline := time.Now().Add(time.Second)
	for len(mt.sentFrames()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("SendData never marked TX pending")
		}
		time.Sleep(time.Millisecond)
	}
	for _, resp := range resps {
		mt.injectFrame(resp)
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("SendData did not return after TX response")
		return nil
	}
}

func TestModem_TxFlowControl_DoneSuccess(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt, WithTxFlowControl(2*time.Second))
	frame := &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_TX_DONE, 0x01}}
	if err := sendAndAwait(t, m, mt, frame); err != nil {
		t.Errorf("SendData on TX_DONE success = %v, want nil", err)
	}
}

func TestModem_TxFlowControl_DoneFailure(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt, WithTxFlowControl(2*time.Second))
	frame := &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_TX_DONE, 0x00}}
	if err := sendAndAwait(t, m, mt, frame); !errors.Is(err, ErrTxFailed) {
		t.Errorf("SendData on TX_DONE failure = %v, want ErrTxFailed", err)
	}
}

func TestModem_TxFlowControl_DoneMissingByte(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt, WithTxFlowControl(2*time.Second))
	frame := &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_TX_DONE}}
	if err := sendAndAwait(t, m, mt, frame); !errors.Is(err, ErrTxFailed) {
		t.Errorf("SendData on TX_DONE without result byte = %v, want ErrTxFailed", err)
	}
}

func TestModem_HwError_TxBusyDoesNotResolveSend(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt, WithTxFlowControl(20*time.Millisecond))
	defer m.Close()
	frame := &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_ERROR, HW_ERR_TX_BUSY}}
	if err := sendAndAwait(t, m, mt, frame); !errors.Is(err, ErrTxTimeout) {
		t.Errorf("SendData on ambiguous HW_ERR_TX_BUSY = %v, want ErrTxTimeout", err)
	}
}

func TestModem_HwError_Reported(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt)
	got := make(chan error, 1)
	m.SetErrorHandler(func(err error) {
		select {
		case got <- err:
		default:
		}
	})
	m.onFrame(&KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_ERROR, HW_ERR_NO_CALLBACK}})

	select {
	case err := <-got:
		if !strings.Contains(err.Error(), "not supported by this board") {
			t.Errorf("error = %v, want it to name the HW_ERR_NO_CALLBACK cause", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hardware error was not reported")
	}
	if n := m.Stats().HwErrors; n != 1 {
		t.Errorf("HwErrors = %d, want 1", n)
	}
}

func TestModem_TxFlowControl_Busy(t *testing.T) {
	mt := newMockTransport()
	m := NewKissModem(mt, WithTxFlowControl(2*time.Second))
	busy := &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_ERROR, HW_ERR_TX_BUSY}}
	done := &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: []byte{HW_RESP_TX_DONE, 0x01}}
	if err := sendAndAwait(t, m, mt, busy, done); err != nil {
		t.Errorf("SendData on TX_BUSY then TX_DONE = %v, want nil", err)
	}
}

func TestModem_SendData_RejectsBadSizes(t *testing.T) {
	mt := newMockTransport()
	modem := NewKissModem(mt, WithTxFlowControl(0))
	defer modem.Close()

	if err := modem.SendData(nil); !errors.Is(err, ErrPacketSize) {
		t.Errorf("SendData(empty) = %v, want ErrPacketSize", err)
	}
	if err := modem.SendData(make([]byte, KISS_MAX_PACKET_SIZE+1)); !errors.Is(err, ErrPacketSize) {
		t.Errorf("SendData(256) = %v, want ErrPacketSize", err)
	}
	if got := len(mt.sentFrames()); got != 0 {
		t.Fatalf("rejected payloads reached the transport: %d frames", got)
	}
	if err := modem.SendData(make([]byte, KISS_MAX_PACKET_SIZE)); err != nil {
		t.Errorf("SendData(255) = %v, want nil", err)
	}
	if got := len(mt.sentFrames()); got != 1 {
		t.Errorf("sent frames = %d, want 1", got)
	}
}

func TestModem_CloseFromHandler(t *testing.T) {
	for _, workers := range []int{0, 2} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			mt := newMockTransport()
			modem := NewKissModem(mt, WithHandlerWorkers(workers))
			returned := make(chan error, 1)
			modem.SetFrameHandler(func(*KissFrame) { returned <- modem.Close() })

			mt.injectFrame(makeDataFrame([]byte{0x01}))
			select {
			case err := <-returned:
				if err != nil {
					t.Fatalf("Close() = %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Close() from handler deadlocked")
			}
			if !mt.closed.Load() {
				t.Error("transport not closed")
			}
		})
	}
}

func makeHwRespFrame(subCmd byte, payload ...byte) *KissFrame {
	return &KissFrame{Port: 0, Command: KISS_CMD_SETHARDWARE, Data: append([]byte{subCmd}, payload...)}
}

// connectedModem returns a started modem plus its transport.
func connectedModem(t *testing.T) (*KissModem, *mockTransport) {
	t.Helper()
	mt := newMockTransport()
	m := NewKissModem(mt)
	if err := m.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m, mt
}

func TestRequest_ReturnsTheMatchingReply(t *testing.T) {
	m, mt := connectedModem(t)

	go func() {
		time.Sleep(20 * time.Millisecond)
		mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_BATTERY), 0x10, 0x0F)) // 3856 mV
	}()

	mv, err := m.Battery(context.Background())
	if err != nil {
		t.Fatalf("Battery: %v", err)
	}
	if mv != 3856 {
		t.Errorf("battery = %d mV, want 3856", mv)
	}
}

// The firmware sends signed tenths of a degree, so a uint16 read would be wrong
// in both scale and sign below zero.
func TestMCUTemp_DecodesSignedTenths(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  int16
		want float32
	}{
		{"positive", 235, 23.5},
		{"below freezing", -55, -5.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mt := connectedModem(t)
			go func() {
				time.Sleep(10 * time.Millisecond)
				mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_MCU_TEMP),
					byte(uint16(tc.raw)), byte(uint16(tc.raw)>>8)))
			}()
			got, err := m.MCUTemp(context.Background())
			if err != nil {
				t.Fatalf("MCUTemp: %v", err)
			}
			if got != tc.want {
				t.Errorf("MCUTemp = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNoiseFloor_DecodesNegativeDBm(t *testing.T) {
	m, mt := connectedModem(t)
	go func() {
		time.Sleep(10 * time.Millisecond)
		dbm := int16(-85)
		raw := uint16(dbm)
		mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_NOISE_FLOOR), byte(raw), byte(raw>>8)))
	}()
	got, err := m.NoiseFloor(context.Background())
	if err != nil {
		t.Fatalf("NoiseFloor: %v", err)
	}
	if got != -85 {
		t.Errorf("NoiseFloor = %d, want -85", got)
	}
}

// A reply that never arrives must fail, not hand back a stale or zero reading:
// silently returning the previous poll's numbers is what callers had to do for
// themselves before, and it cannot be distinguished from a fresh value.
func TestRequest_TimesOutRatherThanReturningStale(t *testing.T) {
	m, _ := connectedModem(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := m.Battery(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Battery with no reply = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v to give up", elapsed)
	}
}

// A late reply belongs to the request that timed out, not to the next one.
func TestRequest_DoesNotAdoptALateReply(t *testing.T) {
	m, mt := connectedModem(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.Battery(ctx); err == nil {
		t.Fatal("first request should have timed out")
	}
	// The abandoned reply arrives now, before the next request is made.
	mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_BATTERY), 0x00, 0x01))

	go func() {
		time.Sleep(20 * time.Millisecond)
		mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_BATTERY), 0x10, 0x0F))
	}()
	mv, err := m.Battery(context.Background())
	if err != nil {
		t.Fatalf("Battery: %v", err)
	}
	if mv != 3856 {
		t.Errorf("battery = %d mV, want 3856 — the second request took the stale reply", mv)
	}
}

func TestRequest_SurfacesHardwareError(t *testing.T) {
	m, mt := connectedModem(t)

	go func() {
		time.Sleep(10 * time.Millisecond)
		mt.injectFrame(makeHwRespFrame(HW_RESP_ERROR, HW_ERR_NO_CALLBACK))
	}()
	_, err := m.MCUTemp(context.Background())
	if !errors.Is(err, ErrHwRequestFailed) {
		t.Fatalf("MCUTemp on a board that cannot read it = %v, want ErrHwRequestFailed", err)
	}
}

func TestFirmwareCounters_DecodesThreeUint32(t *testing.T) {
	m, mt := connectedModem(t)
	go func() {
		time.Sleep(10 * time.Millisecond)
		mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_STATS),
			0x01, 0, 0, 0, 0x02, 0, 0, 0, 0x03, 0, 0, 0))
	}()
	got, err := m.FirmwareCounters(context.Background())
	if err != nil {
		t.Fatalf("FirmwareCounters: %v", err)
	}
	if got != (FirmwareStats{PacketsRecv: 1, PacketsSent: 2, PacketsErrors: 3}) {
		t.Errorf("counters = %+v", got)
	}
}

// The pre-existing OnHwResponse handlers must keep firing alongside Request.
func TestRequest_DoesNotStarveOnHwResponseHandlers(t *testing.T) {
	m, mt := connectedModem(t)

	seen := make(chan []byte, 1)
	m.OnHwResponse(HwResp(HW_CMD_GET_BATTERY), func(_ byte, data []byte) {
		select {
		case seen <- data:
		default:
		}
	})
	go func() {
		time.Sleep(10 * time.Millisecond)
		mt.injectFrame(makeHwRespFrame(HwResp(HW_CMD_GET_BATTERY), 0x10, 0x0F))
	}()
	if _, err := m.Battery(context.Background()); err != nil {
		t.Fatalf("Battery: %v", err)
	}
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Error("registered OnHwResponse handler never fired")
	}
}
