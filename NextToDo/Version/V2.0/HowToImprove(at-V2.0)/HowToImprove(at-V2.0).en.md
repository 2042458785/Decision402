# Decision402: Feature Comparison and Improvements

[中文](HowToImprove.md)

## Existing features: compare with Circle and improve

| Feature | What we have | Circle's equivalent | What to change | Status | Date | Commit |
|---|---|---|---|---|---|---|
| Key storage | Plaintext private keys in local files | [Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets) uses MPC with separate key shares that agents cannot access | Update `owner.go` and `payment.go`: encrypt stored keys, centralize signing, and test locking, permissions, and backup recovery. MPC and file encryption are different protections. | Not done |  |  |
| Spending limits | Per-payment and per-task caps | [Wallet policies](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies) offer per-payment, daily, weekly, monthly, and address controls on mainnet | Update `policy.go` and add a ledger: share limits across tasks, reserve before signing, keep unknown payments reserved, and test concurrency and restarts. | Not done |  |  |
| Service discovery | Four fixed local weather samples | [Marketplace search code](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts) reads search results and organizes service details | Update `app.go` and `model.go`: load real providers from configuration, including URLs, inputs, prices, and payment methods. Check whether two providers offer comparable data. | Not done |  |  |
| Quotes and selection | Read 402, rank by budget/risk, and recheck before signing | [CLI tutorial](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) supports search, payment inspection, and a purchase cap; equivalent ranking is unconfirmed | Update `payment.go`: handle real payment options and check amount, asset, chain, and recipient. Keep our ranking; test price and address changes. | Not done |  |  |
| Risk checks | Intercepta address data evaluated by Go rules | [Wallets](https://developers.circle.com/agent-stack/agent-wallets) and [Facilitator](https://developers.circle.com/facilitator-service) offer sanctions screening; this does not establish broader risk coverage | Update the risk interface: save source, time, and reason. Stop for missing or stale results; check again before signing. | Not done |  |  |
| Payments and settlement | Base Sepolia test USDC; seller submits settlement | [Gateway](https://developers.circle.com/agent-stack/agent-nanopayments) batches small payments; [Facilitator](https://developers.circle.com/facilitator-service) settles USDC on Arc, Base, and Polygon | Keep our x402 payment flow and add failure tests. For a new chain, verify contracts, signing, settlement, and recovery. Assess batching against actual demand. | Not done |  |  |
| Payment recovery | Unknown payments need manual review and block other payments | [Facilitator's tutorial](https://developers.circle.com/facilitator-service/quickstart) provides payment IDs, pending states, and seller queries | Update `app.go` and `payment.go`: save the original authorization, resume chain checks after restart, avoid another charge while unresolved, and isolate wallets. Buyer access differs from the seller query API. | Not done |  |  |
| Result records | Payment results and transaction hashes without automatic recovery | [Gateway](https://developers.circle.com/gateway-nanopayments/concepts/batched-settlement) distinguishes acceptance from onchain settlement | Update task records and UI: show signed, pending, paid, or failed, plus recovery results. Use states that match our payment method. | Not done |  |  |

## Features we could add

| Feature | What it would do |
|---|---|
| Check data usefulness | Check location, date, update time, and required fields. Show payment status separately from data quality. |
| Switch providers | Before signing, try another provider with fresh quote and risk checks under the same budget. Resolve unknown payments first; buying again after payment needs user authorization. |
| Pause new payments | Stop new signatures when paused; continue checking payments already sent. |
| An API for other agents | Accept a task, budget, and allowed services; return data, spending, and failure reasons. |
| Provider history | Record success, stale data, prices, and response times to help users choose. |

[Current code and flow](<../Version/V1.0(ETHGlobalTokyo2026Hackathon)/V1.0.en.md>) · [Circle flow and source code](<../Learn(LearnFromThese)/WebSite/WebSiteCanLearn/Circle/CircleCodeStudy.en.md>) · [Product ideas](<../Think(SomeIdeas)/Think(SomeIdeas).en.md>)
