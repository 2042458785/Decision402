# Wallet: phases 1 and 2

[中文](Wallet.md) · 2026-10-05

## Phase 2: owner control

| Change | Behavior |
|---|---|
| Smart wallet | New agents use an ERC-1271 contract wallet owned by MetaMask. ERC-4337 EntryPoint and bundlers are not included. |
| One authorization | The owner signs one MetaMask transaction to set recipients, per-payment and daily limits, and expiry. Initial deployment is a separate transaction. |
| Automatic payment | The backend stores only an encrypted restricted session key. The model cannot access it; the owner key stays in MetaMask. Quotes, risk, and task budgets are still checked. |
| Daily budget | Reserve the amount onchain before signing an x402 payment. Tasks share the same daily budget; replacing an authorization does not reset it. |
| Revocation | `Revoke onchain` is signed by MetaMask. Once mined, unsettled signatures under the previous authorization become invalid. Completed payments cannot be reversed. |
| Withdrawal | `Withdraw with MetaMask` returns USDC to the owner. The session key cannot withdraw or change the withdrawal recipient. |
| Backend offline | Save `Save standalone owner controls` and the contract address. This standalone page uses MetaMask to revoke or withdraw without the project backend. |
| Backup | The keystore backs up the session key only. On another computer, restore as `Smart wallet session key`, then link the original contract address. Back up MetaMask separately. |
| Existing wallets | Existing EOA addresses, balances, and encrypted files stay intact. Create a new smart wallet and move funds yourself; old wallets retain phase-one custody. |

| Step | Action |
|---|---|
| 1. Create | Prepare Base Sepolia test ETH in MetaMask → connect → enter agent details and encryption password → `Create agent + session key`. |
| 2. Deploy | Select `Deploy with MetaMask` and confirm the Base Sepolia transaction. The app verifies contract code, owner, and USDC before linking. Use `Link existing wallet` for an existing deployment. |
| 3. Fund | Send test USDC to the smart wallet using `Fund with MetaMask`. Use `Fund session gas` to send a small amount of test ETH to the session address for reservations. Do not send USDC to the session address. |
| 4. Authorize | Enter merchant recipients, payment/daily caps, and expiry → `Authorize with MetaMask` → wait for mining. UI limits: 0.10 USDC per payment, 100 USDC per day, expiry within 30 days. |
| 5. Buy | Unlock the session key with its password for up to 10 minutes, then submit a payment task. Both the onchain authorization and local unlock must be active. |
| 6. Stop / withdraw | `Lock now` stops new signatures; `Revoke onchain` invalidates the authorization; `Withdraw with MetaMask` returns USDC. |
| Deployment timeout | Check the transaction first. If it succeeded, copy its contract address and link it instead of deploying again. |
| Backend RPC | Defaults to `https://sepolia.base.org`. Set process environment variable `DECISION402_RPC_URL` to your own Base Sepolia RPC if needed. Wrong-chain operations are rejected. |

