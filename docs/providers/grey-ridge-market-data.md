# Market-data x402 provider: Grey Ridge Signals (re: #4)

A ready-to-wire **real** external x402 service for issue #4's checklist. Every number
below is callable right now; nothing here needs an account or API key.

## Task 1 — provider, data, update frequency, price, chain, payment method

| field | value |
|---|---|
| provider | Grey Ridge Signals — `x402-data-api.sigrunner.workers.dev` |
| endpoint | `GET /crypto/prices?coins=bitcoin,ethereum` (up to 25 CoinGecko ids/call) |
| data | USD spot price, 24h change %, a `confidence` score, and a per-coin unix `timestamp` |
| price | **0.001 USDC** (atomic `amount: "1000"`, 6-decimal USDC) |
| chain | **Base** — `eip155:8453`; asset USDC `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| payTo | `0x5765ae06a52dc7A0BB71c36A11db512c7ea9ed10` |
| payment method | x402 `exact` scheme, EIP-3009 gasless USDC authorization; `x402-fetch` signs + retries |
| discovery | `GET /.well-known/x402` (machine-readable manifest, x402Version 2) |

> Scope note: this provider covers **crypto** prices (BTC/ETH/etc.), not equities such as
> NVIDIA — stated up front so the agent routes equity requests elsewhere.

## Task 2 — compare with a free API, why pay

A free API (e.g. CoinGecko's public tier) returns the same spot number, so the paid value
is **not** the number — it is the **settlement + provenance** your policy engine already
wants: each paid call is a signed Base USDC receipt tying the quote to an on-chain payment,
and the response is a single keyless HTTP call your agent pays inline (no key to store, no
rate-limit account, no CORS dance). For an agent that must *decide whether to pay*, a free
key-gated API gives you no payment to screen; this gives you exactly the `402 -> screen ->
pay -> receipt` loop Decision402 is built around. Use the free API when you only need the
number with no audit trail; use this when the payment itself is the artifact.

## Task 4 — read the live HTTP 402 quote before paying

Unpaid, the endpoint returns a standard x402 v2 quote your budget/risk step can parse before
signing (verified live):

```json
{
  "x402Version": 2,
  "error": "Payment required",
  "resource": { "url": "https://x402-data-api.sigrunner.workers.dev/crypto/prices?coins=bitcoin,ethereum" },
  "accepts": [{
    "scheme": "exact",
    "network": "eip155:8453",
    "amount": "1000",
    "asset": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
    "payTo": "0x5765ae06a52dc7A0BB71c36A11db512c7ea9ed10",
    "maxTimeoutSeconds": 300
  }]
}
```

## Tasks 3, 5, 7 — preview test + freshness check (reject outdated)

Free preview (no wallet, no payment) returns the real shape so you can test
"reject missing/outdated data" before wiring the paid path:

```sh
curl -s https://x402-data-api.sigrunner.workers.dev/crypto/prices/preview
# {"preview":[{"id":"bitcoin","symbol":"BTC","price":...,"change_24h":...,
#   "confidence":0.99,"timestamp":1792584125}], "note":"Free 1-of-N sample..."}
```

Each item carries a unix `timestamp`. Your "reject outdated" rule (Done-when) maps directly:

```go
if time.Since(time.Unix(item.Timestamp, 0)) > maxStaleness {
    return errOutdated // do not present as real-time
}
```

The paid call (`?coins=...`) returns the same fields for up to 25 ids at once.

## Payment rail note

Live payments settle on **Base mainnet** USDC. Decision402's demo currently runs on Base
**Sepolia** — the 402 quote shape and the screen->pay->receipt flow are identical, so you can
wire and test the integration now and point at mainnet when you graduate the demo. Free
`/preview` needs no testnet funds at all.

---
Maintained by Grey Ridge Signals Group LLC - contact@greyridgesignals.ai - listed on x402scan.
