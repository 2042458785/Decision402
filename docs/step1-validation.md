# 第一步验证记录 / Step 1 Validation

2026-09-26：已实际调用 Intercepta Quick Scan Address，两个样例均返回 HTTP 200。

2026-09-26: both live Intercepta Quick Scan Address calls returned HTTP 200.

- Normal: `toxicScore=0`, `traits=[]`; 2636.62 ms.
- Risk: `toxicScore=100`, traits `known_scammer`, `attack_money_target`; 2278.02 ms.
- Local evidence: `artifacts/intercepta-20260926T063336Z-1570998528/`.

以上是单次响应证据。零分不代表绝对安全，风险标签是供应商的判断；未测量从拿到 key 到首次接通的开发耗时，也未证明所有链或所有风险均可识别。

These are individual observed responses. Zero does not guarantee safety, and risk labels are vendor assessments. Development time from receiving the key to the first successful call was not measured; coverage of all chains or risks was not established.

原始 probe 保留字段，不自动下结论。付款策略和后续验证见[第三步](step3-intercepta-gate.md)。

The raw probe records fields without interpreting verdicts. See [Step 3](step3-intercepta-gate.md) for payment policy and subsequent validation.
