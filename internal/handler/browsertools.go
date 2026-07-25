package handler

import (
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/browsertools"
)

// browserToolsSocket adapts a websocket connection to browsertools.Socket. The
// mutex is not optional: the hub writes to this end from the *counterpart's*
// read loop, and a websocket connection permits only one writer at a time.
type browserToolsSocket struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (s *browserToolsSocket) Send(frame []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, frame)
}

// BrowserToolsWS upgrades one end of a user's browser-tool channel: the
// extension that executes tool calls, or the harness that issues them, told
// apart by `?role=`. Authentication has already happened in RequireAuthWS, whose
// resolved user id the upgraded connection inherits — it, not anything the
// client claims, is what the frames are routed by.
//
// The connection is a pure conduit: every frame read here is handed to the hub
// verbatim, so adding a tool needs no change on the server.
func (a *API) BrowserToolsWS() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		user, ok := conn.Locals(auth.LocalsUserID).(int64)
		if !ok {
			return
		}
		role, ok := browsertools.ParseRole(conn.Query("role"))
		if !ok {
			// Nothing can be routed to or from an unknown end; close rather than
			// hold a connection that will never be addressed.
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"role must be extension or harness"}`))
			return
		}

		leave := a.browserTools.Join(user, role, &browserToolsSocket{conn: conn})
		defer leave()

		for {
			messageType, frame, err := conn.ReadMessage()
			if err != nil {
				return // the peer went away; leave() detaches this end
			}
			if messageType != websocket.TextMessage {
				continue
			}
			a.browserTools.Forward(user, role, frame)
		}
	}, websocket.Config{
		// Echo back only the marker, never the JWT the client sent beside it.
		Subprotocols: []string{auth.WSSubprotocolMarker},
	})
}
