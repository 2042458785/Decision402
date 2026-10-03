# Circle：功能、购买流程、源码

资料核对：2026-10-03。[English](CircleCodeStudy.en.md)

## 各环节由谁负责

| 顺序 | 环节 | 产品或工具 | 负责什么 |
|---|---|---|---|
| 1 | 准备工具 | Circle CLI、Circle Skills | CLI 提供后续操作命令；Skills 教 AI 怎么调用，可选。 |
| 2 | 钱包与规则 | Agent Wallets | 建钱包、设额度和地址限制，购买时签名。 |
| 3 | 购买资金 | Gateway | 接收存入的 USDC，提供购买余额。 |
| 4 | 找服务 | Agent Marketplace | 返回候选服务；Agent 或应用决定买哪家。 |
| 5 | 看报价 | CLI、商家 | 读取商家的付款要求，设置本次付款上限。 |
| 6 | 签名 | Agent Wallets、付款客户端 | 生成授权，由钱包签名后交给商家。 |
| 7 | 收款确认 | 商家、Gateway | 商家提交授权；Gateway 核验、锁定买方金额，记录卖方待结算金额。 |
| 8 | 交付与结算 | 商家、CLI、Gateway | 商家返回数据，CLI 展示；Gateway 后续批量上链。 |

## 购买的实际顺序：Agent Wallets + Marketplace + Gateway

