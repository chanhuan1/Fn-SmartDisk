package collector

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestOfflineDiskStreamOverIPC(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	_ = clientConn.SetDeadline(time.Now().Add(5 * time.Second))
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		handleConn(c, serverConn)
	}()
	encodeDone := make(chan error, 1)
	go func() {
		encodeDone <- json.NewEncoder(clientConn).Encode(request{
			Op: "offline", ID: "sda", Stream: true,
			ConfirmRaid: false, ConfirmBusyIO: false,
		})
	}()

	var steps []string
	result, err := readStream(json.NewDecoder(clientConn), func(step, detail string) {
		steps = append(steps, step+":"+detail)
	})
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if err := <-encodeDone; err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if result == nil || !result.OK || result.Power == nil || !result.Power.OK {
		t.Fatalf("bad result: %+v", result)
	}
	if len(steps) < 3 || steps[0][:4] != "sync" || steps[len(steps)-1][:7] != "offline" {
		t.Fatalf("unexpected streamed steps: %v", steps)
	}
	_ = clientConn.Close()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("IPC handler did not finish")
	}
}
