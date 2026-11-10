// Package apiclient is the plain HTTP/JSON client shared by every outbound
// API call gw-server makes natively: GitHub, Docker Hub, GCP.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClient is shared by every outbound API call (GitHub, Docker Hub, GCP).
var HTTPClient = &http.Client{Timeout: 60 * time.Second}

// Error is a non-2xx answer from GitHub, Docker Hub or GCP.
type Error struct {
	Status int
	Body   string
}

// Error is "HTTP <status>: <message>", the message pulled out of the API's
// JSON error body when there is one, and capped in length.
func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Body)
	var v struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Errors  []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal([]byte(e.Body), &v) == nil {
		parts := []string{}
		for _, s := range []string{v.Message, v.Detail} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		for _, er := range v.Errors {
			if er.Message != "" {
				parts = append(parts, er.Message)
			} else if er.Code != "" {
				parts = append(parts, er.Field+" "+er.Code)
			}
		}
		if len(parts) > 0 {
			msg = strings.Join(parts, "; ")
		}
	}
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, msg)
}

// IsStatus reports whether err is an *Error with HTTP status code.
func IsStatus(err error, code int) bool {
	ae, ok := err.(*Error)
	return ok && ae.Status == code
}

// DoJSON sends body (if non-nil) as JSON and decodes a 2xx response into
// out (if non-nil).
func DoJSON(ctx context.Context, method, u string, hdr http.Header, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "d8z-gw-server")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &Error{Status: resp.StatusCode, Body: string(b)}
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}
