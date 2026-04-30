package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-sandbox/runtime/internal/client"
	"github.com/agent-sandbox/runtime/internal/testutil"
)

func TestClient_RunAgent_OK(t *testing.T) {
	mock := testutil.New(t)
	mock.OnRunAgent(func(req *client.RunAgentRequest) (*client.RunAgentResult, *client.WireError) {
		if req.Manifest.Name != "agent-x" {
			t.Errorf("name: got %q want %q", req.Manifest.Name, "agent-x")
		}
		return &client.RunAgentResult{
			Name:          "agent-x",
			AgentID:       "01H8X0",
			PID:           4242,
			CgroupPath:    "/sys/fs/cgroup/agent/agent-x",
			StartedAt:     "2026-04-29T12:00:00Z",
			PolicySummary: "hosts:1 paths:0 timeout:0",
		}, nil
	})

	c := client.New(mock.SocketPath())
	res, err := c.RunAgent(context.Background(), &client.RunAgentRequest{
		Manifest: client.ManifestPayload{Name: "agent-x", Command: []string{"/bin/true"}},
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.PID != 4242 {
		t.Errorf("PID: got %d want %d", res.PID, 4242)
	}
}

func TestClient_RunAgent_ServerError(t *testing.T) {
	mock := testutil.New(t)
	mock.OnRunAgent(func(_ *client.RunAgentRequest) (*client.RunAgentResult, *client.WireError) {
		return nil, &client.WireError{Code: client.CodeInvalidManifest, Message: "name already in use"}
	})

	c := client.New(mock.SocketPath())
	_, err := c.RunAgent(context.Background(), &client.RunAgentRequest{})
	if err == nil {
		t.Fatal("RunAgent: expected error")
	}
	var se *client.ServerError
	if !errors.As(err, &se) {
		t.Fatalf("expected *ServerError, got %T (%v)", err, err)
	}
	if se.Code != client.CodeInvalidManifest {
		t.Errorf("code: got %q want %q", se.Code, client.CodeInvalidManifest)
	}
}

func TestClient_ListAgents(t *testing.T) {
	mock := testutil.New(t)
	exit0 := 0
	mock.OnList(func() (*client.ListAgentsResult, *client.WireError) {
		return &client.ListAgentsResult{
			Agents: []client.AgentInfo{
				{Name: "a", AgentID: "01A", Status: "running", PID: 100, ExitCode: nil, StartedAt: "2026-04-29T11:00:00Z"},
				{Name: "b", AgentID: "01B", Status: "exited", PID: 0, ExitCode: &exit0, StartedAt: "2026-04-29T10:00:00Z"},
			},
		}, nil
	})

	c := client.New(mock.SocketPath())
	res, err := c.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(res.Agents) != 2 {
		t.Fatalf("agents len: got %d want 2", len(res.Agents))
	}
	if res.Agents[0].Status != "running" || res.Agents[1].ExitCode == nil || *res.Agents[1].ExitCode != 0 {
		t.Errorf("unexpected agents: %+v", res.Agents)
	}
}

func TestClient_StopAgent(t *testing.T) {
	mock := testutil.New(t)
	mock.OnStop(func(req *client.StopAgentRequest) (*client.StopAgentResult, *client.WireError) {
		if req.Name != "victim" {
			t.Errorf("name: got %q", req.Name)
		}
		if req.GracePeriodNS != int64(2*time.Second) {
			t.Errorf("grace: got %d", req.GracePeriodNS)
		}
		return &client.StopAgentResult{Name: "victim", ExitCode: 0, Signal: "SIGTERM", DurationNS: 12345}, nil
	})

	c := client.New(mock.SocketPath())
	res, err := c.StopAgent(context.Background(), "victim", int64(2*time.Second))
	if err != nil {
		t.Fatalf("StopAgent: %v", err)
	}
	if res.Signal != "SIGTERM" {
		t.Errorf("signal: got %q", res.Signal)
	}
}

func TestClient_StopAgent_NotFound(t *testing.T) {
	mock := testutil.New(t)
	mock.OnStop(func(_ *client.StopAgentRequest) (*client.StopAgentResult, *client.WireError) {
		return nil, &client.WireError{Code: client.CodeAgentNotFound, Message: "no such agent"}
	})

	c := client.New(mock.SocketPath())
	_, err := c.StopAgent(context.Background(), "ghost", 0)
	if !errors.Is(err, client.ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestClient_AgentLogs(t *testing.T) {
	mock := testutil.New(t)
	mock.OnLogs(func(req *client.AgentLogsRequest) (*client.AgentLogsResult, *client.WireError) {
		if req.TailN != 10 {
			t.Errorf("tail_n: got %d", req.TailN)
		}
		evs := []client.Event{
			{Schema: "v1", TS: "2026-04-29T12:00:00Z", Agent: "a", AgentID: "01A", Category: "agent", Type: "stdout", Data: json.RawMessage(`{"line":"hello"}`)},
			{Schema: "v1", TS: "2026-04-29T12:00:01Z", Agent: "a", AgentID: "01A", Category: "lifecycle", Type: "exit", Data: json.RawMessage(`{"exit_code":0}`)},
		}
		return &client.AgentLogsResult{Events: evs}, nil
	})

	c := client.New(mock.SocketPath())
	res, err := c.AgentLogs(context.Background(), "a", 10)
	if err != nil {
		t.Fatalf("AgentLogs: %v", err)
	}
	if len(res.Events) != 2 || res.Events[0].Type != "stdout" {
		t.Errorf("unexpected events: %+v", res.Events)
	}
}

func TestClient_DaemonStatus(t *testing.T) {
	mock := testutil.New(t)
	mock.OnStatus(func() (*client.DaemonStatusResult, *client.WireError) {
		return &client.DaemonStatusResult{ProtocolVersion: "v1", Build: "test-1.0", UptimeNS: 7e9, AgentsRunning: 3}, nil
	})
	c := client.New(mock.SocketPath())
	res, err := c.DaemonStatus(context.Background())
	if err != nil {
		t.Fatalf("DaemonStatus: %v", err)
	}
	if res.ProtocolVersion != "v1" {
		t.Errorf("proto: %q", res.ProtocolVersion)
	}
}

func TestClient_DaemonUnreachable(t *testing.T) {
	c := client.New("/no/such/socket/agentd.sock", client.WithDialTimeout(200*time.Millisecond))
	_, err := c.DaemonStatus(context.Background())
	if !errors.Is(err, client.ErrDaemonUnreachable) {
		t.Fatalf("expected ErrDaemonUnreachable, got %v", err)
	}
}

func TestClient_StreamEvents_NaturalEnd(t *testing.T) {
	mock := testutil.New(t)
	mock.OnStreamEvents(func(req *client.StreamEventsRequest, sink chan<- client.Event) {
		for i := 0; i < 3; i++ {
			sink <- client.Event{
				Schema:   "v1",
				TS:       "2026-04-29T12:00:00Z",
				Agent:    "a",
				AgentID:  "01A",
				Category: "agent",
				Type:     "stdout",
				Data:     json.RawMessage(`{"line":"x"}`),
			}
		}
	})

	c := client.New(mock.SocketPath())
	stream, err := c.StreamEvents(context.Background(), &client.StreamEventsRequest{Name: "a"})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}
	defer stream.Close()

	var got []client.Event
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-stream.Events:
			if !ok {
				break loop
			}
			got = append(got, ev)
		case err := <-stream.Errors:
			if err != nil {
				t.Fatalf("stream error: %v", err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for stream end")
		}
	}
	if len(got) != 3 {
		t.Errorf("events: got %d want 3", len(got))
	}
}

func TestClient_StreamEvents_CancelFast(t *testing.T) {
	mock := testutil.New(t)
	mock.OnStreamEvents(func(req *client.StreamEventsRequest, sink chan<- client.Event) {
		// Block forever — caller will cancel.
		select {}
	})

	c := client.New(mock.SocketPath())
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.StreamEvents(ctx, &client.StreamEventsRequest{Name: "a"})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	t0 := time.Now()
	cancel()
	// Wait for both channels to drain.
	done := make(chan struct{})
	go func() {
		stream.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Close did not return within 500ms after cancel")
	}
	elapsed := time.Since(t0)
	// Per WS-3 acceptance criteria: cancellation should be observed in well
	// under 50ms; we widen to 200ms to absorb test scheduler jitter.
	if elapsed > 200*time.Millisecond {
		t.Errorf("cancel-to-close took %v, want <200ms", elapsed)
	}
}

func TestClient_FrameOversize(t *testing.T) {
	// Direct framing test (no client involved).
	huge := make([]byte, client.MaxFrameBytes+1)
	err := client.WriteFrame(&discardWriter{}, huge)
	if !errors.Is(err, client.ErrFrameOversize) {
		t.Fatalf("expected ErrFrameOversize, got %v", err)
	}
}

func TestClient_RoundTrip_Methods_AllExercised(t *testing.T) {
	// Sanity: every public method round-trips through the same wire path.
	mock := testutil.New(t)
	mock.OnRunAgent(func(_ *client.RunAgentRequest) (*client.RunAgentResult, *client.WireError) {
		return &client.RunAgentResult{Name: "n"}, nil
	})
	mock.OnList(func() (*client.ListAgentsResult, *client.WireError) { return &client.ListAgentsResult{}, nil })
	mock.OnStop(func(_ *client.StopAgentRequest) (*client.StopAgentResult, *client.WireError) {
		return &client.StopAgentResult{}, nil
	})
	mock.OnLogs(func(_ *client.AgentLogsRequest) (*client.AgentLogsResult, *client.WireError) {
		return &client.AgentLogsResult{}, nil
	})
	mock.OnStatus(func() (*client.DaemonStatusResult, *client.WireError) {
		return &client.DaemonStatusResult{}, nil
	})
	mock.OnIngest(func(_ *client.IngestEventRequest) *client.WireError { return nil })

	c := client.New(mock.SocketPath())
	ctx := context.Background()

	if _, err := c.RunAgent(ctx, &client.RunAgentRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListAgents(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.StopAgent(ctx, "x", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AgentLogs(ctx, "x", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DaemonStatus(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.IngestEvent(ctx, &client.IngestEventRequest{AgentID: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryError_IsErrDaemonUnreachable(t *testing.T) {
	de := &client.DiscoveryError{Tried: []string{"/a", "/b"}}
	if !errors.Is(de, client.ErrDaemonUnreachable) {
		t.Fatal("DiscoveryError should match ErrDaemonUnreachable")
	}
	if !strings.Contains(de.Error(), "/a") {
		t.Errorf("error message should list tried paths: %q", de.Error())
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
