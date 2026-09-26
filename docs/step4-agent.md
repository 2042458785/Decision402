# 第四步：Agent + 策略 + 简单界面 / Step 4: Agent, Policy, and UI

## 如何启动 / Start

在仓库根目录执行。第一次或前端变更后先构建；后续只需启动 Go 服务。

Run from the repository root. Build the frontend initially and after UI changes; otherwise just start Go.

```sh
cd /Users/ddy/GolandProjects/Decision402/Decision402
npm --prefix web ci
npm --prefix web run build
go run ./cmd/decision402
```

打开 **http://127.0.0.1:8080**。一个服务同时提供页面、Agent API 和 A/B/C/D 四个本地付费端点，**不需要再启动四个 serve 命令**。旧的 x402-demo 命令仍可独立使用。

Open **http://127.0.0.1:8080**. One server hosts the UI, agent API, and four local paid endpoints; **no separate serve processes are required**. The earlier x402-demo CLI remains available.

如页面显示端口冲突，先在正在运行此服务的终端按 Ctrl+C。一次只运行一个新服务实例。

If the port is occupied, stop the existing instance with Ctrl+C. Only one new app instance may run at a time.

## 你需要的配置 / Configuration

真实配置放在 `.env`；公开模板 `.env.example` 不存密钥。API key 和钱包私钥仅由后端读取。

Keep actual settings in `.env`, never credentials in `.env.example`. API keys and wallet keys are read only by the backend.

| 配置 / Setting | 作用 / Purpose |
| --- | --- |
| `DEEPSEEK_BASE_URL=https://api.deepseek.com` | 固定官方模型服务 / Official model API |
| `DEEPSEEK_API_KEY` | 模型调用鉴权 / Model API authentication |
| `DEEPSEEK_MODEL=deepseek-flash` | 本次账号模型列表验证可用的模型 / Model verified in this account's model list |
| `INTERCEPTA_API_KEY` | 真实收款地址风险检查 / Live recipient screening |
| `INTERCEPTA_NORMAL_ADDRESS` | D 的演示收款地址；未另配 C 时也用于 C，需由你控制 / D recipient; also C by default; must be controlled by you |
| `INTERCEPTA_RISK_ADDRESS` | A、B 共用的已知风险样例 / Shared known-risk fixture for A and B |
| `INTERCEPTA_ADDRESS_SOURCE` | 样例来源记录 / Fixture provenance |
| `INTERCEPTA_LOW_RISK_TRAITS` | 可选，逗号分隔的**真实 API `trait.name` 精确值**；仅在核实后填写 / Optional exact reviewed API trait names; leave empty until verified |
| `INTERCEPTA_LOW_RISK_ADDRESS` | 可选，C 的独立收款地址；你必须控制其 Base Sepolia 地址，且主网 API 必须显示已核实的轻微信号 / Optional C recipient; you must control it on Base Sepolia and verify its advisory mainnet API signal |
| `.buyer-key` 文件 / file | 买方测试钱包私钥，权限 600，需有测试 USDC / Buyer test key, mode 600, funded with test USDC |

模型调用消耗 DeepSeek 账户额度。页面上的 USDC 预算只约束本任务的 x402 付款，不包括模型费用或其他独立任务。

Model calls consume DeepSeek API credit. The USDC budget controls this task's x402 payment only, not model charges or other independently authorized tasks.

## 界面怎么用 / Use the UI

1. 设置单次上限、任务总预算、允许的风险等级、价格或风险优先。
2. 输入：“帮我获取东京天气样例，按我的策略选择服务。”
3. 选择模式后点击运行。查看候选表、选中服务、Agent 解释和执行记录。

1. Set the per-payment cap, task budget, maximum accepted risk, and price/risk preference.
2. Ask for a Tokyo weather sample using your policy.
3. Select a mode and run. Inspect candidates, selection, the explanation, and the expandable execution log.

## 比赛主演示 / Main judging demo

1. 设置单次上限 `0.10`、任务预算 `0.10` USDC；只允许风险等级 `0`，选择“价格优先”。输入：“帮我获取一份东京天气样例数据，按照页面设置的预算和风险规则选择服务。”
2. 先选“真实 API 预览”。预期看到 A/B 的风险分数与原因，C/D 符合授权，后端说明“过滤：A/B 风险阻断；C/D 符合授权。筛选：价格优先，选择 C（0.01 USDC）”。这一步没有签名或付款；若真实结果不同，先核查原因。
3. 再选“真实测试网执行”并新建任务。展示签名前的 `final_risk`、最终结算状态、Base Sepolia 交易哈希和静态样例数据。A/B 不会收到付款。四个端点属于同一本地样例程序，不是四家独立供应商。

