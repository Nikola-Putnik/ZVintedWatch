package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) recentActivity(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.Bus.Recent(queryInt(r, "limit", 500)))
}

// activityStream pushes activity events with Server-Sent Events. The browser
// reconnects automatically if the connection drops.
func (s *Server) activityStream(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable buffering in nginx reverse proxies

	events, cancel := s.Bus.Subscribe(512)
	defer cancel()

	fmt.Fprint(w, "retry: 3000\n\n")
	if err := rc.Flush(); err != nil {
		return
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		case e, open := <-events:
			if !open {
				return
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.ID, data)
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
