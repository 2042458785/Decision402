# Decision402：功能对比与改进

[English](HowToImprove.en.md)

## 现有功能：和 Circle 比，怎么改

| 功能 | 我们现在怎么做 | Circle 的同类功能 | 我们怎么改 | 完成情况 | 完成时间 | 提交代码 |
|---|---|---|---|---|---|---|
| 钱包密钥保管 | Agent 私钥和个人 DeepSeek Key 已加密保存在本地；支持锁定、解锁和钱包备份。解锁后仍由后台签名 | [Agent Wallets](https://developers.circle.com/agent-stack/agent-wallets)用 MPC 分开保管密钥份额，Agent 拿不到份额 | 本地加密已完成；用户独立控制钱包留到以后研究 | 本地加密已完成 | 2026-10-05 | `192fc7d` |
| 花费限制 | 只检查单笔、单任务上限；没有跨任务每日额度 | [钱包规则](https://developers.circle.com/agent-stack/agent-wallets/wallet-operations/custom-policies)有单笔、日、周、月额度和地址名单；适用于主网 | 先验证智能钱包能否完成现有 x402 付款，再设计跨任务额度和地址限制 | 未完成 |  |  |
| 找服务 | 固定四家本地天气样例 | [Marketplace 搜索代码](https://github.com/circlefin/agent-stack-starter-kits/blob/master/packages/circle-tools/src/services.ts)读取搜索结果、整理服务信息 | 改 `app.go`、`model.go`：从配置读取真实商家，保存网址、参数、价格和支付方式；验证两家能否提供同类数据。 | 未完成 |  |  |
| 报价与选商家 | 读取 402，按预算和风险排序；签名前复查 | [CLI 教程](https://developers.circle.com/agent-stack/agent-nanopayments/quickstart)支持搜索、查看付款要求、设置单次付款上限；未确认相同排序算法 | 改 `payment.go`：支持真实商家的支付选项，核对金额、币种、链和收款人；保留自己的排序，测试涨价和换地址。 | 未完成 |  |  |
| 风险检查 | Intercepta 查地址，Go 规则判断 | [钱包](https://developers.circle.com/agent-stack/agent-wallets)和 [Facilitator](https://developers.circle.com/facilitator-service)有制裁筛查；不能据此判断谁覆盖更多风险 | 改风险检查接口：保存来源、时间和原因；查不到或结果过期就停；签名前再查。 | 未完成 |  |  |
| 支付与结算 | Base Sepolia 测试 USDC，卖方提交结算 | [Gateway](https://developers.circle.com/agent-stack/agent-nanopayments)支持小额批量结算；[Facilitator](https://developers.circle.com/facilitator-service)支持 Arc、Base、Polygon 的 USDC 结算 | 保留现有 x402 付款，补失败测试；接入新链时验证合约、签名、结算和查账。是否做批量付款另看实际需求。 | 未完成 |  |  |
| 付款出错后恢复 | 结果未知需人工核对，还会挡住其他付款 | [Facilitator 教程](https://developers.circle.com/facilitator-service/quickstart)有付款编号、待确认状态和卖方查询 | 改 `app.go`、`payment.go`：保存原授权，重启后继续查链上结果；查清前不再付款；让不同钱包互不阻塞。买方不能直接套用卖方查询接口。 | 未完成 |  |  |
| 结果记录 | 保存付款结果和交易哈希，缺少自动查账 | [Gateway](https://developers.circle.com/gateway-nanopayments/concepts/batched-settlement)区分接受付款和上链结算 | 改任务记录和结果页：分清已签名、待确认、已付款、失败，显示查账结果；按我们的付款方式记状态。 | 未完成 |  |  |

## 钱包提现已完成

| 功能 | 现在能做什么 | 提交代码 |
|---|---|---|
| 转出测试 USDC | 把 Agent 的测试 USDC 转回当前登录的 MetaMask；待确认的提现可查状态、重发同一笔交易 | `777a9a5` |

提现仍由后台签名。提现查账没有解决 **x402 购买付款** 结果未知的问题。详见 [钱包改动](<../WhatWeImprove/Wallet/Wallet.md>)。

## 可以新增什么

| 功能 | 具体做什么 |
|---|---|
| 检查数据有没有用 | 检查地点、日期、更新时间和必需字段；分别显示“钱付了没有”和“数据合不合格”。 |
| 自动换商家 | 签名前失败，可重查另一家的报价和风险；共用原预算。付款未知先查账；已付后再买，要有用户授权。 |
| 暂停新付款 | 用户暂停后停止新签名，已发出的付款继续查结果。 |
| 给其他 Agent 调用 | 提供一个接口，接收任务、预算和允许的服务，返回数据、花费和失败原因。 |
| 商家使用记录 | 记录成功率、过期数据、价格和耗时，帮助用户选择。 |

[当前代码与流程](<../../V1.0(ETHGlobalTokyo2026Hackathon)/V1.0.md>) · [Circle 流程与源码](<../../../Learn(LearnFromThese)/WebSite/WebSiteCanLearn/Circle/CircleCodeStudy.md>) · [产品想法](<../../../Think(SomeIdeas)/Think(SomeIdeas).md>)