1. Set both caps to `0.10` USDC, accept only level `0`, select price-first, and request the Tokyo sample weather dataset under the page policy.
2. Run Live API Preview. Show A/B's risk scores and reasons, C/D as eligible, and the backend explanation selecting C at `0.01` USDC. This step never signs or pays; investigate any different live result.
3. Run Testnet Execution as a new task. Show `final_risk`, settlement status, the Base Sepolia transaction hash, and static sample data. A/B never receive payment. All four endpoints belong to one local demo program, not independent vendors.

| 模式 / Mode | 真实使用的能力 / Live components | 是否付款 / Payment |
| --- | --- | --- |
| 策略模拟 / Simulation | DeepSeek Tool Calling；模拟 C 等级 1、0.01 USDC，D 等级 0、0.05 USDC / DeepSeek tools; synthetic C level 1 at 0.01 and D level 0 at 0.05 | 不签名、不付款 / No signing or payment |
| 真实 API 预览 / Live preview | DeepSeek + HTTP 402 报价 + Intercepta / DeepSeek, HTTP 402 offers, Intercepta | 不签名、不付款 / No signing or payment |
| 真实测试网执行 / Testnet execution | 上述能力 + 最终检查 + EIP-3009 授权及 x402 结算 / Above plus final checks, authorization, settlement | 会花费测试 USDC / Spends test USDC |

**当前真实演示走两类风险结果。**A/B 的已知风险地址被阻断；C/D 使用同一个正常地址、同为等级 0，Agent 在预算内选择价格更低的 C。签名前再次调用 Intercepta；结果变化、API 出错或报价变化都停止签名。当前没有真实轻微信号，页面在真实模式只允许等级 0；等级 1 的 C/D 偏好取舍明确限定在模拟模式。模拟不是赞助商真实风控证据。

**The current live demo uses two outcomes.** A/B are blocked after screening the known-risk address. C/D share a clean recipient at level 0, so the agent picks the cheaper C within budget. Intercepta runs again before signing; changed risk, API failure, or a changed quote stops signing. No verified advisory fixture is available, so the live UI authorizes only level 0. The C/D level-1 preference trade-off is simulation only and is not sponsor API evidence.

代码保留了可选等级 1 扩展，但**这次展示无需配置** `INTERCEPTA_LOW_RISK_TRAITS` 或 `INTERCEPTA_LOW_RISK_ADDRESS`，保持为空即可。将来若有经过赞助商确认的真实 API `trait.name`，以及你控制的独立 C 测试网收款地址，可再启用；项目规则要求总分为 0、所有 trait 精确列入白名单且每条 `risk` ≤ 20，并在签名前复查。20 是我们的保守上限，不是赞助商官方阈值。不要把不受你控制的 Dashboard 示例地址设为收款人。

Optional level-1 support remains in the code, but **neither** `INTERCEPTA_LOW_RISK_TRAITS` **nor** `INTERCEPTA_LOW_RISK_ADDRESS` is needed for this demo; leave both empty. Enable them later only with a sponsor-reviewed exact API `trait.name` and a separate C recipient you control on Base Sepolia. Our project policy requires score 0, all traits allowlisted with each `risk` ≤ 20, and a fresh scan before signing. The 20-point ceiling is ours, not an official sponsor threshold.

四个端点均为同一程序提供的**静态东京天气样例**，不是四家真实独立商家，也不是实时天气。A/B 共用风险地址；C 默认与 D 共用正常地址，只有配置了独立 C 地址时才可能得出不同风险结论。C/D 同等级时，风险优先也会因价格更低选 C。这是正确行为，不伪造差异。

All four endpoints serve **static Tokyo demo weather** from the same program. They are not four independent vendors or live weather sources. A/B share the risky address. C shares D's clean recipient by default; only a separately configured C recipient can yield a different live risk result. If C/D have the same level, risk-first also chooses cheaper C. This is expected, without fabricated differentiation.

## 执行流程 / Execution flow

```text
Vue 表单 / user form
  → Go 校验并持久保存任务授权 / validate and save immutable task authorization
  → DeepSeek 理解任务 / understand task
  → find_services(city) / request available services
  → Go 获取报价并调用 Intercepta / fetch offers and screen recipients
  → Go 策略：过滤 → 排序 / filter and rank
  → execute_purchase() / request execution without override parameters
  → Go 最终检查 + 记录授权 / final check + durable authorization reservation
  → x402 签名 → facilitator 结算 / sign → settlement
  → 返回样例数据与回执 / return sample data and receipt
  → Go 后端依据真实结果说明过滤、选择与付款状态 / Go reports filtering, selection and actual payment status
```

