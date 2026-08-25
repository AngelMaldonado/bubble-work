package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Live updates (F4): a Server-Sent Events stream of the caller's bubbles, plus
// the fan-out it rides on. Split out of server.go in docs/journal/PLANE-SYNC.md Phase 7.
//
// Since Phase 2 a broadcast is triggered by the MIRROR changing rather than by a
// Plane fetch completing, which is why the board now repaints within a sync
// interval instead of on a fixed timer.

// subscribe registers an SSE listener; unsubscribe removes it.
//
// The channel carries a THREAD ID: empty means "the board changed", a value
// means "this thread's artifacts changed". Without the id an open thread could
// only be refreshed by re-fetching it on every board nudge — which is a fetch
// per client per sync tick for a thread that usually did not change.
func (s *Server) subscribe() chan string {
	ch := make(chan string, 1)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan string) {
	s.subsMu.Lock()
	delete(s.subs, ch)
	s.subsMu.Unlock()
}

// broadcast nudges every listener non-blockingly (a full buffer means an update
// is already pending, so the drop is harmless).
func (s *Server) broadcast() { s.nudge("") }

// broadcastThread nudges listeners about one thread's artifacts, so a client
// watching that thread repaints while an agent is editing it (MCP-ACCESS.md).
func (s *Server) broadcastThread(threadID string) { s.nudge(threadID) }

func (s *Server) nudge(threadID string) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- threadID:
		default:
			// A pending nudge is already queued. Dropping a BOARD nudge is
			// harmless; dropping a THREAD one would lose the id, so the queued
			// nudge is left alone and the client's board refresh catches up.
		}
	}
}

// handleStream is a Server-Sent Events stream of the caller's bubbles (F4). It
// pushes the current board on connect and whenever the snapshot changes, so the
// UI doesn't have to poll. A heartbeat keeps intermediaries from timing out.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // stop reverse proxies buffering the stream

	sub := s.subscribe()
	defer s.unsubscribe(sub)

	send := func() bool {
		vs, err := s.Bubbles(r.Context())
		if err != nil {
			return true // transient upstream — keep the connection, retry next nudge
		}
		data, err := json.Marshal(vs)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: bubbles\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() { // initial board on connect
		return
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case tid := <-sub:
			if !send() {
				return
			}
			if tid != "" {
				// A second, cheaper event: the client re-fetches this thread only
				// if it is the one on screen.
				if _, err := fmt.Fprintf(w, "event: thread\ndata: %q\n\n", tid); err != nil {
					return
				}
				flusher.Flush()
			}
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
