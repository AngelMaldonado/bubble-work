package bubble

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// The actual shell command must terminate both during a long poll and between
// polls, and adding the HTTP status must not leave a newline in the next seq.
func TestWatchCommandEndsWhenArtifactIsGone(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is required to exercise the generated watch command")
	}
	for _, tc := range []struct {
		name   string
		status int
		answer bool
		want   string
	}{
		{"gone_before_poll", 404, false, "fin: el artefacto ya no está disponible seq=3\n"},
		{"closed_during_poll", 200, false, "fin: el artefacto se cerró seq=3\n"},
		{"answer_then_gone", 404, true, "respondidas Q1 seq=9\nfin: el artefacto ya no está disponible seq=9\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				wantSeq := "3"
				if n > 1 {
					wantSeq = "9"
				}
				if got := r.URL.Query().Get("since"); got != wantSeq {
					t.Errorf("since=%q, want %q", got, wantSeq)
				}
				if tc.answer && n == 1 {
					fmt.Fprintln(w, "respondidas Q1 seq=9")
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprintln(w, "fin: el artefacto se cerró seq=3")
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			got, err := exec.CommandContext(ctx, "sh", "-c", watchCommand(server.URL, "artifact", "key", 3)).CombinedOutput()
			if err != nil {
				t.Fatalf("watch did not terminate cleanly: %v; output %q", err, got)
			}
			if string(got) != tc.want {
				t.Errorf("output=%q, want %q", got, tc.want)
			}
		})
	}
}
