package core

import "golang.org/x/net/websocket"

// windowSizeMessage represents a window resize event message
type windowSizeMessage struct {
	Type string `json:"type"`
	Data struct {
		Rows uint16 `json:"rows"`
		Cols uint16 `json:"cols"`
	} `json:"data"`
}

// sendWindowSize sends a window size update message via WebSocket
func sendWindowSize(ws *websocket.Conn, width, height int) error {
	msg := windowSizeMessage{
		Type: "resize",
		Data: struct {
			Rows uint16 `json:"rows"`
			Cols uint16 `json:"cols"`
		}{
			Rows: uint16(height),
			Cols: uint16(width),
		},
	}
	return websocket.JSON.Send(ws, msg)
}