| Limit | Detail |
|---|---|
| Day boundary | Daily budgets reset at 00:00 UTC (08:00 China time). Unsettled signatures reserved on the previous day become invalid. |
| Failed purchases and fees | Reservations count for the rest of the day even if the purchase fails. Each purchase adds an onchain transaction paid in test ETH. The session key controls ETH at its own address, so fund only a small gas balance. |
| Interrupted payments | Save the reservation hash before broadcasting; save the payment attempt separately before signing USDC. Insufficient gas or a failed reservation journal is not marked as a signed payment. An unknown USDC settlement still blocks further payments. |
| Difference from Circle | Circle uses MPC key shares. This implementation uses a MetaMask owner plus a restricted session key, not Circle MPC. [Circle documentation](https://developers.circle.com/agent-stack/agent-wallets) |
| Tested scope | Local contract and x402 settlement tests passed. No deployment with the user's MetaMask on public Base Sepolia, or complete public-facilitator payment, has been verified. Test use only; no independent contract audit. |

| Check | Result |
|---|---|
| Contracts | Covers recipients, per-payment/daily caps, expiry, day rollover, replay, reauthorization without resetting spend, revoking old signatures, and owner-only withdrawal. |
| Local x402 settlement | Anvil: HTTP 402 → onchain reservation → contract signature verification → test-token transfer → settlement response. Passed. |
| Backend | Full Go tests and race checks passed, including session backup/restore, unauthorized requests, wrong-chain rejection, and existing wallet regression tests. |
| Frontend | Build passed. Simulated MetaMask and Anvil UI checks covered creation, deployment, linking, funding, authorization, revocation, withdrawal, and standalone-page withdrawal. No real wallet was used. |

Code: [`DecisionWallet.sol`](../../../../../contracts/src/DecisionWallet.sol) · [`smart_wallet.go`](../../../../../internal/agent/smart_wallet.go) · [`smart_signer.go`](../../../../../internal/agent/smart_signer.go) · [`smart_wallet_test.go`](../../../../../internal/agent/smart_wallet_test.go)

To repeat: run `npm ci --ignore-scripts` and `forge test` inside `contracts`; from the project root run `DECISION402_SMART_TEST=1 DECISION402_LIVE_TEST=0 go test -race ./...` and `npm --prefix web run build`. Local-chain tests require Anvil. Contracts use Solidity 0.8.37 and OpenZeppelin 5.4.0. After changing the contract, run `python3 contracts/scripts/export.py` to update the embedded ABI and bytecode.

## Phase 1: encryption and locking (legacy EOA wallets)

| Change | Current behavior |
|---|---|
| Private-key encryption | go-ethereum V3 keystore encrypts the wallet with the user's password. Files remain `0600`. The password is not retained. |
| DeepSeek key | Personal keys are encrypted with the same password, rather than stored as plaintext in Agent JSON. The shared key comes only from the process environment variable `DEEPSEEK_API_KEY`. |
| Lock / unlock | Locked by default. Unlock lasts up to 10 minutes and belongs to the current login session. Manual lock, logout, expiry, or restart blocks further signing. |
| Signing | Quote, budget, and risk checks must pass first. The private key is decrypted briefly for signing, then cleared. Locking also stops tasks that have started but have not signed yet. |
| Backup / restore | Enter the password to export an encrypted keystore. Restore the same wallet address using that file and password. The backup excludes the model key; enter it again when restoring. |
| Legacy migration | Choose `Encrypt old wallet` and enter the new password twice. Encryption is verified before saving and removing the old `.key`. The address and personal model key are preserved. Interrupted migration can be resumed. |
| Secret handling | APIs return public metadata only. Password/key fields are masked, cleared after the operation, and excluded from browser storage. Model responses containing the API key are discarded. |

| Use case | Steps |
|---|---|
| New Agent | Connect wallet → enter settings and password → create → fund with test tokens → unlock → run a task. |
| Existing Agent | Choose `Encrypt old wallet` first. Tasks are blocked until migration finishes. Updating the code alone does not encrypt existing files. |
| New computer | Download the encrypted backup and keep its password separately → connect on the new computer → select the backup → enter the original password and model settings → `Restore wallet`. Lost passwords cannot be recovered. |
| DeepSeek key previously in `.env` | Enter a personal key in the UI or configure the process environment variable. Remove the old value from `.env` yourself. Other `.env` settings still work. |

| Check | Result |
|---|---|
| Backend tests | `DECISION402_LIVE_TEST=0 go test ./...` passed using local mock services, with no real payments. |
| Race tests | `DECISION402_LIVE_TEST=0 go test -race ./internal/agent` passed. Includes locking during signing and cancelling a pending unlock. |
| Recovery and access | Wrong passwords, other users, and other login sessions are rejected. Restored backups can sign. Restart locks wallets. Interrupted migration can resume. |
| Payment checks | Changed quotes, unacceptable risk, journal failure, and locking before signing prevent payment. The normal path still passes the x402 mock-seller test. |
| UI | Build passed. Local test-wallet checks covered creation, wrong passwords, unlock, lock, cleared inputs, and button states. Backup APIs and restore passed tests; saving the browser download to disk was not confirmed in the in-app browser. |

| Detail | Limit |
|---|---|
| While unlocked | Server memory retains temporary decryption capability, not the user's password. Locking clears that capability. Go cannot guarantee erasing every copy created by the runtime or dependencies. |
| Payments already signed | Locking cannot cancel them; their payment results still need checking. |
| Legacy CLI | The standalone `cmd/x402-demo` remains the earlier demo. Its `.buyer-key` / `.seller-key` files are not migrated into Agent wallets. The new main app does not read those files. |

Code: [`vault.go`](../../../../../internal/agent/vault.go) · [`wallet_api.go`](../../../../../internal/agent/wallet_api.go) · [`vault_test.go`](../../../../../internal/agent/vault_test.go) · [`App.vue`](../../../../../web/src/App.vue)
