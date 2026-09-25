package probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const endpoint = "https://api.web3antivirus.io/api/public/v2/extension/account/"
const maxResponseBytes = 1 << 20

type Client struct {
	key  string
	http *http.Client
}

func NewClient(key string, timeout time.Duration) *Client {
	return &Client{key: key, http: &http.Client{
		Timeout: timeout,
		// Never forward the API key to a redirect destination.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type Result struct {
	Case               string  `json:"case"`
	ExpectedProfile    string  `json:"expected_profile"`
	Address            string  `json:"address"`
	URL                string  `json:"url"`
	StartedAt          string  `json:"started_at"`
	DurationMS         float64 `json:"duration_ms"`
	HTTPStatus         int     `json:"http_status"`
	HTTPOK             bool    `json:"http_ok"`
	ContentType        string  `json:"content_type,omitempty"`
	ValidJSON          bool    `json:"valid_json"`
	ResponseBytesRead  int     `json:"response_bytes_read"`
	Truncated          bool    `json:"truncated"`
	SecretRedacted     bool    `json:"secret_redacted"`
	StoredBodySHA256   string  `json:"stored_body_sha256"`
	ResponseFile       string  `json:"response_file"`
	Error              string  `json:"error,omitempty"`
	RiskInterpretation string  `json:"risk_interpretation"`
	Fields             []Field `json:"fields"`
	body               []byte
}

// Scan records transport evidence only. "normal" and "risk" are fixture
// expectations, never a fabricated API verdict. No response schema is assumed.
func (c *Client) Scan(ctx context.Context, label, address string) (r Result) {
	start := time.Now()
	r = Result{Case: label, ExpectedProfile: label, Address: address,
		StartedAt: start.UTC().Format(time.RFC3339Nano), RiskInterpretation: "unreviewed", Fields: []Field{}}
	defer func() { r.DurationMS = float64(time.Since(start)) / float64(time.Millisecond) }()
	if !addressPattern.MatchString(address) {
		r.Error = "invalid EVM address; request not sent"
		return
	}
	r.URL = endpoint + address + "/quick-scan"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		r.Error = "cannot construct request"
		return
	}
	req.Header.Set("X-API-KEY", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		r.Error = c.redact(err.Error())
		return
	}
	defer resp.Body.Close()
	r.HTTPStatus = resp.StatusCode
	r.HTTPOK = resp.StatusCode >= 200 && resp.StatusCode < 300
	r.ContentType = c.redact(resp.Header.Get("Content-Type"))
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	r.ResponseBytesRead = len(body)
	if len(body) > maxResponseBytes {
		r.Truncated = true
		body = body[:maxResponseBytes]
	}
	r.body = []byte(c.redact(string(body)))
	r.SecretRedacted = !bytes.Equal(body, r.body)
	hash := sha256.Sum256(r.body)
	r.StoredBodySHA256 = hex.EncodeToString(hash[:])
	r.ValidJSON = json.Valid(r.body) && readErr == nil && !r.Truncated
	if r.ValidJSON {
		r.Fields = describeJSON(r.body)
	}
	switch {
	case readErr != nil:
		r.Error = "reading response: " + c.redact(readErr.Error())
	case r.Truncated:
		r.Error = "response exceeded 1 MiB; retained prefix only"
	case !r.HTTPOK:
		r.Error = fmt.Sprintf("HTTP %d; inspect retained response; no automatic retry", resp.StatusCode)
	case !r.ValidJSON:
		r.Error = "HTTP success but response is not valid JSON"
	}
	return
}

func (c *Client) redact(value string) string {
	if c.key == "" {
		return value
	}
	return strings.ReplaceAll(value, c.key, "[REDACTED]")
}
