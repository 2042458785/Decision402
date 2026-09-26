# Agent 集成验证 / Agent Integration Validation

验证日期：2026-09-26，日本时间。以下区分真实观测和模拟测试，不代表生产级安全审计或获奖资格确认。

Checked on 2026-09-26 JST. Observations and simulated tests are distinguished; this is not a production security audit or award-eligibility confirmation.

## 真实观测 / Live observations

| 验证 / Check | 结果 / Result | 证据 / Evidence |
| --- | --- | --- |
| DeepSeek 账号模型列表 / Available model list | 返回 deepseek-flash、deepseek-v4-pro，配置前者 / Configured deepseek-flash | 官方 `/models` 实际鉴权成功 / Authenticated official API |
| 模型工具调用 / Model tools | 实际按 find_services → execute_purchase；Go 依据记录生成执行说明 / Actual tool calls; Go renders the factual report | 以下任务日志 / Task journals below |
| 模拟价格优先 / Simulated price-first | 选择 C，未付款 / Selected C without payment | `aac8dbca-4dbd-4779-966a-d137c24dc072` |
| 模拟风险优先 / Simulated risk-first | 页面执行，选择 D，未付款 / Executed via UI, selected D without payment | `d58f441d-13fe-4420-8823-757bb5270ea9` |
| 真实 API 预览 / Live preview | A/B 评分 100 被拒；C/D 评分 0，选择 C；未付款 / Rejected A/B, selected C, no payment | `55f1a32e-dfe9-410c-8327-9d893495d697` |
| 完整 Agent 付款 / Full agent payment | 单次与总预算 0.01，A/B 风险拒绝、D 超预算，C 付款成功 / 0.01 caps, risk excludes A/B, budget excludes D, C settled | `3353128f-ea1c-4bf4-bfd2-1558b365e552` |
| 重复提交同一已结算任务 / Replay settled task | 返回同一交易，事件数量不变，没有重跑 / Same transaction and event count, no execution replay | 本地 HTTP POST 重复检查 / Local HTTP replay check |
| 本轮真实 API 预览 / Current live preview | A/B 评分 100 阻断，C/D 评分 0，选 C；后端明确写出“真实 API 预览，未签名或付款” / A/B blocked, C selected, factual no-payment report | `e820e8b1-0666-4f79-8d57-a7783d882ed1` |
| 本轮完整测试网演示 / Current end-to-end testnet demo | A/B 阻断，C 签名前再次筛查后支付 0.01 测试 USDC 并结算 / A/B blocked, C re-screened and settled for 0.01 test USDC | `6ca181c6-3ad7-484f-a1e4-e783d0e87227` |

完整任务日志在 `artifacts/tasks/<id>.json`，默认不提交 Git。任务授权、候选报价、风险原始字段、决策和付款结果均可查看。

Full task journals are in `artifacts/tasks/<id>.json`, Git-ignored by default. They contain authorization, offers, risk fields, decisions, and payment results.

本次付款发现阶段只有两个不同收款地址，各真实扫描一次：风险样例 1421.98 ms，正常样例 467.11 ms；签名前对 C 再扫描一次，492.98 ms。A/B 与 C/D 分别共用地址，发现阶段复用同地址结果，最终签名前不复用。

Discovery scanned two distinct recipients once each: risky 1421.98 ms and normal 467.11 ms. C was scanned again before signing, taking 492.98 ms. Discovery reuses results for identical recipients; final signing performs a fresh call. These are individual samples, not latency guarantees.

## 独立链上核对 / Independent onchain verification

- Network: Base Sepolia (`eip155:84532`).
- Transaction: `0x9b9fdf1d62b838203e01ba27804d4a7d91f30a2755524d7b2ccb7f31c83c0a7c`.
- Receipt status: `0x1` (success).
- USDC token: `0x036CbD53842c5426634e7929541eC2318f3dCF7e`.
- Buyer: `0x4BF911B45Dc75E796a41367406f07c4FAD596171`.
- Recipient: `0x55eCFa861042bF2e5Ba9D1BDbF4C07D3C253B4a9`.
- Transfer amount: `0x2710` = 10000 atomic units = **0.01 test USDC**.
- Returned dataset: provider C, city Tokyo, 23°C, `sample_only=true`.

以上通过 Base Sepolia RPC 的独立 `cast receipt` 查询确认，不仅依赖服务端显示成功。

Verified independently with `cast receipt` against Base Sepolia RPC, rather than relying only on the application display.

[查看交易 / View transaction](https://sepolia.basescan.org/tx/0x9b9fdf1d62b838203e01ba27804d4a7d91f30a2755524d7b2ccb7f31c83c0a7c)

本轮更新后再次核对：交易 [`0x60d9…6f142`](https://sepolia.basescan.org/tx/0x60d9eacd911a31a9ba5bcc1e83fd5746f5f5279f09b3aca58f5a3060a8d6f142) 的 Base Sepolia RPC receipt `status=0x1`；USDC `Transfer` 日志显示买方 `0x4BF9…96171` 向配置的 C 收款地址 `0x55eC…B4a9` 转移 10000 atomic units，即 **0.01 测试 USDC**。页面和任务日志还记录了 `final_risk`、签名及结算；链上查询独立于应用回执。

After the update, independent Base Sepolia RPC verification of transaction [`0x60d9…6f142`](https://sepolia.basescan.org/tx/0x60d9eacd911a31a9ba5bcc1e83fd5746f5f5279f09b3aca58f5a3060a8d6f142) returned receipt `status=0x1`. The USDC `Transfer` log moved 10,000 atomic units (**0.01 test USDC**) from the buyer to C's configured recipient. The task journal also records `final_risk`, signing, and settlement.

## 本地检查 / Local checks

- `go test ./...`: passed.
- `go test -race ./internal/agent`: passed. macOS linker emitted an LC_DYSYMTAB warning, but the test exited successfully.
- `go vet ./...`: passed.
- `npm --prefix web run build`: Vue TypeScript checking and Vite build passed; TypeScript pinned to 5.9.3 for vue-tsc compatibility.
- `git diff --check`: passed.
- UI: simulation risk-first selected D; live preview displayed real scores; the successful payment task displayed C, a settlement receipt, and sample data.

## 仍有限制 / Remaining limits

- 本次增加了真实等级 1 的条件分档与签名前复查代码，并通过本地测试；**尚无已核实的轻微风险 API 样例或真实等级 1 付款证据**。两个旧样例不能证明生产风控覆盖率。 / Conditional live level-1 classification and the final signing gate now pass local tests; **no verified advisory API fixture or live level-1 payment has been observed**. The two earlier fixtures do not establish production coverage.
- 只支持东京静态天气目录，尚无外部服务发现和质量验证。 / Only a static Tokyo weather catalog; no external discovery or quality verification.
- 无 ZK、无智能合约策略验证、无多用户认证与钱包隔离。 / No ZK, contract-enforced policy, multi-user authentication, or wallet isolation.
- 自动防重试不能证明不存在所有旁路；本机操作者仍控制代码和私钥。 / Replay protection does not prove absence of all bypasses; the local operator controls code and keys.
- 授权预留后结算未知时暂停；没有自动对账恢复功能。 / Unknown settlement after reservation holds; there is no automatic reconciliation/resume.
