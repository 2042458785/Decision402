# 第三步：付款前真实风险检查 / Step 3: Live Screening Before Payment

后续进度 / Later progress: Agent 版本已完成真实筛查与测试网结算，见 [Step 4 validation](step4-validation.md)。以下保留第三步接入时的验证范围。

## 做了什么 / What changed

```text
HTTP 402 报价 / quote
 → 检查网络、USDC、金额、预期收款人 / validate quote
 → Intercepta 扫描报价中的同一个 payTo / scan exact payTo
 → allow：签名并尝试结算 / sign and attempt settlement
 → deny 或 hold：停止，不生成支付签名 / stop without a payment signature
```

风险数据来自主网；支付网络是 Base Sepolia。没有查询 Base Sepolia 风险历史，也没有跨链桥。

Risk data comes from mainnet; payments use Base Sepolia. This does not query Base Sepolia risk history or use a cross-chain bridge.

## 决策规则 / Decision policy

| 返回情况 / API result | 本项目动作 / Project action |
| --- | --- |
| `toxicScore=0` 且 `traits=[]` / and empty traits | `allow`：报价检查也必须通过；不是安全保证 / Quote checks must also pass; not a safety guarantee |
| 分数大于 0 或有完整风险标签 / Positive score or complete risk traits | `deny`：不签名 / No signature |
| 超时、非成功状态码、字段缺失或格式错误 / Timeout, non-success status, missing or malformed fields | `hold`：本次停止，人工排查 / Stop this attempt for review |

这是我们选择的保守 Demo 策略，**不是 Intercepta 官方阈值或官方返回的 allow/deny**。`hold` 暂未实现审批后恢复。每次 pay 都重新扫描，不缓存放行结果。

This conservative demo policy is ours, **not an official Intercepta threshold or allow/deny response**. `hold` does not yet support resuming after human approval. Every pay attempt scans again, without cached approvals.

`toxicScore` 是供应商返回的风险评分；`traits` 提供 `risk`、`name`、`description`。终端 JSON 同时显示 HTTP 状态、地址、评分、原因、耗时和支付网络。

The provider returns a `toxicScore` risk score and `traits` with `risk`, `name`, and `description`. Terminal JSON includes HTTP status, address, score, reasons, elapsed time, and payment network.

## 操作 / Steps

先按 README 跑正常付款。风险路径可先用不付款的测试验证：

First follow the README for the normal payment. Verify the risk path without paying:

```sh
DECISION402_LIVE_TEST=1 go test ./cmd/x402-demo -run '^TestLiveInterceptaGate$/risk$' -v -count=1
```

预期 `deny`、`payload_spy_calls=0`、`PASS`。只有真实 API 返回拒绝才算通过；超时产生的 hold 不算风险样例测试成功。

Expect `deny`, `payload_spy_calls=0`, and `PASS`. Only a real rejection passes this fixture test; a timeout producing hold does not count.

需要展示完整的本地报价→风险阻断路径时：终端 1 用 Ctrl+C 停掉原服务，再启动风险报价服务：

To demonstrate the local quote-to-block flow, stop the existing seller with Ctrl+C in terminal 1, then start the risk-offer server:

```sh
go run ./cmd/x402-demo -mode serve -pay-to 0x39308ae43e5dda98db5fb17d005c5c764e5a2fed
```

终端 2：

Terminal 2:

```sh
go run ./cmd/x402-demo -mode pay -pay-to 0x39308ae43e5dda98db5fb17d005c5c764e5a2fed
```

预期输出 `deny`、风险标签和错误退出；没有签名、结算或交易哈希。此地址是供应商网站提供的风险样例，不是你的收款钱包。无需、也不要获取它的私钥或给它充值。先确认上面的只读风险测试通过；这条 pay 命令不是 dry-run，若未来供应商结果变成零风险，当前策略会允许测试网付款。

Expect `deny`, risk traits, and a nonzero exit; no signature, settlement, or transaction hash. This address is the vendor dashboard's risk fixture, not your receiving wallet. No private key or funding for it is needed. Confirm the non-paying risk test first. The pay command is not a dry run: if the provider later returns zero risk, the current policy would permit a testnet payment.

## 已验证与未验证 / Verified and pending

2026-09-26 JST：真实 API + SDK 钩子测试通过，证据保存在本地 `artifacts/intercepta-gate-live.txt`。

2026-09-26 JST: the live API + SDK hook test passed; local evidence is in `artifacts/intercepta-gate-live.txt`.

| 样例 / Fixture | HTTP | toxicScore | 策略 / Policy | 单次耗时 / Single-call duration |
| --- | --- | --- | --- | --- |
| normal | 200 | 0 | allow | 1822.68 ms |
| risk | 200 | 100 | deny | 3145.75 ms |

风险标签为 `known_scammer`、`attack_money_target`。这是供应商对样例的标记。耗时只有两个样本，不代表 SLA。测试以替代模块观察是否进入授权生成，未签名、未转账。

Risk traits were `known_scammer` and `attack_money_target`, as attributed by the vendor. These two latency samples are not an SLA. A replacement module observed entry into authorization creation; no signatures or transfers occurred.

原 CLI 的完整 HTTP→签名→结算未在此第三步记录里重测。后续 Agent 版本已经完成真实筛查、测试网付款和前端展示，见[第四步验证记录](step4-validation.md)。ZK 尚未实现。当前只筛查收款地址；代币通过固定链和合约地址校验，未调用 Intercepta 的 token 或签名扫描接口。

The standalone CLI flow was not rerun in this Step 3 record. The subsequent Agent version completed live screening, testnet payment, and UI display; see [Step 4 validation](step4-validation.md). ZK is not implemented. Live screening covers the recipient; the token is checked against fixed network/contract constants, without Intercepta token or signature scans.
