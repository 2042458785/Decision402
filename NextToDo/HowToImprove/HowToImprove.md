# Decision402：自己实现，参考 Circle 的代码

更新：2026-10-02。本文是待办计划，尚未实施。[English](HowToImprove.en.md)

## 先把方向说清楚

**我们的功能自己写、自己运行。Circle 只作为学习资料，不接它的托管钱包、CLI、Marketplace 搜索或 Gateway 支付服务。**

保留现有 Go、Vue、x402 和 go-ethereum 开源库；业务自己写，签名和加密不另造算法。USDC 和 Arc 仍按现有项目目标保留。现有模型、RPC 和风险数据服务也是外部依赖，本次不把它们一起重写。

顺序：**保留能工作的版本 → 找两家真实服务 → 改钱包保管 → 补预算 → 补查账恢复 → 完善自己的付款流程 → 验收数据和换商家 → 用户试用。**

不是先学完 Circle 再动手。每次只读下一步需要的代码，写出自己的实现并测试。阅读时分清：哪些是真正的实现，哪些只是调用 Circle 后台。照着调用代码翻译成 Go，并不能得到那个后台。

| 之前的缺点 | 对应步骤 |
|---|---|
| 没有真实采购用途 | 第 1 步找需求和服务，第 6 步完成真实购买。 |
| 本地文件保管私钥 | 第 2 步。 |
| 预算不跨任务累计 | 第 3 步。 |
| 付款未知后不能恢复、阻塞其他任务 | 第 4 步。 |
| 付了钱不代表数据有用 | 第 6 步。 |
| 与 Circle 重复，用户价值不清楚 | 第 7 步比较效果；自己写不等于用户一定需要。 |

## 0. 保留现在能工作的版本

- [ ] 保存当前修改并记录 Git 提交号，密钥、`.env` 和模型 Key 不进入 Git。
- [ ] 在项目根目录运行以下检查，记录现有失败，先查清原因。

```sh
DECISION402_LIVE_TEST=0 go test ./...
npm --prefix web run build
```

**完成标准：** 留下原版本及检查结果。后续每步在 `docs/improvement-log.md` 记录“参考了什么、改了什么、测试结果”（实施时新建，不放凭据）。

## 1. 自己做服务列表，先放进两家真实商家

**先读：** [services.ts](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts) 的 `mapSearchItem`、`preferredAccept`。学习服务信息包含什么、怎样选择支付链。`searchServices` 调用的是 Circle CLI，不照搬，也不安装 CLI。

**动手顺序：**

1. 找一位有实际采购需求的开发者，写下“买什么、做什么用、多久买一次”。不用等访谈满 5 人才开发。
2. 从商家官网或公开文档找两家同类 API。优先选择直接支持 x402 的服务。记录网址、请求方法、参数、返回字段、数据时间、价格和支付链；用免费／测试请求取样，暂不付真钱。
3. 新增 `config/providers.json` 和 `internal/agent/providers.go`，自己加载、筛选这两家。修改 [app.go](../../internal/agent/app.go) 的 `NewApp`，不再把 A/B/C/D 当真实供应商。模拟服务与真实服务分开，`SellerHandler` 仍只处理自己的演示路由。
4. 扩展 [policy.go](../../internal/agent/policy.go) 的 `Service`，保存请求方法和参数。修改 [payment.go](../../internal/agent/payment.go) 的 `Quote`，自己请求商家的 HTTP 402 报价，核对金额、币种、链、收款人。多种支付选项只选支持的那一种，不盲取第一项。
5. 换了用途，就同时改 [model.go](../../internal/agent/model.go) 的 `find_services` 和 [app.go](../../internal/agent/app.go) 中写死的东京天气说明。模型只能提出任务，网址、预算和收款范围由我们的代码限制。

- [ ] **完成标准：** 不安装 Circle CLI、不调用它的搜索接口，`preview` 仍能显示两家候选、报价和风险结果。风险未知或支付方式不支持时不付款。两天找不到合适候选，就换用途，先不做完整搜索平台。

## 2. 自己做钱包管理，先解决明文密钥和签名权限

