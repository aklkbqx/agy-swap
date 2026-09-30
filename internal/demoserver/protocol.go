package demoserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const (
	// The TUI renders down to 28x12 (make tui-smoke), so the gateway allows it.
	minCols = 28
	maxCols = 180
	minRows = 12
	maxRows = 60
)

func clamp(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func parseResize(data []byte) (int, int, error) {
	var message struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil || message.Type != "resize" {
		return 0, 0, errors.New("invalid resize message")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return 0, 0, errors.New("trailing resize data")
	}
	return clamp(message.Cols, minCols, maxCols), clamp(message.Rows, minRows, maxRows), nil
}
