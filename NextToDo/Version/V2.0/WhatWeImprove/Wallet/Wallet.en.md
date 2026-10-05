# Wallet changes: phase 1

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

Only ordinary Agent wallets are supported now. There is no smart wallet, onchain daily limit, or UI action to return funds to MetaMask. Spending from separate tasks is not added into a daily total. Phase 1 does not establish that the app is ready to hold real funds for unfamiliar users. The separate `cmd/x402-demo` remains an older CLI demo and does not use Agent wallets created in the UI.
