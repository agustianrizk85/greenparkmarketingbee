package handler

import (
	"encoding/json"
	"errors"
	"html"
	"io"
)

// Small shared helpers for the Meta OAuth handler.

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

func errString(s string) error { return errors.New(s) }

func htmlEscape(s string) string { return html.EscapeString(s) }

// jsonString encodes s as a JSON string literal (with surrounding quotes) for
// safe embedding inside an inline <script>.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
