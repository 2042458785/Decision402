# Wallet changes: current version

[中文](Wallet.md) · 2026-10-05

## Phase 1: local encryption for an ordinary Agent wallet

| Feature | Before phase 1 | What it does now | Why it helps |
|---|---|---|---|
| Wallet private key | The private key was stored as plaintext in a `.key` file. Anyone with its contents could use the wallet. | In phase 1, creating an ordinary Agent wallet generates a complete private key. Your password encrypts it inside the Agent `.json`; no plaintext key file or password is saved. | Copying the Agent file alone cannot spend the wallet's funds. |
| DeepSeek API key | A personal key was stored as plaintext in the Agent `.json`. | Your personal key is encrypted separately with the same password inside the Agent `.json`. If you do not enter one, the server may use its configured shared key. | Someone reading the Agent file cannot directly read your personal key. |
| Lock / unlock | Signing in did not leave a separate wallet lock or require a wallet password. | Agents start locked. Entering the password gives this login session temporary backend access for at most 10 minutes. Lock, logout, expiry, or restart ends it. | Signing in does not authorize payments indefinitely. |
| Signing and model calls | Payment signing read the plaintext `.key` file; model calls read the plaintext personal key. | The private key briefly exists as plaintext in backend memory when signing. The personal DeepSeek key briefly exists there when calling the model. The app makes a best effort to clear them afterward. | Plaintext keys are not kept in a file or held in memory between uses. |
| Backup / restore | There was no user-facing encrypted wallet backup and import. | The downloaded keystore backs up only the encrypted Agent-wallet private key. On another computer or in an installation with empty project data, import it with the original password to restore the same wallet address. Re-enter the name and personal DeepSeek key. | You can access the same wallet funds on another computer; this is not a backup of the whole Agent or its tasks. |
| Page, API, and logs | Ordinary APIs already returned only public wallet information, but the local file held a plaintext personal key and model replies were not checked for key echoes. | Password and key inputs are masked and cleared after use. Ordinary API responses and logs do not print plaintext keys. A model reply echoing the API key in use is discarded. | Reduces accidental display of secrets. |

Which files appear: MetaMask sign-in creates no Agent file. Creating an Agent writes `artifacts/agents/<Agent ID>.json` on the project computer. Its name, wallet address, and model settings are readable; its private key and personal DeepSeek key are encrypted. Submitting a task separately writes `artifacts/tasks/<Task ID>.json` with the instruction, budget, providers and quotes, execution, and payment result. Only when you request a backup does the browser download `decision402-<Agent ID>.keystore.json`. It backs up the wallet key, not the model key or tasks.

What happens during the 10 minutes: At unlock, the backend briefly uses the password to open the private key and personal DeepSeek key from disk. It then seals them again in memory with a new random key. It does not retain your password as the 10-minute credential. During that window, it briefly opens the needed plaintext when signing or calling the model, then makes a best effort to clear it. The backend can use both secrets during this window. Go cannot guarantee erasing every copy made by the runtime.

Encryption in plain terms: The password can decrypt the ciphertext back into the original value; an encrypted private key is not an incomplete key. Both disk secrets use go-ethereum V3 encryption separately with the same password. It derives a key with scrypt, encrypts with AES-128-CTR, and checks the password/ciphertext; this is not AES-256. Temporary in-memory sealing uses a new random key and AES-GCM.

Current limit: Encrypted files still live on the computer running the app, and the backend sees plaintext briefly when unlocked. This reduces the risk of copied files but is not yet a mature multi-user custody service.

## How to check it

| Check | Expected result |
|---|---|
| Create an Agent | The page shows one Agent-wallet address. Locally, there is `artifacts/agents/<Agent ID>.json` but no matching `.key` file. |
| Try a wrong password, then the right one | A wrong password cannot unlock. The right one unlocks for at most 10 minutes. Locking stops new signatures. |
| Download and restore the keystore | Import the backup into an installation with empty project data and enter the original password. The wallet address is unchanged; re-enter the name and personal DeepSeek key. One installation cannot register the same wallet to two Agents at once. |
| Preview and test a purchase | Go policy and risk checks still choose the provider. Preview sends no payment; pay mode signs x402 with the Agent wallet. |

## Added in this version: return test USDC to MetaMask

| Before | Now | Keep in mind |
|---|---|---|
| The page could fund and spend from an Agent wallet, but could not return its balance. | Sign in with MetaMask, unlock the Agent, enter an amount, and enter the wallet password again. Test USDC can go only to the signed-in MetaMask address. The page shows status and a transaction link. | The Agent wallet needs Base Sepolia test ETH for gas. The page can send it 0.001 test ETH from MetaMask. |

Before broadcast, the app saves the request ID and signed transaction under `artifacts/withdrawals/`. After a connection failure, check its status. “Resend same transaction” does not sign a new transfer. Another withdrawal is blocked while one is pending. **The backend still decrypts the ordinary wallet key to sign a withdrawal; this does not give the user independent wallet control.**

Try it yourself: send a little test USDC and test ETH to the Agent, unlock it, and withdraw a small amount. Check the recipient, amount, and success status on the page and BaseScan. A wrong password should be rejected; resending the same transaction should keep the same hash.

## Phase 2: user control (deferred from this version)

Study and build a MetaMask-owned smart wallet, limited Agent authorization, revocation, and a design in which the backend does not hold the full wallet key.

## Phase 3: wallet rules and records (deferred from this version)

Design enforceable cumulative spending limits and recipient allowlists with the smart-wallet approach. Transfers are visible onchain; task details, offers, and risk checks still need application records.

---

## Next wallet version

| Work | Success check |
|---|---|
| Test smart-wallet x402 compatibility | On Base Sepolia, complete one real offer, signature, and settlement using a smart wallet. |
| Give the user control | MetaMask can revoke Agent authorization and withdraw independently; the backend can spend only within the user's grant. |
| Decide how to enforce limits | Measure whether a strict onchain daily total adds an onchain transaction to each small purchase before choosing the design. |

This version still uses ordinary Agent wallets and has no cross-task daily limit. It is not ready to hold real funds for unfamiliar users. The separate `cmd/x402-demo` does not use Agent wallets created in the UI.
