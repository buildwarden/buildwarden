package drivercmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/buildwarden/buildwarden/driver"
)

// The messages of run as the driver writes them (v1.md, Messages).
type (
	startMessage struct {
		Type string `json:"type"`
		driver.Start
	}
	relayStartedMessage struct {
		Type string `json:"type"`
		driver.RelayStarted
	}
	typeMessage struct {
		Type string `json:"type"`
	}
	exitedMessage struct {
		Type   string `json:"type"`
		Which  string `json:"which"`
		Detail string `json:"detail,omitempty"`
	}
	stopMessage struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
		Detail string `json:"detail"`
	}
	stoppedMessage struct {
		Type string   `json:"type"`
		Kept []string `json:"kept,omitempty"`
	}
	failedMessage struct {
		Type  string `json:"type"`
		Error *Error `json:"error"`
	}
)

// msgWriter writes one message per line. Once a write fails it writes no
// more and reports nothing: an orchestrator that died must not stop the
// driver's teardown (DRV-51).
type msgWriter struct {
	mu     sync.Mutex
	w      io.Writer
	broken bool
}

func (m *msgWriter) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.broken {
		return
	}
	if _, err := m.w.Write(append(data, '\n')); err != nil {
		m.broken = true
	}
}

// pipeHost is the kit's driver.Host for a driver binary: the messages of
// run over its standard input and output. The end of its input cancels the
// build's context and returns Wait (DRV-50).
type pipeHost struct {
	out     *msgWriter
	cancel  context.CancelFunc
	claimed chan struct{}
	stopped chan struct{}
	ended   chan struct{}
	once    sync.Once
	reason  string
	detail  string
}

func newPipeHost(out *msgWriter, cancel context.CancelFunc) *pipeHost {
	return &pipeHost{out: out, cancel: cancel, claimed: make(chan struct{}),
		stopped: make(chan struct{}), ended: make(chan struct{})}
}

// read takes the orchestrator's messages until its input ends. A message of
// a type it does not know is ignored (DRV-20).
func (h *pipeHost) read(in *bufio.Reader) {
	defer func() {
		close(h.ended)
		h.cancel()
	}()
	var claimedOnce sync.Once
	for {
		line, err := readLine(in)
		if err != nil {
			return
		}
		var m stopMessage
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		switch m.Type {
		case "claimed":
			claimedOnce.Do(func() { close(h.claimed) })
		case "stop":
			h.once.Do(func() {
				h.reason, h.detail = m.Reason, m.Detail
				close(h.stopped)
			})
		}
	}
}

// ClaimRelay implements driver.Host.
func (h *pipeHost) ClaimRelay(ctx context.Context, r driver.RelayStarted) error {
	h.out.send(relayStartedMessage{Type: "relay-started", RelayStarted: r})
	select {
	case <-h.claimed:
		return nil
	case <-h.stopped:
		return fmt.Errorf("the build was stopped before the relay was claimed (%s): %s",
			h.reason, h.detail)
	case <-h.ended:
		return fmt.Errorf("the orchestrator's input ended before the relay was claimed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Started implements driver.Host.
func (h *pipeHost) Started() error {
	h.out.send(typeMessage{Type: "environment-started"})
	return nil
}

// Exited implements driver.Host.
func (h *pipeHost) Exited(which driver.ExitedPart, detail string) {
	h.out.send(exitedMessage{Type: "exited", Which: string(which), Detail: detail})
}

// Wait implements driver.Host: the stop's reason, or interrupted when the
// input ended or ctx did.
func (h *pipeHost) Wait(ctx context.Context) string {
	select {
	case <-h.stopped:
		return h.reason
	case <-h.ended:
		return driver.StopInterrupted
	case <-ctx.Done():
		return driver.StopInterrupted
	}
}
