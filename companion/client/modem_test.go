package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/meshcore-go/meshcore-go/companion"
)

func TestCompanionModem_SendData(t *testing.T) {
	mt := &mockTransport{}
	mt.onSend = func(cmd []byte) {
		go mt.fireResponse(companion.Response{
			Code: companion.RespOk,
			Data: companion.OkResponse{},
		})
	}

	c := New(mt)
	m := NewCompanionModem(context.Background(), c)
	defer m.Close()

	payload := []byte{0x01, 0x02, 0x03}
	if err := m.SendData(payload); err != nil {
		t.Fatalf("SendData() error: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.sent) != 1 {
		t.Fatalf("sent %d commands, want 1", len(mt.sent))
	}

	cmd := mt.sent[0]
	if cmd[0] != companion.CmdSendRawData {
		t.Errorf("command byte = 0x%02x, want 0x%02x", cmd[0], companion.CmdSendRawData)
	}
	if cmd[1] != 0x00 {
		t.Errorf("path length = %d, want 0", cmd[1])
	}
	got := cmd[2:]
	if len(got) != len(payload) {
		t.Fatalf("raw data length = %d, want %d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Errorf("raw data[%d] = 0x%02x, want 0x%02x", i, got[i], payload[i])
		}
	}
}

func TestCompanionModem_ReceivePush(t *testing.T) {
	mt := &mockTransport{}
	c := New(mt)
	m := NewCompanionModem(context.Background(), c)
	defer m.Close()

	var mu sync.Mutex
	var gotData []byte
	var gotSNR int8
	var gotRSSI int8

	m.SetDataHandler(func(data []byte, snr int8, rssi int8) {
		mu.Lock()
		gotData = data
		gotSNR = snr
		gotRSSI = rssi
		mu.Unlock()
	})

	mt.fireResponse(companion.Response{
		Code: companion.PushRawData,
		Data: companion.PushRawDataResponse{
			LastSNR:  -5,
			LastRSSI: -42,
			Payload:  []byte{0xAA, 0xBB, 0xCC},
		},
	})

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if gotData == nil {
		t.Fatal("handler not called")
	}
	if len(gotData) != 3 || gotData[0] != 0xAA || gotData[1] != 0xBB || gotData[2] != 0xCC {
		t.Errorf("data = %x, want aabbcc", gotData)
	}
	if gotSNR != -5 {
		t.Errorf("SNR = %d, want -5", gotSNR)
	}
	if gotRSSI != -42 {
		t.Errorf("RSSI = %d, want -42", gotRSSI)
	}
}

func TestCompanionModem_NoHandlerNoPanic(t *testing.T) {
	mt := &mockTransport{}
	c := New(mt)
	m := NewCompanionModem(context.Background(), c)
	defer m.Close()

	mt.fireResponse(companion.Response{
		Code: companion.PushRawData,
		Data: companion.PushRawDataResponse{
			LastSNR:  0,
			LastRSSI: 0,
			Payload:  []byte{0x01},
		},
	})
}

func TestCompanionModem_CloseUnblocksSend(t *testing.T) {
	mt := &mockTransport{}
	c := New(mt)
	m := NewCompanionModem(context.Background(), c)

	done := make(chan error, 1)
	go func() {
		done <- m.SendData([]byte{0x01})
	}()

	time.Sleep(10 * time.Millisecond)
	m.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after Close, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendData did not unblock after Close")
	}
}

func TestCompanionModem_SendError(t *testing.T) {
	mt := &mockTransport{sendErr: errMock}
	c := New(mt)
	m := NewCompanionModem(context.Background(), c)
	defer m.Close()

	if err := m.SendData([]byte{0x01}); err == nil {
		t.Fatal("expected error from SendData")
	}
}

var errMock = errors.New("mock send error")
