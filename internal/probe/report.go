package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Field struct {
	Pointer string          `json:"pointer"`
	Type    string          `json:"type"`
	Value   json.RawMessage `json:"value,omitempty"`
}

// JSON Pointer paths distinguish object names, array indexes and absent values.
// UseNumber keeps large onchain integers exact instead of converting to float64.
func describeJSON(body []byte) []Field {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil
	}
	fields := []Field{}
	var walk func(string, any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			fields = append(fields, Field{Pointer: path, Type: "object"})
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				escaped := strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
				walk(path+"/"+escaped, x[k])
			}
		case []any:
			fields = append(fields, Field{Pointer: path, Type: "array"})
			for i, item := range x {
				walk(path+"/"+strconv.Itoa(i), item)
			}
		default:
			typ := "null"
			switch x.(type) {
			case string:
				typ = "string"
			case bool:
				typ = "boolean"
			case json.Number:
				typ = "number"
			}
			encoded, _ := json.Marshal(v)
			fields = append(fields, Field{Pointer: path, Type: typ, Value: encoded})
		}
	}
	walk("", value)
	return fields
}

type Report struct {
	Version            int      `json:"version"`
	CreatedAt          string   `json:"created_at"`
	EndpointDocs       string   `json:"endpoint_docs"`
	FixtureSource      string   `json:"fixture_source"`
	CollectionComplete bool     `json:"collection_complete"`
	VerdictsConfirmed  bool     `json:"verdicts_confirmed"`
	Notes              []string `json:"notes"`
	Results            []Result `json:"results"`
}

func Run(ctx context.Context, cfg Config, client *Client, output string, log io.Writer) (string, error) {
	if err := os.MkdirAll(output, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(output, "intercepta-"+time.Now().UTC().Format("20060102T150405Z")+"-")
	if err != nil {
		return "", err
	}
	report := Report{
		Version: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		EndpointDocs:  "https://docs.web3antivirus.io/reference/quick-scan-address",
		FixtureSource: client.redact(cfg.AddressSource), CollectionComplete: true,
		Notes: []string{
			"Fixture expectations are user supplied, not API conclusions. No risk verdict is inferred.",
			"collection_complete means two complete 2xx JSON responses only; API error objects still need review.",
			"Mainnet risk screening only. No payment, wallet signature, or chain-specific testnet verification occurs.",
			"duration_ms measures this client's request through body read/processing, not provider-only processing or a latency benchmark.",
			"Response bodies are preserved with exact API-key occurrences redacted if encountered; hashes refer to stored bytes.",
		},
	}
	cases := []struct{ label, address string }{{"normal", cfg.NormalAddress}, {"risk", cfg.RiskAddress}}
	for _, tc := range cases {
		fmt.Fprintf(log, "Scanning %s fixture %s...\n", tc.label, tc.address)
		r := client.Scan(ctx, tc.label, tc.address)
		if r.body != nil {
			ext := ".txt"
			if r.ValidJSON {
				ext = ".json"
			}
			r.ResponseFile = tc.label + "-response" + ext
			if err := os.WriteFile(filepath.Join(dir, r.ResponseFile), r.body, 0600); err != nil {
				return dir, err
			}
		}
		if r.Error != "" {
			report.CollectionComplete = false
		}
		report.Results = append(report.Results, r)
		// Persist after each request so a failed second request doesn't hide the first.
		if err := writeReport(dir, report); err != nil {
			return dir, err
		}
		fmt.Fprintf(log, "%s: HTTP %d, %.2f ms, JSON=%t, risk=unreviewed\n", tc.label, r.HTTPStatus, r.DurationMS, r.ValidJSON)
		if r.Error != "" {
			fmt.Fprintln(log, "  "+r.Error)
		}
	}
	if !report.CollectionComplete {
		return dir, errors.New("response collection incomplete; evidence retained; no risk conclusions made")
	}
	fmt.Fprintln(log, "Two HTTP JSON responses collected. Review the actual fields before confirming normal/risk expectations.")
	return dir, nil
}

func writeReport(dir string, report Report) error {
	// A partial snapshot must never advertise two completed calls.
	report.CollectionComplete = report.CollectionComplete && len(report.Results) == 2
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(body, '\n'), 0600); err != nil {
		return err
	}
	var summary strings.Builder
	fmt.Fprint(&summary, "# Intercepta live response collection\n\n")
	fmt.Fprintf(&summary, "Created: %s\n\n", report.CreatedAt)
	fmt.Fprintf(&summary, "Two complete HTTP JSON responses: **%t**. Risk verdicts confirmed: **false**.\n\n", report.CollectionComplete)
	fmt.Fprintln(&summary, "| Fixture expectation | Address | HTTP | Duration (ms) | Valid JSON | Response |")
	fmt.Fprintln(&summary, "| --- | --- | --- | --- | --- | --- |")
	for _, r := range report.Results {
		fmt.Fprintf(&summary, "| %s | `%s` | %d | %.2f | %t | %s |\n", r.Case, r.Address, r.HTTPStatus, r.DurationMS, r.ValidJSON, r.ResponseFile)
	}
	fmt.Fprintln(&summary, "\nThe exact JSON field paths, types, values and errors are in `report.json`. Fixture labels are expectations, not safety findings.")
	fmt.Fprintln(&summary, "\n## Review before the payment integration\n\n- Identify the actual risk/reason fields and confirm their meaning with the sponsor.\n- Check that the two fixture responses support the expected contrast.\n- Treat missing/unsupported/unknown results separately from an explicitly low-risk result.\n- Record actual API feedback: time to first call, confusing behavior, missing information.")
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(summary.String()), 0600)
}
