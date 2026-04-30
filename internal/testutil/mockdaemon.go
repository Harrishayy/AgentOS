// Package testutil provides an in-process mock of agentd suitable for unit
// tests of the daemon client and CLI subcommands.
//
// The mock listens on a Unix domain socket inside t.TempDir(), reads one or
// more length-prefixed JSON request frames per connection, and emits responses
// via user-supplied handlers. Streaming methods (StreamEvents) keep the
// connection open and emit frames pushed via PushEvent until Stop or
// EndStream is called.
package testutil

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agent-sandbox/runtime/internal/client"
)

// Handler maps a decoded request envelope to a response envelope. Used for
// unary methods only.
type Handler func(req *client.RequestEnvelope) *client.ResponseEnvelope

// StreamHandler is invoked when a StreamEvents subscription opens. The handler
// pushes events on `sink` and returns when the stream should end. Closing
// `sink` is optional — the connection loop closes it after the handler returns.
type StreamHandler func(req *client.StreamEventsRequest, sink chan<- client.Event)

// TB is the subset of testing.TB the mock needs. The standard *testing.T
// satisfies it; e2e harnesses can supply their own implementation.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Cleanup(fn func())
	TempDir() string
}

// MockDaemon is an in-process Unix-socket server for tests.
type MockDaemon struct {
	t        TB
	socket   string
	listener net.Listener

	mu          sync.Mutex
	handlers    map[string]Handler
	streamFn    StreamHandler
	streamSinks map[*net.UnixConn]chan client.Event

	wg sync.WaitGroup

	calls map[string][]json.RawMessage
}

// New starts a MockDaemon on a fresh socket inside t.TempDir() and registers
// t.Cleanup to stop it.
func New(t *testing.T) *MockDaemon {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "agentd.sock")
	return NewWithSocket(t, sock)
}

// NewWithSocket is New but lets the caller pick the socket path. Useful from
// e2e harnesses that need to pre-export the path via env var before the test
// process starts.
func NewWithSocket(t TB, sock string) *MockDaemon {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("mockdaemon: listen: %v", err)
	}
	m := &MockDaemon{
		t:           t,
		socket:      sock,
		listener:    ln,
		handlers:    map[string]Handler{},
		streamSinks: map[*net.UnixConn]chan client.Event{},
		calls:       map[string][]json.RawMessage{},
	}
	m.wg.Add(1)
	go m.acceptLoop()
	t.Cleanup(m.Stop)
	return m
}

// SocketPath returns the absolute Unix-socket path.
func (m *MockDaemon) SocketPath() string { return m.socket }

// On registers a unary handler for `method`.
func (m *MockDaemon) On(method string, h Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[method] = h
}

// OnRunAgent is a typed shortcut.
func (m *MockDaemon) OnRunAgent(fn func(req *client.RunAgentRequest) (*client.RunAgentResult, *client.WireError)) {
	m.On(client.MethodRunAgent, func(env *client.RequestEnvelope) *client.ResponseEnvelope {
		var r client.RunAgentRequest
		_ = json.Unmarshal(env.Params, &r)
		out, werr := fn(&r)
		return wrap(out, werr)
	})
}

// OnList is a typed shortcut.
func (m *MockDaemon) OnList(fn func() (*client.ListAgentsResult, *client.WireError)) {
	m.On(client.MethodListAgents, func(_ *client.RequestEnvelope) *client.ResponseEnvelope {
		out, werr := fn()
		return wrap(out, werr)
	})
}

// OnStop is a typed shortcut.
func (m *MockDaemon) OnStop(fn func(req *client.StopAgentRequest) (*client.StopAgentResult, *client.WireError)) {
	m.On(client.MethodStopAgent, func(env *client.RequestEnvelope) *client.ResponseEnvelope {
		var r client.StopAgentRequest
		_ = json.Unmarshal(env.Params, &r)
		out, werr := fn(&r)
		return wrap(out, werr)
	})
}

// OnLogs is a typed shortcut.
func (m *MockDaemon) OnLogs(fn func(req *client.AgentLogsRequest) (*client.AgentLogsResult, *client.WireError)) {
	m.On(client.MethodAgentLogs, func(env *client.RequestEnvelope) *client.ResponseEnvelope {
		var r client.AgentLogsRequest
		_ = json.Unmarshal(env.Params, &r)
		out, werr := fn(&r)
		return wrap(out, werr)
	})
}

// OnStatus is a typed shortcut.
func (m *MockDaemon) OnStatus(fn func() (*client.DaemonStatusResult, *client.WireError)) {
	m.On(client.MethodDaemonStatus, func(_ *client.RequestEnvelope) *client.ResponseEnvelope {
		out, werr := fn()
		return wrap(out, werr)
	})
}

// OnIngest is a typed shortcut.
func (m *MockDaemon) OnIngest(fn func(req *client.IngestEventRequest) *client.WireError) {
	m.On(client.MethodIngestEvent, func(env *client.RequestEnvelope) *client.ResponseEnvelope {
		var r client.IngestEventRequest
		_ = json.Unmarshal(env.Params, &r)
		werr := fn(&r)
		if werr != nil {
			return &client.ResponseEnvelope{Ok: false, Error: werr}
		}
		raw, _ := json.Marshal(struct{}{})
		return &client.ResponseEnvelope{Ok: true, Result: raw}
	})
}