`find_services` 只支持 Tokyo/东京；不相关任务应由模型说明限制。模型只能调用两个工具，不能传入私钥、收款地址、预算或风险覆盖值。付款工具绑定服务器创建的任务，不接受任意任务 ID。

`find_services` supports Tokyo only; the model should explain unsupported tasks. Only two tools are exposed. The model cannot pass private keys, recipients, budget/risk overrides, or arbitrary task IDs to the bound purchase tool.

策略是确定性 Go 代码：先剔除高风险、未知风险、超预算和不符合授权的选项，再按偏好排序。价格相同按风险排序，风险相同按价格排序，仍相同则按服务 ID。用户授权由表单提交并在任务内保持不变；模型提示词不是付款权限的唯一约束。

Policy is deterministic Go code: remove high/unknown risk, over-budget, and unauthorized candidates before sorting. Price ties use risk; risk ties use price; final ties use service ID. The submitted authorization is immutable within a task; prompts are not the only payment control.

## 各文件做什么 / Files

| 文件 / File | 职责 / Responsibility |
| --- | --- |
| `cmd/decision402/main.go` | 配置、单实例锁、启动本地 HTTP 服务 / Config, single-instance lock, local server |
| `internal/agent/model.go` | DeepSeek HTTP 请求、两个工具的描述 / Model requests and tool definitions |
| `internal/agent/policy.go` | 整数金额、预算与风险过滤、候选排序 / Integer money, authorization checks, ranking |
| `internal/agent/app.go` | API、任务日志、模型工具循环、模拟与真实模式 / API, journal, tool loop, simulation/live modes |
| `internal/agent/summary.go` | 从已记录的候选与结算结果生成确定性演示说明 / Factual decision summary from recorded candidates and settlement |
| `internal/agent/payment.go` | 四个 x402 端点、读报价、最终扫描、签名及回执 / Paid endpoints, quotes, final scan, signing, receipts |
| `internal/probe/client.go` | 实际 Intercepta 请求：`Client.Scan` / Actual Intercepta HTTP call |
| `internal/probe/decision.go` | 真实响应校验与保守风险映射 / Response validation and conservative risk mapping |
| `web/src/App.vue` | 配置表单、候选表、状态、执行记录 / Form, candidate table, status, event log |
| `web/src/style.css` | 简单响应式界面 / Simple responsive styling |
| `internal/agent/agent_test.go` | 策略、签名前保护、持久记录、重复工具调用测试 / Policy, signing gate, journal and tool replay tests |
| `artifacts/tasks/<id>.json` | 每次授权、扫描、选择、付款及结果记录；不提交 Git / Local task evidence, Git-ignored |

这里的日志是普通本地记录，没有 ZK、链上策略合约或密码学审计证明。

Logs are ordinary local records. There is no ZK proof, onchain policy contract, or cryptographic audit attestation.

## 重复调用与失败 / Replays and failures

- 每个任务最多一次付款尝试。重复模型工具调用不会重新付款；相同请求 ID 复用结果，更改同一 ID 的策略被拒绝。
- 签名前把授权预留写盘并同步；写盘失败就不签名。重启后未完成任务暂停，不自动恢复。
- 已预留授权但未确认结算时，阻止新的真实付款任务；先核对交易和回执。当前没有自动对账或解锁功能，不要删除日志后盲目重付。
- 已结算但读取数据失败，也不能再付一次。任务上限不是整个钱包的日预算。

- One payment attempt per task. Repeated tool calls do not pay again. Identical request IDs reuse results; changed policy on the same ID is rejected.
- Authorization is durably reserved before signing; persistence failure prevents signing. Interrupted tasks hold after restart.
- An unresolved reserved payment blocks new live purchases pending reconciliation. There is no automatic reconciliation/unlock; do not delete evidence and blindly repay.
- Settlement followed by a data failure does not permit another payment. A task budget is not a wallet-wide daily budget.

本版本仅面向单人在本机演示，绑定 `127.0.0.1`；尚无公开部署所需的用户登录和钱包隔离。付款使用 Base Sepolia，风险来自主网。不要声称“绝不越权”或“等级 0 绝对安全”。

This is a single-user localhost demo, without public-deployment authentication or per-user wallet isolation. Payments run on Base Sepolia; risk data comes from mainnet. Do not claim universal prevention of unauthorized actions or absolute safety at level 0.

## 验证 / Validation

```sh
go test ./...
go vet ./...
npm --prefix web run build
```

自动测试使用明确的合成响应，不计为真实 API 或链上证据。实际集成验证记录见 `docs/step4-validation.md`。

Automated tests use synthetic responses and are not live API or onchain evidence. See `docs/step4-validation.md` for observed integration results.
