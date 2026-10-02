# Decision402: Build It Ourselves, Learn from Circle's Code

Updated: 2026-10-02. This is a pending work plan, not an implemented change. [中文](HowToImprove.md)

## Make the direction clear

**We write and run our own features. Circle is a learning reference only. We do not integrate its hosted wallets, CLI, Marketplace search, or Gateway payment services.**

Keep the existing Go, Vue, x402, and go-ethereum libraries. Write the application logic ourselves; do not invent new signing or encryption algorithms. USDC and Arc remain part of the current project goals. The model, RPC, and risk-data services remain external dependencies; this round does not rebuild them too.

Order: **preserve the working version → find two real providers → improve key storage → add budgets → add payment recovery → improve our payment flow → validate delivery and switch providers → user trials.**

Do not learn all of Circle before starting. Read only the code needed for the next step, write our implementation, and test it. Distinguish actual implementations from calls to Circle's backend. Translating a service call into Go does not recreate that backend.

| Existing weakness | Step |
|---|---|
| No real purchasing use case | Find the need and providers in step 1; complete real purchases in step 6. |
| Private keys stored in local files | Step 2. |
| Budgets do not accumulate across tasks | Step 3. |
| Unknown payments cannot recover and block other tasks | Step 4. |
| Paying does not prove the data is useful | Step 6. |
| Overlap with Circle and unclear user value | Compare results in step 7; writing it ourselves does not prove demand. |

## 0. Preserve the working version

- [ ] Save current changes and record the Git commit. Keep keys, `.env`, and model API keys out of Git.
- [ ] Run these checks from the project root. Record existing failures and understand their causes first.

```sh
DECISION402_LIVE_TEST=0 go test ./...
npm --prefix web run build
```

**Done when:** The starting version and check results are recorded. For each later step, record what we studied, changed, and tested in `docs/improvement-log.md`. Create it during implementation and keep credentials out.

## 1. Build our service list, starting with two real providers

**Read first:** `mapSearchItem` and `preferredAccept` in [services.ts](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts). Learn the service fields and how payment chains are selected. `searchServices` calls Circle CLI; do not copy that call or install the CLI.

**Implementation order:**

1. Find one developer with a real purchasing need. Record what they buy, why, and how often. Do not wait for five interviews before starting development.
2. Find two comparable APIs through provider websites or public documentation. Prefer direct x402 support. Record URLs, methods, parameters, response fields, data timestamps, prices, and payment chains. Start with free or test samples, without real-money purchases.
3. Add `config/providers.json` and `internal/agent/providers.go` to load and filter these providers ourselves. Update `NewApp` in [app.go](../../internal/agent/app.go); A/B/C/D are no longer real-provider candidates. Separate demo and real services, keeping `SellerHandler` limited to our own demo routes.
4. Extend `Service` in [policy.go](../../internal/agent/policy.go) with request methods and parameters. Update `Quote` in [payment.go](../../internal/agent/payment.go) to request the provider's HTTP 402 quote directly. Check amount, token, chain, and recipient. Select a supported payment option rather than blindly taking the first.
5. If the use case changes, update `find_services` in [model.go](../../internal/agent/model.go) and the hardcoded Tokyo-weather instructions in [app.go](../../internal/agent/app.go). The model proposes a task; our code restricts URLs, budgets, and recipients.

- [ ] **Done when:** Preview shows both candidates, quotes, and risk results without Circle CLI or its search API. Unknown risk or unsupported payment methods prevent payment. If no suitable candidates are found within two days, change the use case before building a full search platform.

## 2. Build wallet management, starting with encrypted keys and signing permissions

