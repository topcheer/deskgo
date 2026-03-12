package relay

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func newRelayTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	globalService = nil
	serviceOnce = sync.Once{}

	router := gin.New()
	router.GET("/api/desktop/:session_id", HandleDesktopConnection)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsBaseURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/desktop"
	return server, wsBaseURL
}

func dialTestConn(t *testing.T, url string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial websocket %s: %v", url, err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return conn
}

type desktopMessageResult struct {
	msg DesktopMessage
	err error
}

func startDesktopMessageReader(t *testing.T, conn *websocket.Conn) <-chan desktopMessageResult {
	t.Helper()

	results := make(chan desktopMessageResult, 8)
	go func() {
		defer close(results)
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				results <- desktopMessageResult{err: err}
				return
			}

			var msg DesktopMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				results <- desktopMessageResult{err: err}
				return
			}

			results <- desktopMessageResult{msg: msg}
		}
	}()
	return results
}

func awaitDesktopMessage(t *testing.T, results <-chan desktopMessageResult, timeout time.Duration) DesktopMessage {
	t.Helper()

	select {
	case result, ok := <-results:
		if !ok {
			t.Fatal("desktop message channel closed unexpectedly")
		}
		if result.err != nil {
			t.Fatalf("read desktop message: %v", result.err)
		}
		return result.msg
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for desktop message after %s", timeout)
		return DesktopMessage{}
	}
}

func TestStopCaptureWhenLastViewerDisconnectsAndResume(t *testing.T) {
	_, wsBaseURL := newRelayTestServer(t)
	sessionID := "stop-capture-resume"

	cliConn := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=client&user_id=cli")
	cliMessages := startDesktopMessageReader(t, cliConn)
	viewerOne := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=web&user_id=viewer-1")

	startMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if startMsg.Type != "start_capture" {
		t.Fatalf("expected start_capture, got %q", startMsg.Type)
	}

	if err := viewerOne.Close(); err != nil {
		t.Fatalf("close viewer one: %v", err)
	}

	stopMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if stopMsg.Type != "stop_capture" {
		t.Fatalf("expected stop_capture, got %q", stopMsg.Type)
	}

	viewerTwo := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=web&user_id=viewer-2")
	startAgainMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if startAgainMsg.Type != "start_capture" {
		t.Fatalf("expected second start_capture, got %q", startAgainMsg.Type)
	}

	if err := viewerTwo.Close(); err != nil {
		t.Fatalf("close viewer two: %v", err)
	}
}

func TestStopCaptureOnlyAfterLastViewerDisconnects(t *testing.T) {
	_, wsBaseURL := newRelayTestServer(t)
	sessionID := "stop-capture-last-viewer"

	cliConn := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=client&user_id=cli")
	cliMessages := startDesktopMessageReader(t, cliConn)
	viewerOne := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=web&user_id=viewer-1")

	startMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if startMsg.Type != "start_capture" {
		t.Fatalf("expected first start_capture, got %q", startMsg.Type)
	}

	viewerTwo := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=web&user_id=viewer-2")
	secondStartMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if secondStartMsg.Type != "start_capture" {
		t.Fatalf("expected second start_capture, got %q", secondStartMsg.Type)
	}

	if err := viewerOne.Close(); err != nil {
		t.Fatalf("close viewer one: %v", err)
	}

	select {
	case result, ok := <-cliMessages:
		if !ok {
			t.Fatal("desktop message channel closed while waiting for viewer two")
		}
		if result.err != nil {
			t.Fatalf("unexpected read error while another viewer remains: %v", result.err)
		}
		t.Fatalf("expected no stop_capture while another viewer remains, got %q", result.msg.Type)
	case <-time.After(300 * time.Millisecond):
	}

	if err := viewerTwo.Close(); err != nil {
		t.Fatalf("close viewer two: %v", err)
	}

	stopMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if stopMsg.Type != "stop_capture" {
		t.Fatalf("expected stop_capture after last viewer leaves, got %q", stopMsg.Type)
	}
}

func TestSessionIDsAreCaseInsensitive(t *testing.T) {
	_, wsBaseURL := newRelayTestServer(t)

	cliConn := dialTestConn(t, wsBaseURL+"/Win11Studio?type=client&user_id=cli")
	cliMessages := startDesktopMessageReader(t, cliConn)
	viewer := dialTestConn(t, wsBaseURL+"/win11studio?type=web&user_id=viewer")

	startMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if startMsg.Type != "start_capture" {
		t.Fatalf("expected start_capture, got %q", startMsg.Type)
	}
	if startMsg.SessionID != "win11studio" {
		t.Fatalf("expected normalized session id, got %q", startMsg.SessionID)
	}

	if err := viewer.Close(); err != nil {
		t.Fatalf("close viewer: %v", err)
	}
}

func TestViewerPingGetsRelayPongWithoutForwardingToCLI(t *testing.T) {
	_, wsBaseURL := newRelayTestServer(t)
	sessionID := "relay-ping-pong"

	cliConn := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=client&user_id=cli")
	cliMessages := startDesktopMessageReader(t, cliConn)
	viewer := dialTestConn(t, wsBaseURL+"/"+sessionID+"?type=web&user_id=viewer")

	startMsg := awaitDesktopMessage(t, cliMessages, 2*time.Second)
	if startMsg.Type != "start_capture" {
		t.Fatalf("expected start_capture, got %q", startMsg.Type)
	}

	const timestamp = 12345.678
	if err := viewer.WriteJSON(DesktopMessage{
		Type:      "ping",
		SessionID: sessionID,
		UserID:    "viewer",
		Timestamp: timestamp,
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	var pong DesktopMessage
	if err := viewer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set viewer read deadline: %v", err)
	}
	if err := viewer.ReadJSON(&pong); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pong.Type != "pong" {
		t.Fatalf("expected pong, got %q", pong.Type)
	}
	if pong.Timestamp != timestamp {
		t.Fatalf("expected timestamp %v, got %v", timestamp, pong.Timestamp)
	}

	select {
	case result, ok := <-cliMessages:
		if !ok {
			t.Fatal("desktop message channel closed unexpectedly")
		}
		if result.err != nil {
			t.Fatalf("unexpected cli read error: %v", result.err)
		}
		t.Fatalf("expected ping to stay in relay, got forwarded message %q", result.msg.Type)
	case <-time.After(300 * time.Millisecond):
	}
}
