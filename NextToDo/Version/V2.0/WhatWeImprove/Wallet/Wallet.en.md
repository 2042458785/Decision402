# Wallet changes: phases 1 and 2

[中文](Wallet.md) · 2026-10-05

## Phase 1: local encryption for an ordinary Agent wallet

| Feature | Before phase 1 | What it does now | Why it helps |
|---|---|---|---|
| Wallet private key | The private key was stored as plaintext in a `.key` file. Anyone with its contents could use the wallet. | In phase 1, creating an ordinary Agent wallet generates a complete private key. Your password encrypts it inside the Agent `.json`; no plaintext key file or password is saved. | Copying the Agent file alone cannot spend the wallet's funds. |
| DeepSeek API key | A personal key was stored as plaintext in the Agent `.json`. | Your personal key is encrypted separately with the same password inside the Agent `.json`. If you do not enter one, the server may use its configured shared key. | Someone reading the Agent file cannot directly read your personal key. |
| Lock / unlock | Signing in did not leave a separate wallet lock or require a wallet password. | Agents start locked. Entering the password gives this login session temporary backend access for at most 10 minutes. Lock, logout, expiry, or restart ends it. | Signing in does not authorize payments indefinitely. |
| Signing and model calls | Payment signing read the plaintext `.key` file; model calls read the plaintext personal key. | The private key briefly exists as plaintext in backend memory when signing. The personal DeepSeek key briefly exists there when calling the model. The app makes a best effort to clear them afterward. | Plaintext keys are not kept in a file or held in memory between uses. |
| Backup / restore | There was no user-facing encrypted wallet backup and import. | The downloaded keystore backs up only the encrypted Agent-wallet private key. Import it with the original password when creating an Agent to reuse the wallet address. Re-enter the name and personal DeepSeek key. | You can access the same wallet funds on another computer; this is not a backup of the whole Agent or its tasks. |
| Page, API, and logs | Ordinary APIs already returned only public wallet information, but the local file held a plaintext personal key and model replies were not checked for key echoes. | Password and key inputs are masked and cleared after use. Ordinary API responses and logs do not print plaintext keys. A model reply echoing the API key in use is discarded. | Reduces accidental display of secrets. |

Which files appear: MetaMask sign-in creates no Agent file. Creating an Agent writes `artifacts/agents/<Agent ID>.json` on the project computer. Its name, wallet address, and model settings are readable; its private key and personal DeepSeek key are encrypted. Submitting a task separately writes `artifacts/tasks/<Task ID>.json` with the instruction, budget, providers and quotes, execution, and payment result. Only when you request a backup does the browser download `decision402-<Agent ID>.keystore.json`. It backs up the wallet key, not the model key or tasks.

What happens during the 10 minutes: At unlock, the backend briefly uses the password to open the private key and personal DeepSeek key from disk. It then seals them again in memory with a new random key. It does not retain your password as the 10-minute credential. During that window, it briefly opens the needed plaintext when signing or calling the model, then makes a best effort to clear it. The backend can use both secrets during this window. Go cannot guarantee erasing every copy made by the runtime.

Encryption in plain terms: The password can decrypt the ciphertext back into the original value; an encrypted private key is not an incomplete key. Both disk secrets use go-ethereum V3 encryption separately with the same password. It derives a key with scrypt, encrypts with AES-128-CTR, and checks the password/ciphertext; this is not AES-256. Temporary in-memory sealing uses a new random key and AES-GCM.

Current limit: Encrypted files still live on the computer running the app, and the backend sees plaintext briefly when unlocked. This reduces the risk of copied files but is not yet a mature multi-user custody service.

## Phase 2: owner control over payments

| Feature | After phase 1 | Change | Why | Practical benefit |
|---|---|---|---|---|
| Who controls the wallet | The old Agent was an ordinary wallet. Once unlocked, the backend could sign with its full private key. MetaMask handled sign-in and funding but could not revoke the Agent's private key. | New Agents use ERC-1271 smart wallets controlled by MetaMask. The backend holds only an encrypted restricted session key. | Limit what an unlocked backend can do. | A stolen session key cannot bypass the contract to withdraw smart-wallet USDC. |
| Authorization | The app checked each task's budget and risk, but the wallet had no onchain recipient list or expiry. | The owner sends a MetaMask transaction setting recipients, per-payment and daily limits, and expiry. Initial deployment is another transaction. | Enforce spending boundaries in the wallet contract. | The Agent can buy within those bounds without asking for a MetaMask signature on every purchase. |
| Across-task budget | Each task had a limit; multiple tasks did not share a daily total. | Reserve each payment onchain. Tasks using one smart wallet share its daily limit; reauthorizing does not reset it. | Prevent overspending by splitting purchases into tasks. | With a 0.20 USDC daily limit, a third 0.10 payment fails after two 0.10 reservations. |
| Payment signature | After local checks, an ordinary wallet signed the x402 payment. | Reserve the exact recipient and amount onchain before the session key signs. The contract accepts only that reserved payment digest. | Avoid relying solely on backend rules to constrain the session key. | A backend mistake cannot make that key produce a valid payment to an address outside the whitelist. Each purchase adds a chain transaction. |
| Revocation | Local lock stopped new signatures but could not invalidate payments already signed. | The owner calls `Revoke onchain` from MetaMask. Once mined, unsettled signatures from the old authorization become invalid. | Let the owner withdraw onchain authority. | Stop later payments after detecting a problem. Transfers already completed cannot be reversed. |
| Withdrawal | The UI had no way to return the Agent wallet balance to its owner. | `Withdraw with MetaMask` is owner-only and sends USDC only to the owner's address. | Funds should not be trapped when an Agent is retired. | Recover unused test USDC; the session key cannot withdraw it. |
| Backend outage | A backend-only revoke/withdraw UI would leave the owner dependent on the service being online. | Save `wallet-control.html` and the contract address. The standalone page calls MetaMask directly to revoke or withdraw. | Ownership should still work during an outage. | Manage the deployed wallet even if the project backend stops. |
| Backup meaning | A phase-1 keystore backs up the old ordinary wallet's full private key. | A new smart-wallet keystore backs up only its session key. Restore it and relink the original contract address. | The two keys have different authority. | Continue using the Agent on another computer; back up the MetaMask owner separately. |

