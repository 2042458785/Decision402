# Wallet changes: phases 1 and 2

[中文](Wallet.md) · 2026-10-05

## Phase 1: encrypt local keys

| Feature | Before | Change | Why | Practical benefit |
|---|---|---|---|---|
| Wallet private key | An Agent's plaintext key was in a `.key` file. Its permissions were `0600`, but anyone with the file contents could control the wallet. | Store a go-ethereum encrypted keystore in the Agent record. The user enters a password; the password is not saved to disk. | File permissions cannot protect a copied file. | Someone who obtains the Agent file alone cannot immediately spend its funds. |
| DeepSeek key | Personal keys were plaintext in the Agent `.json`; a shared key could be read from `.env`. | Encrypt personal keys with the wallet password. Read the shared key only from the process environment variable `DEEPSEEK_API_KEY`. | Agent and configuration files can be copied. | Moving Agent files no longer moves a plaintext model key with them. Old values in `.env` must be removed manually. |
| Lock / unlock | Signing had no separate Agent wallet lock after login. | New wallets start locked. A password unlocks signing for the current login session for at most 10 minutes. Lock, logout, expiry, and restart require another unlock. | Being signed in should not grant indefinite signing access. | Stop new signatures as soon as a task is done; an unopened wallet page cannot simply pay. |
| Key use while signing | The backend read a plaintext key file and passed the key to the payment signer. | After quote, budget, and risk checks, decrypt only for signing and make a best effort to clear the key afterward. | Reduce the time a plaintext key is present in memory. | Keep automatic payments while reducing long-lived key exposure. |
| Backup / restore | There was no user-facing encrypted backup and restore flow. | Export an encrypted keystore with the password; restore the same address with that file and password. Tests cover signing after restore. | Moving computers should not require copying a plaintext key. | Continue using the original wallet address. The backup excludes the model key; enter it again after restore. |
| Existing wallets | Updating the app did not change old `.key` files. | `Encrypt old wallet` verifies the encrypted replacement before saving it and removing the `.key` at its original location. | A failed migration must not strand an existing wallet. | Keep the wallet address and personal model key. Other copies of an old plaintext file still need handling. |
| Page, API, and logs | The old API already returned public wallet fields, but local JSON contained a plaintext key and model replies lacked a check for key echoes. | Mask and clear password/key inputs; keep plaintext keys out of ordinary API responses and logs; discard model replies containing the configured API key. | Reduce accidental display of secrets. | Reviewing task results or errors is less likely to expose the configured key. |

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
| Old wallets | Existing encrypted ordinary wallets kept their original addresses and balances. | Keep them as they are. Create a new smart wallet and move funds yourself to use the new controls. | This app does not convert an old address into the new contract address. | Old wallets are not mistakenly shown as protected by onchain rules. |

## How to use it now

| Situation | Action |
|---|---|
| New Agent | Prepare Base Sepolia test ETH in MetaMask → `Create agent + session key` → `Deploy with MetaMask` → `Fund with MetaMask` to send test USDC to the contract → `Fund session gas` to send a little test ETH to the session address → `Authorize with MetaMask` to set recipients, limits, and expiry → unlock and run a task. Deployment and authorization are separate transactions; each purchase also reserves its limit onchain. |
| Existing old Agent | First use `Encrypt old wallet` to complete phase 1 migration. For phase 2 controls, create a smart wallet and move the funds yourself. |
| New computer | Save the encrypted keystore, password, and smart-wallet contract address. Select the correct wallet type during restore; relink the existing contract for a smart wallet. The backup excludes the personal DeepSeek key. |
| Pause or exit | `Lock now` stops local new signatures. `Revoke onchain` invalidates the onchain grant once mined. `Withdraw with MetaMask` returns the smart-wallet balance to the owner. |

## Limits that matter

| Case | Fact |
|---|---|
| “Never leaks” | Passwords and keys briefly pass through page and server memory on input. Go cannot guarantee erasing every runtime copy. Do not put other secrets in task instructions; task instructions are saved. |
| Daily limit and gas | The limit resets at 00:00 UTC (08:00 China time). A failed purchase does not release its reservation that day. A previous-day signature that has not settled becomes invalid after the day changes. The session address can spend its own ETH, so fund it with only a small gas balance. |
| Unclear payment result | Save the reservation transaction hash first, then save a separate record before signing USDC. When USDC settlement is unknown, the existing app still blocks later payments to avoid paying twice. If deployment times out, check the transaction and link the deployed contract instead of deploying again. |
| Scope | This is an ERC-1271 smart wallet, without the full ERC-4337 flow. The backend still holds encrypted full private keys for old ordinary wallets and encrypted restricted keys for new wallets. |
| Verification | Contract, local x402 settlement, Go race, and simulated MetaMask UI tests have passed. A full payment using the user's MetaMask and a public facilitator on Base Sepolia remains untested; the contract has not had an independent audit. Demo keys in the old `cmd/x402-demo` were not migrated into Agent wallets. |

## How to verify

| Phase | Existing checks |
|---|---|
| Phase 1 | Wrong passwords and other users or sessions cannot unlock; a restored backup keeps its address and can sign; interrupted migration can resume; locking before signing prevents payment. See [`vault_test.go`](../../../../../internal/agent/vault_test.go). |
| Phase 2 | Contract tests cover recipients, limits, revocation, and withdrawal. Anvil locally ran HTTP 402 → reservation → contract signature check → test-token transfer. Go tests and simulated MetaMask UI cover the main actions. See [`DecisionWallet.t.sol`](../../../../../contracts/test/DecisionWallet.t.sol) and [`smart_wallet_test.go`](../../../../../internal/agent/smart_wallet_test.go). |
| Run again | From the project root: `DECISION402_SMART_TEST=1 DECISION402_LIVE_TEST=0 go test -race ./...` and `npm --prefix web run build`; from `contracts`: `forge test`. Local-chain tests need Anvil. |

Code: phase 1 [`vault.go`](../../../../../internal/agent/vault.go) · [`wallet_api.go`](../../../../../internal/agent/wallet_api.go); phase 2 [`DecisionWallet.sol`](../../../../../contracts/src/DecisionWallet.sol) · [`smart_signer.go`](../../../../../internal/agent/smart_signer.go) · [`smart_wallet.go`](../../../../../internal/agent/smart_wallet.go).