**Read first:** Examples and tests in the [Modular Wallets SDK](https://github.com/circlefin/modularwallets-web-sdk) to see how wallet operations are separated from UI code. Review the [wallet contracts](https://github.com/circlefin/buidl-wallet-contracts/tree/master/src/msca) for where permission checks happen. These are not Circle's complete MPC wallet backend.

**Implementation order:**

1. Keep ordinary EVM wallets for the first version. Do not rebuild MPC or smart-wallet contracts yet. Add `internal/agent/wallet.go` for creation, balance lookup, locking, unlocking, and signing. The UI and model must not receive private keys.
2. Use [go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore), from an existing dependency, and add `internal/agent/keystore.go` to manage encrypted keys. Do not store the unlock password alongside the key file or expose it to the model, logs, or frontend.
3. Update [owner.go](../../internal/agent/owner.go): preserve wallet ownership while replacing plaintext `.key` writes with encrypted storage. Test with new test wallets first. Back up existing wallets and verify recovery before migration; do not overwrite them directly.
4. Update [payment.go](../../internal/agent/payment.go) to sign only the approved payment through our wallet module. Do not expose arbitrary signing to the model. Changing limits, unlocking, and pausing require user identity checks.
5. Define pause behavior: stop new signatures, but keep checking already-signed payments. Separate model API keys from ordinary metadata and restrict read access.

- [ ] **Done when:** Test wrong passwords, payments while locked, pause, user A accessing user B's wallet, backup recovery, and credential leaks in logs. Encrypted files alone do not make public multi-user custody ready; this round is limited to controlled tests and small-spend trials.

## 3. Write cumulative budgets without Circle's limit service

**Read first:** The [spending-limit documentation](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies) and [agent-wallet-policy](https://github.com/circlefin/skills/blob/master/plugins/circle/skills/agent-wallet-policy/SKILL.md). Learn limit types and user confirmation flows only. These documents are not backend ledger source; we implement the following ourselves.

**Implementation order:**

1. Add a daily agent cap in [policy.go](../../internal/agent/policy.go), with a shared overall cap per wallet. Keep per-payment and task limits. Leave weekly and monthly limits for later.
2. Add `internal/agent/ledger.go`, initially using SQLite. Store the task, agent, wallet, integer amount, payment ID, authorization ID, time, and status.
3. Connect it to `beforeSign` in [payment.go](../../internal/agent/payment.go): check and reserve budget in one database transaction. A failed write prevents signing. Save the complete authorization parameters before sending each signed authorization.
4. Settled payments become spent budget; unknown outcomes stay reserved. Release only after confirming no payment occurred and the original authorization can no longer charge. Repeated updates must not charge or release twice. Provider switching uses the same task budget.
5. Use Beijing time for daily accounting. Unresolved payments crossing midnight remain reserved. Show API budgets, model costs, and network fees separately.

- [ ] **Done when:** A simulated daily cap of 0.10 USDC permits at most two of three concurrent payments of 0.04 each. On the same day, restarts, new tasks, and new agents sharing the wallet must not allow the third. Also test midnight, failed writes, and crashes during signing. Test amounts are not mainnet spending authorization.

## 4. Build our own payment lookup and restart recovery

**Read first:** [Steps 3–5 of Circle's Facilitator tutorial](https://developers.circle.com/facilitator-service/quickstart) for fixed payment IDs, unknown outcomes, and later queries. Learn the behavior without calling that service. Implement using the project's existing [x402 Go](https://github.com/x402-foundation/x402/tree/main/go) foundation.

**Implementation order:**

1. Extend `Task` in [app.go](../../internal/agent/app.go) and `PaymentOutcome` in [payment.go](../../internal/agent/payment.go). Store the payment ID, authorization nonce, validity window, amount, sender, and recipient. Duplicate submissions return the original task; changed amounts or recipients are rejected.
2. Add `internal/agent/reconcile.go`. Query transaction receipts, authorization state, and events through the target chain's RPC. Without a transaction hash, search using the saved authorization ID and block range. Match the chain, token, sender, recipient, and amount, and apply the chain's confirmation rules before recording settlement.
3. A missing transaction, RPC timeout, or used authorization does not establish the payment outcome by itself. Match payment or cancellation evidence. Release budget only after confirming failure or expiry with no possible later settlement; otherwise keep the outcome unknown.
4. Update `NewApp` to resume payment lookup after restart, without signing again. After ledger and recovery tests pass, replace global blocking in `createTask` and the global lock in `run` with per-wallet handling.

- [ ] **Done when:** A successful payment with a lost response is found after restart, duplicate requests do not charge again, and another wallet keeps working. Also test RPC failures, late settlement, and canceled authorizations.

## 5. Improve our payment flow and verify settlement requirements

**Read first:** [arc-nanopayments/agent.mts](https://github.com/circlefin/arc-nanopayments/blob/master/agent.mts) for purchase steps and result records only. Its Gateway calls are not complete settlement source. For implementation, read the [x402 Go client](https://github.com/x402-foundation/x402/blob/main/go/CLIENT.md) and [facilitator](https://github.com/x402-foundation/x402/blob/main/go/FACILITATOR.md) docs against the project's pinned version; do not assume the latest branch has the same interfaces.

**Implementation order:**

1. Keep the existing x402 `exact` approach. Make the sequence in [payment.go](../../internal/agent/payment.go) explicit: **read quote → check recipient and amount → fresh risk check → reserve budget → save authorization and sign → send request → reconcile → update records.** Our Go code controls each decision.
2. Test success, changed quotes, changed recipients, exceeded limits, unknown risk, and connection loss after signing. Model or provider text must not change the rules.
3. Separate buyer and seller responsibilities. When buying another provider's API, the seller submits settlement; we retain the authorization and independently check the outcome. We do not control the seller's infrastructure and must not count it as our own implementation.
4. To run our own demo seller's settlement as well, add `cmd/facilitator` using the open-source x402 facilitator components. Run verification, transaction submission, and status storage ourselves. Replace the hardcoded `https://x402.org/facilitator` in `SellerHandler` with configuration pointing to our service. This is additional seller-side work: separately test duplicate submissions, restart, insufficient gas funds, and failed transactions. Do not copy Circle's hosted service integration.
5. Do not rebuild Gateway batching or crosschain systems for the first version. Test the target chain, USDC contract, signing format, settlement, and risk data individually. Changing the chain ID alone does not establish Arc support.

- [ ] **Done when:** The buyer's core flow makes no calls to Circle's hosted APIs. After mock and testnet checks pass, set explicit mainnet per-payment and total caps before small real-money tests. If we run demo settlement ourselves, its recovery tests must pass separately.

## 6. Validate delivery ourselves, then add provider switching

**Reference:** Circle examples explain payment flows; they do not establish whether purchased data is useful.

1. Complete one real purchase from each provider. Add `internal/agent/delivery.go` to normalize output and check required fields, task relevance, and freshness. Label quality checks we cannot perform as “not checked.”
2. Store payment and delivery states separately. Update [summary.go](../../internal/agent/summary.go) so “paid” does not automatically mean “task complete.”
3. In `discover` and `execute_purchase` in [app.go](../../internal/agent/app.go), choose another remaining provider only after a pre-signing rejection. Recheck quotes and risk, using the original budget, allowed URLs, and data-sharing scope.
4. Allow at most one switch in the first version. Signed but unknown payments must be reconciled first. Paid but poor data must not trigger automatic repurchase. Even a supported free refetch must disable automatic payment signing.

- [ ] **Done when:** Test normal purchase, pre-signing switching, failed risk lookup, unknown payment status, stale data, and no usable provider. Record provider, signing status, cost, and result. Label mocks separately from real calls.

## 7. Make the UI clear, then run user trials

**Read first:** Payment-history pages in [arc-nanopayments/app](https://github.com/circlefin/arc-nanopayments/tree/master/app) for what information to display. Build the UI ourselves in Vue.

1. Update [App.vue](../../web/src/App.vue): task, budget, allowed services, start, and pause. Show the provider, cost, data acceptance, and failure reason.
2. Ask two target developers to try their own tasks. Start with preview, then small payments after financial checks pass. Record all costs, time, manual interventions, and repeat use.
3. Compare fixed-provider and automatic selection under the same budget, covering normal operation, outages, risk rejection, and stale data. Record cases with no improvement too.

- [ ] **Done when:** Actual records explain what users save and at what cost. Building it ourselves or completing a feature list cannot replace this step.

## How to take notes when studying Circle

For each file, record four things: **inputs, checks, outputs, and where we will implement it.** Read the source and tests, write our version, then test our own failure cases. Code that calls an external SDK is not the SDK or backend implementation. Check licenses before copying or modifying code; changing languages does not automatically remove license obligations.

## Timing and review findings

- Initially allow two days for steps 0–1 and defining the key-storage changes in step 2. Estimate the rest afterward. Building it ourselves brings more responsibility; a complete mature product in two weeks is not promised.
- The first five implementation limitations have direct code evidence and high confidence. Why users need our product still requires user trials. This review has not confirmed public source for Circle's complete wallet, budget, or marketplace backends; the plan does not depend on obtaining that code.
- Target-chain support, recovery correctness, key storage, and demand still need testing. Encrypted files or passing tests alone do not prove public multi-user custody is safe.
- Start with one use case, two providers, and one chain. Keep the existing model and risk-data services for now. Missing risk information stops payment; do not invent a safe verdict.

Treat the Arc application as a separate goal. The [event page](https://community.arc.io/public/events/arc-microgrants-f8tijfjhyq) currently requires a working Arc mainnet deployment and lists **October 15, 2026, at 11:59 Beijing time (UTC+8)** as the deadline. Prepare the repository and submission once a small-spend mainnet demo passes its checks. Do not skip payment tests to meet the deadline.

More source links are in the [Circle study notes](<../Learn(LearnFromThese)/WebSite/WebSiteCanLearn/Circle/CircleCodeStudy.en.md>); optional ideas are in [Think](<../Think(SomeIdeas)/Think(SomeIdeas).en.md>).

**Start now: run step 0's checks, read the data-mapping parts of `services.ts`, then write the two-provider comparison and our service configuration.**
