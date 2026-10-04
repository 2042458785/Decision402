# Wallet: phase 1 changes

[中文](Wallet.md) · 2026-10-05

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
