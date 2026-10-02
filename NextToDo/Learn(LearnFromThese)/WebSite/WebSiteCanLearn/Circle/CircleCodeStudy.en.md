# Circle: Where to Look and What to Learn

Checked: 2026-10-02. The direction is now to build it ourselves: study public code and product rules, then implement our own features. Do not integrate Circle CLI, hosted wallets, search, or Gateway services. A Circle account is not required to begin studying. [中文](CircleCodeStudy.md)

“Build ourselves” describes planned work. New module names show where we intend to put the code, not features already implemented.

## Start with these pages

| Page | What it contains |
|---|---|
| [Circle GitHub repositories](https://github.com/orgs/circlefin/repositories) | Public code. Search for the repository names listed below. |
| [Agent Stack documentation](https://developers.circle.com/agent-stack) | The main entry point for wallets, payments, and service discovery. Look under “The agent stack” and “Dive deeper.” |
| [Agent Marketplace](https://agents.circle.com) | Observe the fields shown in service listings. This is a product reference, not a project dependency. |

**Use the website to understand features and GitHub to study implementations. Code that calls a Circle API is not the implementation behind that API.**

## 1. Wallets: creation, signing, and spending permissions

- **Product overview:** [Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets). Explains how agents use wallets and how users restrict them. The docs describe MPC key management, with key shares kept out of the agent's reach.
- **Web SDK source:** [modularwallets-web-sdk](https://github.com/circlefin/modularwallets-web-sdk). Start with the README, then [examples](https://github.com/circlefin/modularwallets-web-sdk/tree/master/examples) and the [SDK directory](https://github.com/circlefin/modularwallets-web-sdk/tree/master/packages/w3s-web-core-sdk) to learn how to integrate smart wallets into a web app.
- **Wallet contract source:** [buidl-wallet-contracts](https://github.com/circlefin/buidl-wallet-contracts). Start with [src/msca](https://github.com/circlefin/buidl-wallet-contracts/tree/master/src/msca) and the tests to learn about smart accounts and permission plugins.
- **Build ourselves:** `wallet.go` for wallets, permissions, and signing; `keystore.go` for encrypted keys. Use Circle code to study module responsibilities. For ordinary EVM wallet storage, use the existing [go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore). Do not build MPC in the first version.
- **What this does not include:** Modular Wallets and Agent Wallets are not one complete shared product codebase. These two repositories do not let us copy Circle's account services, MPC signing infrastructure, and entire backend. The contract repository is marked GPL-3.0; read its license before copying code.

## 2. Payments: how buyers pay and sellers receive money

- **Tutorial:** [Make a nanopayment](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart). Covers funding, service search, quote inspection, payment, and balance checks.
- **Example source:** [arc-nanopayments](https://github.com/circlefin/arc-nanopayments). Read [agent.mts](https://github.com/circlefin/arc-nanopayments/blob/master/agent.mts), then [app](https://github.com/circlefin/arc-nanopayments/tree/master/app). Study buyer payments, paid seller endpoints, and payment history pages.
- **Underlying system:** The [Gateway docs](https://developers.circle.com/gateway) explain a unified USDC balance. [evm-gateway-contracts](https://github.com/circlefin/evm-gateway-contracts) contains the related EVM contract source. Start with `src/`, `test/`, and `quickstart/`.
- **Build ourselves:** Improve quotes, signing, payments, and records in `payment.go`. Keep the existing [x402 Go library](https://github.com/x402-foundation/x402/tree/main/go) without Circle Gateway API calls. If running our own demo settlement service, study the x402 facilitator documentation and source.
- **What this does not include:** The Nanopayments example is described as a testnet app that needs changes for production use. Public Gateway contracts do not mean all its offchain services are included in the repository.

## 3. Budgets: limits per payment, day, week, and month

- **Product tutorial:** [Set spending policies](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies). Covers per-payment, daily, weekly, and monthly limits, plus recipient allowlists and blocklists.
- **GitHub instructions:** [agent-wallet-policy/SKILL.md](https://github.com/circlefin/skills/blob/master/plugins/circle/skills/agent-wallet-policy/SKILL.md), in [circlefin/skills](https://github.com/circlefin/skills). Explains how to view, change, and reset limits. Changes require user verification. The current instructions apply only to mainnet Agent Wallets.
- **Build ourselves:** Store rules in `policy.go` and check cumulative spending in `ledger.go`. Handle concurrent reservations, settled spending, and safe releases in the database. Learn the feature types from Circle, but use Beijing calendar-day limits in our first version without its limit API.
- **What this does not include:** These are usage instructions, not the backend budget ledger source. They do not provide the implementation for concurrent spending, restart recovery, or task budgets. A spending total in a demo is not sufficient for strict budget enforcement either.

## 4. Service discovery: finding providers, prices, and APIs

- **Website:** [Agent Marketplace](https://agents.circle.com). Observe listing fields and the workflow without requiring its search service.
- **Source:** [agent-stack-starter-kits](https://github.com/circlefin/agent-stack-starter-kits). Focus on [services.ts](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts): it runs the search command and extracts service URLs, names, prices, chains, and request methods.
- **Agent integration examples:** The same repository's [kits](https://github.com/circlefin/agent-stack-starter-kits/tree/master/kits) show how different agent frameworks call Circle tools.
- **Build ourselves:** Collect two providers from their documentation into `providers.json`. Load and filter them in `providers.go`; read quotes directly with `Quote`. Learn field mapping from `mapSearchItem`, without copying the Circle CLI call in `searchServices`.
- **What this does not include:** Public search-calling code does not mean the marketplace's listing, search engine, and review backend are fully public. Finding two services does not prove they are interchangeable; test them.

## 5. Payment failures: avoiding a second charge

- **Key tutorial:** [Facilitator Service quickstart](https://developers.circle.com/facilitator-service/quickstart). A facilitator helps sellers verify and settle payments.
- **Read these steps first:** Step 3 for the payment identifier, step 4 for `pending` (the outcome is not yet known), and step 5 for status lookup.
- **Build ourselves:** Use `reconcile.go` to retain and look up the original authorization, check transactions and events through chain RPC, and resume after restart. Learn the rule to keep checking unknown outcomes without charging again; do not call Circle's lookup service.
- **What this does not include:** This is an API tutorial using Arc testnet. Status queries require seller proof, so we cannot assume buyers have the same access. This review has not confirmed a public repository for the complete backend.

## Suggested reading order

1. **Field mapping in `services.ts`:** write our service configuration and quote lookup.
2. **Wallet SDK examples and tests:** write our wallet module and encrypted storage; there is no need to read every contract first.
3. **Limit and payment-failure documentation:** write our ledger and recovery flow. Those backend implementations are not included in the instructions.
4. **Payment examples and UI:** improve our x402 payments and Vue result page without copying Gateway calls.

For each resource, record **inputs, checks, outputs, and where we will implement it.** Read the license before copying or modifying code. Follow [HowToImprove](../../../../HowToImprove/HowToImprove.en.md) for the full work order.
