package handler

import (
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/identity/auth"
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

// BrowserToolsWS upgrades one end of a user's browser-tool channel, the extension (executes tool calls) or the harness
// (issues them), told apart by `?role=`. Frames are routed by the user id RequireAuthWS resolved, not anything the client claims.
// The connection is a pure conduit: frames go to the hub verbatim, so adding a tool needs no server change.
func (a *API) BrowserToolsWS() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		user, ok := conn.Locals(auth.LocalsUserID).(int64)
		if !ok {
			return
		}
		role, ok := browsertools.ParseRole(conn.Query("role"))
		if !ok {
			// Nothing can be routed to or from an unknown end; close rather than hold the connection.
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