**先读：** [Modular Wallets SDK](https://github.com/circlefin/modularwallets-web-sdk) 的示例和测试，学习钱包操作怎样与页面分开；[钱包合约](https://github.com/circlefin/buidl-wallet-contracts/tree/master/src/msca)了解权限检查放在哪里。它们不是 Circle 的完整 MPC 钱包后台。

**动手顺序：**

1. 第一版继续使用普通 EVM 钱包，先不重写 MPC 或智能钱包合约。新增 `internal/agent/wallet.go`，集中处理创建、查余额、锁定／解锁和签名；页面和模型拿不到私钥。
2. 参考并使用现有依赖中的 [go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore)，新增 `internal/agent/keystore.go` 管理加密后的密钥。解锁口令不与密钥文件一起保存，不进入模型、日志或前端。
3. 改 [owner.go](../../internal/agent/owner.go)：保留用户与钱包对应关系，把明文 `.key` 写入改为加密存储。先用新测试钱包验证；旧钱包先备份并验证恢复，再迁移，不能直接覆盖。
4. 改 [payment.go](../../internal/agent/payment.go)：只通过我们自己的钱包模块签当前已批准的付款；不向模型开放“任意内容签名”。修改额度、解锁、暂停必须检查用户身份。
5. 明确暂停的行为：停止新签名，已签名的继续查账。模型 Key 也从普通元数据中分离，限制读取权限。

- [ ] **完成标准：** 测错误口令、锁定后付款、暂停、用户 A 操作 B 钱包、备份恢复和日志泄露。文件加密通过不代表可以公开托管多人资金；这一轮只做受控测试和小额试用。

## 3. 自己写累计预算，不依赖 Circle 的限额服务

**先读：** [额度说明](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies)和 [agent-wallet-policy](https://github.com/circlefin/skills/blob/master/plugins/circle/skills/agent-wallet-policy/SKILL.md)。只学习额度种类和用户确认流程。文档不是后台账本源码，下面的实现由我们完成。

**动手顺序：**

1. 在 [policy.go](../../internal/agent/policy.go) 增加 Agent 每日额度，同钱包共用总上限；保留单笔、任务额度。第一版先不加周、月额度。
2. 新增 `internal/agent/ledger.go`，先用 SQLite。保存任务、Agent、钱包、整数金额、付款编号、授权标识、时间和状态。
3. 接到 [payment.go](../../internal/agent/payment.go) 的 `beforeSign`：同一次数据库事务里检查额度并占用；数据库写入失败就不签名。每个签名授权的完整参数必须在发送前保存。
4. 已付记为花费；未知继续占额度；确认未支付且原授权不能再扣款才释放。重复更新不能多扣或多退；换商家也使用同一任务预算。
5. 按北京时间记每日花费，跨午夜未知付款仍占额度。API 预算、模型费和链上手续费分开显示。

- [ ] **完成标准：** 模拟一天额度 0.10 USDC，三笔并发各 0.04，最多放行两笔。同一天重启、换任务或新建同钱包 Agent，第三笔仍被拦住。另测午夜、写入失败及签名过程崩溃。测试金额不是主网授权。

## 4. 自己查账，解决超时和重启后的恢复

**先读：** [Circle Facilitator 教程 Step 3–5](https://developers.circle.com/facilitator-service/quickstart)，学习固定付款编号、结果未知、再次查询的处理方式；不调用它的服务。实际代码继续基于项目已有的 [x402 Go](https://github.com/x402-foundation/x402/tree/main/go)。

**动手顺序：**

1. 扩展 [app.go](../../internal/agent/app.go) 的 `Task` 和 [payment.go](../../internal/agent/payment.go) 的 `PaymentOutcome`，保存付款编号、授权 nonce、有效期、金额和双方地址。重复提交返回原任务，改金额或收款人则拒绝。
2. 新增 `internal/agent/reconcile.go`，通过目标链 RPC 查询交易回执、授权状态和事件；没有交易哈希时，按已保存的授权标识和区块范围查找。核对实际转账的链、币种、双方地址及金额，按该链确认规则记账。
3. “没查到交易”“RPC 超时”“授权已使用”都不能单独推导出付款结果；需要匹配到付款或撤销证据。确认失败／过期且不会再结算，才释放额度，否则保持未知。
4. 修改 `NewApp`：启动后继续查未完成付款，不重新签名付款。账本和恢复测试通过后，再把 `createTask` 的全局阻塞、`run` 的全局锁改成按钱包处理。

- [ ] **完成标准：** “钱已付但响应丢了”在重启后能查回，重复请求不再扣钱；另一个钱包正常运行。RPC 故障、晚到结算和撤销授权也要测试。

## 5. 完善自己的付款流程，验证需要的结算能力

**先读：** [arc-nanopayments/agent.mts](https://github.com/circlefin/arc-nanopayments/blob/master/agent.mts)，只学习购买步骤和结果记录；其中 Gateway 调用不是完整结算源码。实现时读 [x402 Go client](https://github.com/x402-foundation/x402/blob/main/go/CLIENT.md) 和 [facilitator](https://github.com/x402-foundation/x402/blob/main/go/FACILITATOR.md)，对照项目锁定版本，不能照搬最新分支接口。

**动手顺序：**

1. 保留现有 x402 `exact` 路线。固定 [payment.go](../../internal/agent/payment.go) 的顺序：**读报价 → 检查收款人和金额 → 最新风险检查 → 占用预算 → 保存授权并签名 → 发请求 → 查账 → 更新记录。** 所有决定由我们的 Go 代码执行。
2. 给自己的付款模块补成功、报价变更、换收款人、超限、风险未知、签名后断网等测试。模型或商家返回的文字不能改变规则。
3. 区分买方与卖方：买别人的 API，商家负责提交结算；我们保存授权并独立查账。商家内部用什么设施不由买方决定，不能把它算作我们的自研成果。
4. 如果要把自己的演示卖方也完全自运行：用 x402 开源 facilitator 组件新增 `cmd/facilitator`，自己运行验证、提交交易和状态记录。把 `SellerHandler` 当前写死的 `https://x402.org/facilitator` 改成配置，指向自己的服务。这是额外的卖方工作，单独测试重复提交、重启、手续费余额不足和失败交易，不复制 Circle 的托管服务。
5. 不为首版重写 Gateway 批量结算或跨链系统。目标链、USDC 合约、签名格式、结算和风险数据是否匹配，逐项实测；Arc 不能只改链 ID 就算完成。

- [ ] **完成标准：** 买方核心流程不调用 Circle 托管接口。模拟和测试网检查通过后，先确定主网单笔及总额上限，再小额实测。若自建演示结算服务，也必须通过单独的恢复测试。

## 6. 自己验收交付，再做自动换商家

**参考：** Circle 示例只能帮助理解付款流程，不能替我们判断数据是否有用。

1. 两家各完成一次真实购买。新增 `internal/agent/delivery.go`，统一输出，检查必需字段、任务匹配和数据时效；无法判断的质量问题显示“未检查”。
2. 分开保存付款和交付状态，改 [summary.go](../../internal/agent/summary.go)，不能把“已付款”显示成“任务完成”。
3. 在 [app.go](../../internal/agent/app.go) 的 `discover`、`execute_purchase` 中实现：签名前被拦截，才从剩余商家再选一次。重新查报价和风险，仍受原预算、允许网址和数据共享范围约束。
4. 第一版最多换一次。已签名且结果未知先查账；已付但数据不好不自动再买。接口支持免费重取时，也要禁止自动签名付款。

- [ ] **完成标准：** 测正常购买、签名前切换、风险查询失败、付款未知、过期数据和两家都不可用。记录商家、签名状态、花费和结果；模拟测试与真实调用分别标注。

## 7. 做清楚页面，再找用户试

**先读：** [arc-nanopayments/app](https://github.com/circlefin/arc-nanopayments/tree/master/app) 的付款记录页面，只参考展示信息。页面仍用我们的 Vue 写。

1. 改 [App.vue](../../web/src/App.vue)：任务、预算、允许的服务、开始、暂停；结果显示买谁的、花多少、数据是否合格、为什么失败。
2. 请两位目标开发者试自己的任务。先预览，通过资金检查后小额使用；记录全部费用、耗时、人工处理次数和重复使用。
3. 相同预算下比较固定商家与自动选择，覆盖正常、宕机、风险拦截和过期数据。没有改善的案例也记录。

- [ ] **完成标准：** 能用真实记录说明用户省了什么、代价是多少。自研和功能齐全都不能替代这一步。

## 学 Circle 时具体怎么记

每个文件只留下四项：**输入是什么、检查了什么、输出是什么、我们在哪个文件实现。** 先看源码及测试，再写自己的版本，最后用自己的失败案例检查。调用外部 SDK 的那一层不等于 SDK 或后台源码；复制或修改代码前检查许可证，换语言也不能自动免除许可证要求。

## 工期与审查结论

- 先安排两天完成第 0–1 步，并看清第 2 步的密钥改造范围；其余工作根据结果估时。自研责任更多，不再承诺两周做成完整成熟产品。
- 前五项实现限制有代码依据，把握较高；“用户为什么需要我们”还要靠试用验证。Circle 的完整钱包、预算和市场后台源码，本次没有确认公开，计划不依赖拿到这些源码。
- 仍需验证目标链支持、恢复正确性、密钥保管和实际需求。加密文件或测试通过，都不能单独证明公开多用户资金保管已经安全。
- 首版只做一个用途、两家商家、一条链。模型和风险服务先保留现有方案；风险资料查不到就暂停，不能自己编造安全结论。

Arc 申请作为单独目标：[活动页面](https://community.arc.io/public/events/arc-microgrants-f8tijfjhyq)目前要求 Arc 主网可运行，截止北京时间 **2026-10-15 11:59**。有通过检查的小额主网演示再准备仓库和材料，不为赶截止日跳过付款测试。

更多源码入口见 [Circle 学习笔记](<../Learn(LearnFromThese)/WebSite/WebSiteCanLearn/Circle/CircleCodeStudy.md>)；额外想法见 [Think](<../Think(SomeIdeas)/Think(SomeIdeas).md>)。

**现在就做：跑第 0 步检查，读 `services.ts` 的数据整理部分，然后写两家候选表和自己的服务配置。**