| 步骤 | 谁操作 | 做什么 | 资料 |
|---|---|---|---|
| 1 | 用户、CLI | 邮箱和验证码登录，首次验证后自动创建钱包 | [钱包教程](https://developers.circle.com/agent-stack/agent-wallets/quickstart) |
| 2 | 用户、Agent Wallets | 设置单笔、日、周、月额度和地址限制；修改需验证码。仅主网，额度使用滚动时间窗口 | [额度教程](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies) |
| 3 | 用户、Gateway | 钱包充值 USDC，再存入 Gateway，确认余额可用；前面三步可提前完成 | [购买教程](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 4 | 用户、Agent | 用户提出任务，例如查天气；Agent 决定调用哪些工具 | [产品说明](https://www.circle.com/blog/introducing-circle-agent-stack-financial-infrastructure-for-the-agentic-economy) |
| 5 | Agent、Marketplace | `services search` 搜索候选服务 | [CLI 教程](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 6 | Agent、商家 | `services inspect` 查看付款要求；x402 商家用 HTTP 402 给出金额、币种、链等信息 | [买方教程](https://developers.circle.com/gateway-nanopayments/quickstarts/buyer) |
| 7 | Agent 或应用 | 选商家、钱包、链和本次最高金额 `--max-amount`；排序规则由应用决定 | [CLI 教程](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 8 | 付款客户端、钱包 | 发起 `services pay`；按付款要求签署授权，带 `PAYMENT-SIGNATURE` 请求商家 | [买方教程](https://developers.circle.com/gateway-nanopayments/quickstarts/buyer) |
| 9 | 商家、Gateway | 商家提交授权；Gateway 核验签名、锁定买方金额，记录卖方待结算金额；无效付款被拒绝 | [卖方教程](https://developers.circle.com/gateway-nanopayments/quickstarts/seller) |
| 10 | 商家、Agent | Gateway 接受付款后返回数据，CLI 显示结果；可查剩余余额。数据是否有用还要由应用检查 | [购买教程](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart) |
| 11 | Gateway | 后台合并多笔付款上链；确认后，卖方待结算金额变为可用余额。拿到数据可以早于这一步 | [结算原理](https://developers.circle.com/gateway-nanopayments/concepts/batched-settlement) |

## 另一种收款方式：Facilitator Service（独立于上面的 Gateway 路径）

| 步骤 | 谁操作 | 做什么 |
|---|---|---|
| 1 | 买方、商家 | 请求收费服务，商家返回 HTTP 402 报价 |
| 2 | 买方钱包 | 签署付款授权，再向商家请求数据 |
| 3 | 商家 | 向 Facilitator 提交授权和卖方证明 |
| 4 | Facilitator | 验证授权、筛查双方，拒绝不合格付款；通过后提交 USDC 转账 |
| 5 | 商家 | 已确认就交付；待确认就查原付款状态，重试保留原授权，不能新签一笔来代替查账 |
| 资料 | [官方流程](https://developers.circle.com/facilitator-service/how-it-works) | [付款与查询教程](https://developers.circle.com/facilitator-service/quickstart)；查询是卖方接口 |

## 比较时要看清的区别

| 项目 | 已确认／还没确认 |
|---|---|
| 风险筛查 | [Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets)说明转账上链前筛查制裁对象；[Facilitator](https://developers.circle.com/facilitator-service)筛查买卖双方。未确认与我们的风险排序、签名前复查完全相同。 |
| Gateway 花费限制 | [钱包规则](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies)与 `--max-amount` 都有文档；钱包规则是否覆盖每笔 Gateway 消费，还没确认。 |
| 付款与数据 | [Nanopayments](https://developers.circle.com/agent-stack/agent-nanopayments)解决付款；数据质量检查、失败后自动换商家，未找到与我们设想完全相同的说明。 |

## 哪些代码公开，具体看哪里

| 想看什么 | GitHub 和目录 | 公开了什么／还缺什么 |
|---|---|---|
| 网页钱包 | [modularwallets-web-sdk](https://github.com/circlefin/modularwallets-web-sdk)：`examples/`、`packages/w3s-web-core-sdk/` | SDK、示例和测试；不是完整 Agent Wallets 后台。 |
| 智能钱包权限 | [buidl-wallet-contracts](https://github.com/circlefin/buidl-wallet-contracts)：`src/msca/` 和测试 | 智能账户、权限插件；不是 MPC 密钥保管后台。 |
| 小额购买与记录页 | [arc-nanopayments](https://github.com/circlefin/arc-nanopayments)：[agent.mts](https://github.com/circlefin/arc-nanopayments/blob/master/agent.mts)、[app](https://github.com/circlefin/arc-nanopayments/tree/master/app) | 买卖方示例、页面；调用 Gateway，不是完整结算后台。 |
| Gateway 合约 | [evm-gateway-contracts](https://github.com/circlefin/evm-gateway-contracts)：`src/`、`test/`、`quickstart/` | 链上合约和测试；完整链下后台是否公开未确认。[产品文档](https://developers.circle.com/gateway) |
| 配置额度 | [agent-wallet-policy](https://github.com/circlefin/skills/blob/master/plugins/circle/skills/agent-wallet-policy/SKILL.md) | 操作说明；没有在这份文件中提供预算账本实现。 |
| 搜索服务 | [services.ts](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts)：`mapSearchItem`、`preferredAccept`、`searchServices` | 整理字段、调用 CLI；不是市场搜索和审核后台。 |
| Agent 接工具 | [starter-kits/kits](https://github.com/circlefin/agent-stack-starter-kits/tree/master/kits) | 不同 Agent 框架调用 Circle 工具的示例。 |
| 我们已有的开源库（非 Circle） | [x402 Go](https://github.com/x402-foundation/x402/tree/main/go)、[facilitator](https://github.com/x402-foundation/x402/blob/main/go/FACILITATOR.md)、[go-ethereum keystore](https://github.com/ethereum/go-ethereum/tree/master/accounts/keystore) | 分别看付款、卖方结算、加密密钥保存。复制或修改代码前检查各仓库许可证。 |

| 总入口 | 链接 |
|---|---|
| 官方文档 | [Agent Stack](https://developers.circle.com/agent-stack) |
| 公开仓库 | [circlefin](https://github.com/orgs/circlefin/repositories) |
| 服务目录 | [Marketplace](https://agents.circle.com) |
| 我们的改法 | [HowToImprove](../../../../HowToImprove/HowToImprove.md) |
