# Step 1 validation status

Implementation scope: two sequential Intercepta Quick Scan Address requests, response preservation, actual JSON field inventory and client-side elapsed time.

## Observed

- Local toolchain: Go 1.23.2, darwin/amd64.
- `go test ./...`: passed. Tests use local HTTP servers and synthetic fixtures, not Intercepta.
- `go vet ./...`: passed.
- Production request origin and authentication header match the official Quick Scan Address documentation.

## Not yet observed

- A successful authenticated call to Intercepta.
- A real normal-address response or a real risky-address response.
- The provider's actual response schema, risk semantics, or production latency.

The user has not yet received the sandbox key and sponsor fixtures. No live-call result or API feedback has been fabricated. Local tests establish confidence in the tested collection and failure behavior, not in the provider's risk detection.

## Next evidence

Fill `.env` with the key, two distinct sponsor-provided mainnet addresses and fixture source. Run `go run ./cmd/intercepta-probe`. Inspect the generated `report.json` and original response files with the sponsor's field definitions before implementing allow/block decisions.
