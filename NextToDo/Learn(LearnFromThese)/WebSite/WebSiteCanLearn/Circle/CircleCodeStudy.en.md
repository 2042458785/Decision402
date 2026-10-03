# Circle: Features, Purchase Flow, and Source Code

Sources checked: 2026-10-03. [中文](CircleCodeStudy.md)

## Responsibilities in order

| Order | Stage | Product or tool | Responsibility |
|---|---|---|---|
| 1 | Prepare tools | Circle CLI, Circle Skills | CLI supplies commands used throughout; optional Skills teaches AI tools how to call them. |
| 2 | Wallet and rules | Agent Wallets | Create wallets, set spending/address limits, and sign purchases. |
| 3 | Purchasing funds | Gateway | Accept deposited USDC as purchasing funds. |
| 4 | Find services | Agent Marketplace | Return candidates; the agent or application chooses. |
| 5 | Inspect quotes | CLI, provider | Read payment requirements and set the purchase cap. |
| 6 | Sign | Agent Wallets, payment client | Build authorization, have the wallet sign it, and send it to the provider. |
| 7 | Accept payment | Provider, Gateway | Submit authorization; Gateway verifies, locks buyer funds, and records pending seller funds. |
| 8 | Deliver and settle | Provider, CLI, Gateway | Provider returns data, CLI displays it, and Gateway later batches settlement onchain. |

## Purchase steps: Agent Wallets + Marketplace + Gateway

| Step | Who acts | What happens | Source |
|---|---|---|---|
| 1 | User, CLI | Log in with email and verification code; wallets are created after initial authentication | [Wallet tutorial](https://developers.circle.com/agent-stack/agent-wallets/quickstart) |
| 2 | User, Agent Wallets | Set per-payment, daily, weekly, monthly, and address limits. Changes require verification. Mainnet only; spending windows roll over time | [Policy tutorial](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies) |
| 3 | User, Gateway | Fund the wallet with USDC, deposit into Gateway, and confirm available funds. These first three steps can happen in advance | [Purchase tutorial](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 4 | User, agent | User gives a task, such as weather lookup; the agent decides which tools to call | [Product description](https://www.circle.com/blog/introducing-circle-agent-stack-financial-infrastructure-for-the-agentic-economy) |
| 5 | Agent, Marketplace | Use `services search` for candidates | [CLI tutorial](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 6 | Agent, provider | Use `services inspect` for payment requirements. x402 providers return HTTP 402 with amount, asset, chain, and other details | [Buyer tutorial](https://developers.circle.com/gateway-nanopayments/quickstarts/buyer) |
| 7 | Agent or application | Choose provider, wallet, chain, and `--max-amount`. The application decides how to rank providers | [CLI tutorial](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 8 | Payment client, wallet | Start `services pay`; sign authorization based on payment requirements and request the resource with `PAYMENT-SIGNATURE` | [Buyer tutorial](https://developers.circle.com/gateway-nanopayments/quickstarts/buyer) |
| 9 | Provider, Gateway | Submit authorization; Gateway verifies the signature, locks buyer funds, and records pending seller funds. Invalid payments are rejected | [Seller tutorial](https://developers.circle.com/gateway-nanopayments/quickstarts/seller) |
| 10 | Provider, agent | After Gateway accepts payment, return data for CLI to display; check remaining funds. The application still needs to judge data usefulness | [Purchase tutorial](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 11 | Gateway | Batch payments onchain; after confirmation, pending seller funds become available. Data can arrive before this step | [Settlement mechanism](https://developers.circle.com/gateway-nanopayments/concepts/batched-settlement) |

## Alternative: Facilitator Service (separate from the Gateway path above)

| Step | Who acts | What happens |
|---|---|---|
| 1 | Buyer, provider | Request a paid resource; provider returns an HTTP 402 quote |
| 2 | Buyer wallet | Sign payment authorization and request the resource again |
| 3 | Provider | Submit authorization and seller proof to Facilitator |
| 4 | Facilitator | Validate authorization and screen both parties; reject invalid payments or submit the USDC transfer |
| 5 | Provider | Deliver after confirmation; query pending payments. Retain the original authorization on retries rather than signing a new charge |
| Sources | [Official flow](https://developers.circle.com/facilitator-service/how-it-works) | [Payment and status tutorial](https://developers.circle.com/facilitator-service/quickstart); status access belongs to the seller |

## Distinctions that affect the comparison

| Topic | Confirmed / unconfirmed |
|---|---|
| Risk checks | [Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets) documents sanctions screening before transfers go onchain; [Facilitator](https://developers.circle.com/facilitator-service) screens both parties. Equivalent risk ranking and pre-signing rescans are unconfirmed. |
| Gateway spending limits | [Wallet policies](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies) and `--max-amount` are documented. Whether wallet policies cover every Gateway purchase is unconfirmed. |
| Payment and data | [Nanopayments](https://developers.circle.com/agent-stack/agent-nanopayments) handles payments. We have not found equivalent documentation for our proposed data-quality checks and automatic switching. |

## Public code and where to look

| Topic | GitHub and location | Available code / limits |
|---|---|---|
| Web wallets | [modularwallets-web-sdk](https://github.com/circlefin/modularwallets-web-sdk): `examples/`, `packages/w3s-web-core-sdk/` | SDK, examples, and tests; not the complete Agent Wallets backend. |
| Smart wallet permissions | [buidl-wallet-contracts](https://github.com/circlefin/buidl-wallet-contracts): `src/msca/` and tests | Smart accounts and permission plugins; not MPC key custody. |
| Small purchases and history UI | [arc-nanopayments](https://github.com/circlefin/arc-nanopayments): [agent.mts](https://github.com/circlefin/arc-nanopayments/blob/master/agent.mts), [app](https://github.com/circlefin/arc-nanopayments/tree/master/app) | Buyer/seller examples and UI; calls Gateway, not its full settlement backend. |
| Gateway contracts | [evm-gateway-contracts](https://github.com/circlefin/evm-gateway-contracts): `src/`, `test/`, `quickstart/` | Onchain contracts and tests; public availability of the full offchain backend is unconfirmed. [Product docs](https://developers.circle.com/gateway) |
| Configure limits | [agent-wallet-policy](https://github.com/circlefin/skills/blob/master/plugins/circle/skills/agent-wallet-policy/SKILL.md) | Usage instructions; this file does not implement a spending ledger. |
| Search services | [services.ts](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts): `mapSearchItem`, `preferredAccept`, `searchServices` | Field mapping and CLI calls; not marketplace search or review backends. |
| Agent integrations | [starter-kits/kits](https://github.com/circlefin/agent-stack-starter-kits/tree/master/kits) | Examples of agent frameworks calling Circle tools. |
| Our existing open-source libraries (not Circle) | [x402 Go](https://github.com/x402-foundation/x402/tree/main/go), [facilitator](https://github.com/x402-foundation/x402/blob/main/go/FACILITATOR.md), [go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore) | Payment, seller settlement, and encrypted key storage respectively. Check repository licenses before copying or modifying code. |

| Entry point | Link |
|---|---|
| Official documentation | [Agent Stack](https://developers.circle.com/agent-stack) |
| Public repositories | [circlefin](https://github.com/orgs/circlefin/repositories) |
| Service directory | [Marketplace](https://agents.circle.com) |
| Our changes | [HowToImprove](../../../../HowToImprove/HowToImprove.en.md) |
