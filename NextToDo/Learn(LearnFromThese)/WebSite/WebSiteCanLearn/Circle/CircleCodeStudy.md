# Circle：去哪里看，能学什么

核对日期：2026-10-02。方向已改为自研：学习公开代码和产品规则，功能由我们自己实现；不把 Circle CLI、托管钱包、搜索或 Gateway 服务接进项目。无需注册 Circle 账户才能开始学习。[English](CircleCodeStudy.en.md)

下文“我们自己写”是开发计划；新增模块名表示准备放代码的位置，不代表功能已经完成。

## 先收藏这几个入口

| 页面 | 里面是什么 |
|---|---|
| [Circle GitHub 仓库目录](https://github.com/orgs/circlefin/repositories) | 找公开代码。在仓库搜索框输入下文的仓库名。 |
| [Agent Stack 官方文档目录](https://developers.circle.com/agent-stack) | 钱包、付款、服务搜索等功能的总入口。看页面里的 “The agent stack” 和 “Dive deeper”。 |
| [Agent Marketplace](https://agents.circle.com) | 观察服务列表展示哪些字段；仅作产品参考，不作为项目依赖。 |

**官网用来了解功能，GitHub 用来研究实现。写着“调用 Circle 接口”的代码，不是那个接口的后台源码。**

## 1. 钱包：创建、签名、限制谁能花钱

- **产品说明：** [Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets)。解释 Agent 怎么使用钱包、用户怎么限制它；文档说明它使用 MPC 管理密钥，密钥份额不暴露给 Agent。
- **Web SDK 源码：** [modularwallets-web-sdk](https://github.com/circlefin/modularwallets-web-sdk)。先看 README，再看 [examples](https://github.com/circlefin/modularwallets-web-sdk/tree/master/examples) 和 [SDK 目录](https://github.com/circlefin/modularwallets-web-sdk/tree/master/packages/w3s-web-core-sdk)，学习智能钱包在网页里的接入方式。
- **钱包合约源码：** [buidl-wallet-contracts](https://github.com/circlefin/buidl-wallet-contracts)。先看 [src/msca](https://github.com/circlefin/buidl-wallet-contracts/tree/master/src/msca) 和测试，学习智能账户及权限插件。
- **我们自己写：** `wallet.go` 管理钱包、权限和签名，`keystore.go` 管理加密密钥。Circle 代码用于学习模块分工；普通 EVM 钱包的加密存储可使用已有的 [go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore)。首版不自研 MPC。
- **别混淆：** Modular Wallets 和 Agent Wallets 不是同一份完整产品源码。以上两个仓库不能让我们直接复制 Circle 的账户、MPC 签名和全部后台。合约仓库标有 GPL-3.0，复制代码前看许可证。

## 2. 支付：买方怎么付，卖方怎么收

- **使用教程：** [Make a nanopayment](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart)。演示充值、搜索服务、查看报价、付款和查余额。
- **示例源码：** [arc-nanopayments](https://github.com/circlefin/arc-nanopayments)。先看 [agent.mts](https://github.com/circlefin/arc-nanopayments/blob/master/agent.mts)，再看 [app](https://github.com/circlefin/arc-nanopayments/tree/master/app)。分别学习买方付款、卖方收费接口和付款记录页面。
- **更底层的说明：** [Gateway 文档](https://developers.circle.com/gateway) 介绍统一 USDC 余额；[evm-gateway-contracts](https://github.com/circlefin/evm-gateway-contracts) 是对应的 EVM 合约源码，先看 `src/`、`test/`、`quickstart/`。
- **我们自己写：** 完善 `payment.go` 的报价、签名、付款与结果记录。保留现有 [x402 Go 开源库](https://github.com/x402-foundation/x402/tree/main/go)，不使用 Circle Gateway 接口。要运行自己的演示结算服务，再读 x402 的 facilitator 文档和代码。
- **别混淆：** Nanopayments 示例注明用于测试网，需要改造才能用于生产。Gateway 合约公开，不等于它的全部链下服务也在仓库里。

## 3. 预算：每次、每天、每周能花多少

- **产品教程：** [Set spending policies](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies)。介绍单笔、日、周、月额度，以及收款地址允许／禁止名单。
- **GitHub 使用说明：** [agent-wallet-policy/SKILL.md](https://github.com/circlefin/skills/blob/master/plugins/circle/skills/agent-wallet-policy/SKILL.md)，所在仓库是 [circlefin/skills](https://github.com/circlefin/skills)。它写明查看、修改和重置额度的方法；修改需要用户验证，当前说明仅适用于主网 Agent 钱包。
- **我们自己写：** `policy.go` 保存规则，`ledger.go` 检查并累计花费；在数据库里处理并发占用、成功扣记和失败释放。参考 Circle 的功能种类，第一版明确采用北京时间每日额度，不调用它的限额接口。
- **别混淆：** 这是调用说明，不是后台预算账本源码。不能据此复制出并发扣减、重启恢复和任务预算；示例里的累计花费也不能直接当严格预算控制。

## 4. 服务发现：搜索商家、读取价格和接口

- **网站入口：** [Agent Marketplace](https://agents.circle.com)。只观察列表信息和操作顺序，不要求使用它的搜索服务。
- **源码：** [agent-stack-starter-kits](https://github.com/circlefin/agent-stack-starter-kits)。重点看 [services.ts](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts)：调用搜索命令，整理服务网址、名称、价格、链和请求方法。
- **Agent 接入示例：** 同一仓库的 [kits](https://github.com/circlefin/agent-stack-starter-kits/tree/master/kits)，学习不同 Agent 框架怎么调用 Circle 工具。
- **我们自己写：** 从商家文档收集两家服务到 `providers.json`，由 `providers.go` 加载、筛选，`Quote` 直接读商家报价。参考 `mapSearchItem` 的字段整理，不复制 `searchServices` 里的 Circle CLI 调用。
- **别混淆：** 搜索调用代码公开，不等于服务市场的收录、搜索引擎和审核后台完整公开。搜到两个服务，也要实际验证它们能不能互相替代。

## 5. 付款出错：怎么避免再扣一次钱

- **重点教程：** [Facilitator Service quickstart](https://developers.circle.com/facilitator-service/quickstart)。Facilitator 是帮助卖家验证和结算付款的服务。
- **先看哪几步：** Step 3 的付款编号，Step 4 的 `pending`（结果还没确定），Step 5 的状态查询。
- **我们自己写：** `reconcile.go` 保存并查询原授权，通过目标链 RPC 核对交易和事件，重启后继续查。只借鉴教程里的“未知就继续查、不能新扣一次”，不调用 Circle 的查询服务。
- **别混淆：** 这是服务接口教程，使用 Arc 测试网，查询需要卖方证明；我们作为买方不能直接假设有同样的查询权限。完整后台源码本次尚未确认公开。

## 6. 风险能力：有筛查，不等于完整等价

2026-10-03 补充核对：[Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets) 明确说明转账制裁筛查；[Facilitator Service](https://developers.circle.com/facilitator-service) 明确说明筛查买卖双方。因此不能说 Circle 没有风险拦截。它是否能按我们的任务要求比较数据质量并自动换商家，本次未确认。

制裁筛查是检查交易方是否涉及受限制对象，不等于检查购买的数据是否有用。具体风险算法和完整后台源码仍未确认公开。

## 7. 这些产品怎么连起来

- **用户和钱包：** 用户设置规则，Agent Wallets 管理钱包和签名。
- **找服务：** Agent 通过 Circle CLI 搜索 Marketplace，查看商家的报价。
- **付款有不同路径：** Nanopayments 使用 Gateway 余额并批量结算；Facilitator Service 帮卖家验证买方签名、筛查双方、提交链上付款和查询结果。不是先经过前者再经过后者。[Nanopayments](https://developers.circle.com/agent-stack/agent-nanopayments)／[Facilitator](https://developers.circle.com/facilitator-service)

额度文档说的是主网、滚动时间窗口；不要直接当作我们计划的“北京时间每天重置”。也不能假定所有支付路径都受同一组限制，学习时逐项核对。

## 建议阅读顺序

1. **`services.ts` 的字段整理：** 写自己的服务配置和报价读取。
2. **钱包 SDK 的示例与测试：** 写自己的钱包模块和加密存储；无需先读完合约。
3. **额度说明、付款异常说明：** 写自己的账本和恢复流程；这些后台代码没有直接放在说明文件里。
4. **付款示例和页面：** 完善我们自己的 x402 付款与 Vue 结果页，不搬 Gateway 调用。

每项只记：**输入是什么、检查了什么、输出是什么、我们在哪个文件实现。** 复制或修改代码前看许可证。完整执行顺序以 [HowToImprove](../../../../HowToImprove/HowToImprove.md) 为准。