**There are two keys now.** In phase 1, the Agent wallet had one full private key; once unlocked, the backend could use it to control that wallet. In phase 2, the USDC sits in an onchain smart-wallet contract. Your MetaMask wallet is its owner. The backend holds a separate encrypted session key that can pay only within the contract's rules; it cannot change those rules or withdraw the USDC.

**You set the rules first.** Creating an Agent generates the session key. You use MetaMask to deploy the smart wallet, fund it with test USDC, and authorize allowed recipients, a per-payment limit, a daily limit, and an expiry. Deployment and authorization are separate MetaMask transactions. The session address also needs a little test ETH to pay gas when reserving a purchase.

**What happens during a purchase.** The app checks the quote, risk, and task budget. Once it chooses a provider, the session key registers the recipient and amount in the contract, reserving part of today's limit in an onchain transaction. Only then does it sign the x402 payment. At settlement, the contract accepts only a signature for that registered payment while the authorization is valid. **Reserved does not mean paid**: even if the purchase fails, that day's reservation is not refunded.

**Stopping or taking funds back.** `Lock now` stops new local signatures. `Revoke onchain` requires a MetaMask transaction and takes effect once mined. `Withdraw with MetaMask` returns remaining USDC to your MetaMask address. A stolen session key cannot directly withdraw funds, but it may still buy within the existing rules until revocation is mined.

**What to back up for another computer.** The Agent `.json` records the smart-wallet contract address, encrypted session key, and encrypted personal DeepSeek key. The downloadable keystore backs up **only the session key**, not the funds in the contract or your MetaMask wallet. To restore, you need the original password, contract address, and owner MetaMask wallet. Re-enter your personal DeepSeek key.

## How to use it now

| Situation | Action |
|---|---|
| New Agent | Prepare Base Sepolia test ETH in MetaMask → `Create agent + session key` → `Deploy with MetaMask` → `Fund with MetaMask` to send test USDC to the contract → `Fund session gas` to send a little test ETH to the session address → `Authorize with MetaMask` to set recipients, limits, and expiry → unlock and run a task. Deployment and authorization are separate transactions; each purchase also reserves its limit onchain. |
| New computer | Save the encrypted keystore, password, and smart-wallet contract address. Select the correct wallet type during restore; relink the existing contract for a smart wallet. The backup excludes the personal DeepSeek key. |
| Pause or exit | `Lock now` stops local new signatures. `Revoke onchain` invalidates the onchain grant once mined. `Withdraw with MetaMask` returns the smart-wallet balance to the owner. |

## Limits that matter

| Case | Fact |
|---|---|
| “Never leaks” | Passwords and keys briefly pass through page and server memory on input. Go cannot guarantee erasing every runtime copy. Do not put other secrets in task instructions; task instructions are saved. |
| Daily limit and gas | The limit resets at 00:00 UTC (08:00 China time). A failed purchase does not release its reservation that day. A previous-day signature that has not settled becomes invalid after the day changes. The session address can spend its own ETH, so fund it with only a small gas balance. |
| Unclear payment result | Save the reservation transaction hash first, then save a separate record before signing USDC. When USDC settlement is unknown, the existing app still blocks later payments to avoid paying twice. If deployment times out, check the transaction and link the deployed contract instead of deploying again. |
| Scope | This is an ERC-1271 smart wallet, without the full ERC-4337 flow. The backend still holds encrypted full private keys for old ordinary wallets and encrypted restricted keys for new wallets. |
| Verification | Contract, local x402 settlement, Go race, and simulated MetaMask UI tests have passed. A full payment using the user's MetaMask and a public facilitator on Base Sepolia remains untested; the contract has not had an independent audit.  |

## How to verify

| Phase | Existing checks |
|---|---|
| Phase 1 | Wrong passwords and other users or sessions cannot unlock; a restored backup keeps its address and can sign; locking before signing prevents payment. See [`vault_test.go`](../../../../../internal/agent/vault_test.go). |
| Phase 2 | Contract tests cover recipients, limits, revocation, and withdrawal. Anvil locally ran HTTP 402 → reservation → contract signature check → test-token transfer. Go tests and simulated MetaMask UI cover the main actions. See [`DecisionWallet.t.sol`](../../../../../contracts/test/DecisionWallet.t.sol) and [`smart_wallet_test.go`](../../../../../internal/agent/smart_wallet_test.go). |
| Run again | From the project root: `DECISION402_SMART_TEST=1 DECISION402_LIVE_TEST=0 go test -race ./...` and `npm --prefix web run build`; from `contracts`: `forge test`. Local-chain tests need Anvil. |

Code: phase 1 [`vault.go`](../../../../../internal/agent/vault.go) · [`wallet_api.go`](../../../../../internal/agent/wallet_api.go); phase 2 [`DecisionWallet.sol`](../../../../../contracts/src/DecisionWallet.sol) · [`smart_signer.go`](../../../../../internal/agent/smart_signer.go) · [`smart_wallet.go`](../../../../../internal/agent/smart_wallet.go).