// OnStreamEvents installs a streaming handler. The handler is invoked with the
// request and a sink channel; pushing events on the channel emits frames to
// the connected client.
func (m *MockDaemon) OnStreamEvents(fn StreamHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamFn = fn
}

// PushEvent emits a single event frame to every active streaming subscriber.
// Returns true if at least one subscriber received the frame.
func (m *MockDaemon) PushEvent(ev client.Event) bool {
	m.mu.Lock()
	sinks := make([]chan client.Event, 0, len(m.streamSinks))
	for _, ch := range m.streamSinks {
		sinks = append(sinks, ch)
	}
	m.mu.Unlock()
	delivered := false
	for _, ch := range sinks {
		select {
		case ch <- ev:
			delivered = true
		case <-time.After(100 * time.Millisecond):
		}
	}
	return delivered
}

// Calls returns captured request payloads for `method`.
func (m *MockDaemon) Calls(method string) []json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]json.RawMessage, len(m.calls[method]))
	copy(out, m.calls[method])
	return out
}

// Stop closes the listener and waits for in-flight goroutines.
func (m *MockDaemon) Stop() {
	_ = m.listener.Close()
	m.mu.Lock()
	conns := make([]*net.UnixConn, 0, len(m.streamSinks))
	for c := range m.streamSinks {
		conns = append(conns, c)
	}
	m.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
	m.wg.Wait()
}

func (m *MockDaemon) acceptLoop() {
	defer m.wg.Done()
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		uc, ok := conn.(*net.UnixConn)
		if !ok {
			_ = conn.Close()
			continue
		}
		m.wg.Add(1)
		go m.handleConn(uc)
	}
}

func (m *MockDaemon) handleConn(conn *net.UnixConn) {
	defer m.wg.Done()
	defer conn.Close()

	body, err := client.ReadFrame(conn)
	if err != nil {
		return
	}
	var env client.RequestEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		_ = writeErr(conn, client.CodeInternal, "decode: "+err.Error())
		return
	}
	m.recordCall(env.Method, env.Params)

	if env.Method == client.MethodStreamEvents {
		m.handleStream(conn, &env)
		return
	}

	m.mu.Lock()
	h := m.handlers[env.Method]
	m.mu.Unlock()
	if h == nil {
		_ = writeErr(conn, client.CodeInternal, fmt.Sprintf("no handler for %q", env.Method))
		return
	}
	resp := h(&env)
	if resp == nil {
		return
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		_ = writeErr(conn, client.CodeInternal, "encode: "+err.Error())
		return
	}
	_ = client.WriteFrame(conn, payload)
}

func (m *MockDaemon) handleStream(conn *net.UnixConn, env *client.RequestEnvelope) {
	m.mu.Lock()
	fn := m.streamFn
	m.mu.Unlock()

	sink := make(chan client.Event, 64)
	m.mu.Lock()
	m.streamSinks[conn] = sink
	m.mu.Unlock()

	// Run handler in a goroutine; it pushes to sink and returns.
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		if fn == nil {
			return
		}
		var r client.StreamEventsRequest
		_ = json.Unmarshal(env.Params, &r)
		fn(&r, sink)
	}()

	// Watcher: detect client-side close. The client never writes more after
	// the initial request, so a Read just blocks until EOF/error.
	clientGone := make(chan struct{})
	go func() {
		defer close(clientGone)
		buf := make([]byte, 1)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case ev, ok := <-sink:
			if !ok {
				goto done
			}
			if !writeStreamFrame(conn, ev) {
				goto done
			}
		case <-handlerDone:
			// Drain remaining buffered events then exit.
			for {
				select {
				case ev := <-sink:
					if !writeStreamFrame(conn, ev) {
						goto done
					}
				default:
					goto done
				}
			}
		case <-clientGone:
			goto done
		}
	}
done:
	m.mu.Lock()
	delete(m.streamSinks, conn)
	m.mu.Unlock()
}

func writeStreamFrame(conn *net.UnixConn, ev client.Event) bool {
	frame := client.StreamEventsFrame{Event: ev}
	raw, _ := json.Marshal(frame)
	resp := &client.ResponseEnvelope{Ok: true, Result: raw}
	payload, _ := json.Marshal(resp)
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return client.WriteFrame(conn, payload) == nil
}

func (m *MockDaemon) recordCall(method string, params json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dup := make(json.RawMessage, len(params))
	copy(dup, params)
	m.calls[method] = append(m.calls[method], dup)
}

func writeErr(w net.Conn, code, msg string) error {
	resp := &client.ResponseEnvelope{Ok: false, Error: &client.WireError{Code: code, Message: msg}}
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return client.WriteFrame(w, payload)
}

func wrap(result any, werr *client.WireError) *client.ResponseEnvelope {
	if werr != nil {
		return &client.ResponseEnvelope{Ok: false, Error: werr}
	}
	if result == nil {
		raw, _ := json.Marshal(struct{}{})
		return &client.ResponseEnvelope{Ok: true, Result: raw}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return &client.ResponseEnvelope{Ok: false, Error: &client.WireError{Code: client.CodeInternal, Message: err.Error()}}
	}
	return &client.ResponseEnvelope{Ok: true, Result: raw}
}
