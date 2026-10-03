# Wallet: Current State and Improvement Plan

[中文](Wallet.md)

Sources checked: 2026-10-03. This document records how Decision402 handles the wallet today (sign-in, agent creation, funding, payment signing) and the direction for rebuilding it ourselves.

## How the wallet works today

In one line: **connect MetaMask to prove identity → the backend generates a new private key and writes it to disk when you create an agent → you fund it by sending USDC directly from MetaMask → at payment time the backend signs for the agent using the key on disk.**

### Key files and locations

| What | Location | Permissions / encryption |
|---|---|---|
| Agent wallet private key | `artifacts/agents/<id>.key` | 0600, plaintext hex, not encrypted |
| Agent metadata (owner, wallet address, DeepSeek key) | `artifacts/agents/<id>.json` | 0600, plaintext |
| Default payment key (used without an agent) | `.buyer-key` | plaintext |
| Demo seller key | `.seller-key` | plaintext |
| DeepSeek + Intercepta keys | `.env` | plaintext |

### Who does what at each step

| Step | Who acts | Notes |
|---|---|---|
| Sign-in | User signs a message with MetaMask | Proves identity only; no transaction |
| Create agent | Backend generates an ECDSA key and writes it to disk | The backend holds the key |
| Fund | User sends USDC directly from MetaMask to the agent address | The platform does not touch funding |
| Check balance | Frontend reads the USDC contract `balanceOf` via `eth_call` | Read onchain; the platform does not store it |
| List agents | `GET /api/agents` filtered by owner | Already implemented |
| Payment signing | Backend reads `.key` and signs the x402 authorization | The user does not participate in signing |

Code: [sign-in and agent creation](../../../../../internal/agent/owner.go), [task and payment entry](../../../../../internal/agent/app.go), [payment signing](../../../../../internal/agent/payment.go), [frontend connection](../../../../../web/src/App.vue).

## Current problems

### Key custody (most serious)

1. Private keys are stored in plaintext in `.key`; only 0600 permissions protect them, with no encryption. Anyone who can read the file controls the wallet.
2. The DeepSeek API key is stored in plaintext in `.json`.
3. `.buyer-key`, `.seller-key`, and `.env` are all plaintext.
4. No passphrase and no lock/unlock.

### Control: whose money, who signs

5. Payment is signed by the backend on the agent's behalf; the user's MetaMask is only an identity, not the controller.
6. The agent wallet only supports funding and payment; there is no withdraw, so the user cannot move funds back to themselves.
7. The key lives on the server's disk, not onchain, so the user cannot prove "only I control it."

### Custody and recovery

8. Centralized custody: keys are held by the platform, not self-custody or a smart account.
9. If the machine is lost or the files are deleted, the wallet is permanently lost; there is no recovery.
10. A `.lock` file enforces a single instance, so multi-user and multi-instance are unsupported.

### Spending limits

11. Only per-payment and per-task caps exist; there is no daily cumulative limit or recipient allowlist. Rules are judged in backend memory, not onchain, and are not verifiable.

### Minor

12. Sign-in sessions live in memory, so a restart logs everyone out; sessions expire after one hour.
13. Balances are read live by the frontend via `eth_call`; the backend keeps no asset record or history.

## Planned improvements (study Circle, build ourselves)

### Phase 1: stop the bleeding (do first)

1. Use [go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore) to store keys passphrase-encrypted; the user enters the passphrase and the platform does not store it.
2. Encrypt the DeepSeek API key, or keep it only in environment variables, not plaintext `.json`.
3. Add lock/unlock: refuse signing while locked; decrypt briefly into memory only for signing and clear it afterwards.
4. Backup and recovery: export an encrypted keystore, restore with the passphrase, and test the path.
5. Never expose a private key, passphrase, or API key in logs, API responses, or the frontend.

### Phase 2: user control (learn Circle's "agents never see the shares")

6. Make the agent wallet a smart account (ERC-4337 / smart wallet); the user signs one authorization with MetaMask that sets a recipient allowlist + per-payment/daily limits + expiry.
7. The AI signs within the authorization and the user can revoke it anytime; the backend no longer holds a full private key.
8. Add withdraw: with user authorization, move the agent wallet balance back to the user's address.

### Phase 3: onchain verifiability + experience

9. Put spending limits (per-payment/daily/weekly + recipient allowlist) onchain, or make them verifiable onchain.
10. Have the backend return "agent list + balance" in one response, replacing the frontend's ad-hoc `eth_call` queries.
11. Auto-load agents and balances on reconnect (already implemented; keep and refine).

### Definition of done

- Wrong passphrase, locked state, or user A acting on user B's agent all fail to sign or pay.
- Backups restore; logs and responses contain no sensitive data.
- Payment no longer uses a plaintext key held by the backend; the user can see and revoke the authorization.

## Circle comparison (what we learn)

| Circle's approach | What we adopt |
|---|---|
| 2-of-2 MPC; agents never see the shares | Do not build MPC in the first version; use a smart account + session key for a similar effect: the user keeps control and the agent never holds the full key |
| Limits: per-payment ≤ daily ≤ weekly ≤ monthly + allowlist | Start with per-payment + daily + allowlist; add weekly/monthly later |
| User retains custody | Goal: the user's MetaMask controls and can revoke the agent wallet |
