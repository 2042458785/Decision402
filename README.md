# Decision402 — 用户授权下的 Agent 支付 / Policy-Constrained Agent Payments

## 新版：直接运行 Agent 页面 / New: Run the Agent UI

**新流程：**连接 MetaMask（Base Sepolia）并签署登录消息 → 输入 Agent 名称、DeepSeek API URL／模型／可选个人 API key → 创建 Agent 与专属测试网钱包 → 在 MetaMask 中确认给该地址转入测试 USDC（默认 10）→ 设置单次预算与风险偏好 → 先 Preview，再执行 Pay。页面自动使用该 Agent 的钱包付款；主页面不再需要手工创建 `.buyer-key`。钱包私钥和用户输入的模型 key 保存在本机被 Git 忽略的 `artifacts/agents/`，文件权限为 0600；页面和模型上下文不接收钱包私钥。

**New flow:** Connect MetaMask on Base Sepolia and sign in → enter an agent name, DeepSeek API URL/model, and optional personal API key → create an agent and its testnet wallet → approve a test-USDC transfer to it in MetaMask (default 10) → set the budget and risk policy → Preview, then Pay. The main UI now pays from the agent wallet and does not require a manually created `.buyer-key`. The wallet key and any entered model key stay in Git-ignored local `artifacts/agents/` files with 0600 permissions; neither the page nor the model prompt receives the wallet key.

充值是用户钱包发起的链上 ERC-20 转账，需要用户钱包持有 Base Sepolia ETH 支付这笔充值的 gas。Agent 后续使用现有 x402 授权流程，仍由 Go 策略引擎在签名前执行 Intercepta 复查。`artifacts/agents/` 是本地保管库，删除后无法恢复其中 Agent 钱包及剩余测试币；请只充值测试币并备份该目录。当前仅支持官方 DeepSeek 地址 `https://api.deepseek.com` 与 `deepseek-flash`／`deepseek-v4-pro`。连接钱包只用于本地会话，不表示已经具备可公开部署的多用户托管安全性。

Funding is an onchain ERC-20 transfer from the owner wallet and requires Base Sepolia ETH for gas. Subsequent x402 payments use the existing Go policy engine and a fresh Intercepta check before signing. Back up `artifacts/agents/`: deleting it loses the agent wallet and any remaining test tokens. Use testnet assets only. This adapter currently accepts the official `https://api.deepseek.com` endpoint and `deepseek-flash` or `deepseek-v4-pro`. Wallet sign-in secures the local session; this is not a production multi-user custody service.

现已接入 **DeepSeek + Go 策略引擎 + Intercepta + x402 + Vue3 页面**。一个进程提供页面和四个本地演示服务，不再需要分别启动 serve。

**DeepSeek, a Go policy engine, Intercepta, x402, and a Vue3 UI** are connected. A single process hosts the UI and four local demo services.

```sh
cd /Users/ddy/GolandProjects/Decision402/Decision402
npm --prefix web ci
npm --prefix web run build
go run ./cmd/decision402
```

打开 **http://127.0.0.1:8080**，先完成上面的钱包和 Agent 设置。**比赛主演示**：单次与任务上限均设 `0.10` USDC、只允许等级 `0`、价格优先；输入英文任务“Get a sample Tokyo weather dataset. Choose a service using my budget and risk policy.”先运行“Live API preview · No payment”，确认 A/B 被阻断、C 被选中，再运行“Live testnet execution · Pays automatically”展示签名前复查与 Base Sepolia 回执。C 的报价是 `0.01` 测试 USDC。策略模拟只是额外说明偏好取舍，不能代替真实赞助商 API 证据。模型调用会消耗 DeepSeek 额度。

Open **http://127.0.0.1:8080** and complete wallet/agent setup first. **Main judging demo:** set both caps to `0.10` USDC, allow only level `0`, and choose price-first. Ask for the Tokyo sample weather dataset under the page policy. Run Live API Preview first (no payment): A/B are blocked and C is selected. Then run Testnet Execution to show the final pre-signing scan and Base Sepolia receipt for C's `0.01` test-USDC quote. Simulation is optional and is not evidence of live sponsor screening. Model calls consume DeepSeek API credit.

现场可照读的中英双语讲稿：[Decision402 演示讲稿](docs/demo-script-bilingual.md)。完整的中英操作说明、模块职责、执行流程和限制：[第四步 Agent 指南](docs/step4-agent.md)。实测证据：[第四步验证记录](docs/step4-validation.md)。**当前真实演示只用已验证的两类结果：A/B 风险地址被阻断，Agent 改选本次扫描未检出风险且在预算内的 C。等级 1 的价格／风险取舍仅在标明的模拟模式展示。**

For bilingual instructions, architecture, and limitations, see the [Step 4 guide](docs/step4-agent.md) and [validation record](docs/step4-validation.md). **The live demo uses the verified binary path: risky A/B are blocked and the agent selects affordable C after no risk signal is detected. The level-1 price/risk trade-off remains explicitly simulated.**

以下是仍可独立使用的扫描和 CLI 演示说明。

The sections below document the standalone probe and CLI demos, which remain available.


本 README 按章节提供中英对照。命令和代码示例共用一份。

This README provides Chinese and English explanations in each section. Commands and code examples are shared.

现在 `pay` 会先检查报价，再真实调用 Intercepta 扫描报价里的 `payTo`，最后决定是否签名。`inspect` 仍然只看报价，不扫描、不付款。

`pay` now validates the quote, makes a live Intercepta call for its exact `payTo`, and decides whether signing may proceed. `inspect` only reads the offer; it neither scans nor pays.

**已验证：**此前的 x402 测试网结算成功；2026-09-26 的真实扫描在 SDK 签名前钩子中得到正常样例 `allow`、风险样例 `deny`。本轮使用替代支付模块，没有签名或转账。**后续进度：**Agent 版本已完成带真实筛查的测试网结算，详见第四步验证记录。

**Verified:** an earlier x402 testnet payment settled; live scans on 2026-09-26 produced `allow` for the normal fixture and `deny` for the risk fixture inside the SDK pre-signing hook. This test used a replacement payment module, with no signatures or transfers. **Subsequent progress:** the Agent version completed a screened testnet settlement; see Step 4 validation.

## 原有 CLI 操作 / Standalone CLI Usage

在本仓库根目录执行。真实 key 放在 `.env`，不要放在 `.env.example`；后者会提交到 Git。`INTERCEPTA_ADDRESS_SOURCE` 填普通网址即可，不用 Markdown 链接格式。

Run from the repository root. Keep the real key in `.env`, never in the tracked `.env.example`. Use a plain URL for `INTERCEPTA_ADDRESS_SOURCE`, without Markdown link formatting.

```sh
cd /Users/ddy/GolandProjects/Decision402/Decision402
```

**1. 不花测试币，先验证拦截。**下面会消耗两次真实 API 调用，通过 x402 SDK 检查正常放行、风险阻止；替代支付模块不读取钱包、不生成签名、不转账。

**1. Test the gate without spending test tokens.** This uses two live API calls through the x402 SDK. A replacement payment module verifies allow/block behavior without wallet access, signatures, or transfers.

```sh
DECISION402_LIVE_TEST=1 go test ./cmd/x402-demo -run '^TestLiveInterceptaGate$' -v -count=1
```

预期看到 `action: allow`、`action: deny` 和 `PASS`。这是签名前检查证据，不是完整付款成功证据。

Expect `action: allow`, `action: deny`, and `PASS`. This verifies the pre-signing gate, not a completed payment.

**2. 正常地址付款。**终端 1 启动卖方并保持运行：

**2. Pay the normal recipient.** Start the seller in terminal 1 and leave it running:

```sh
go run ./cmd/x402-demo -mode serve -pay-to 0x55eCFa861042bF2e5Ba9D1BDbF4C07D3C253B4a9
```

终端 2 付款。买方 `.buyer-key` 需已配置，钱包需有 Base Sepolia 测试 USDC。每次成功付款为 0.001 测试 USDC。

Pay in terminal 2. Configure `.buyer-key` and fund that wallet with Base Sepolia test USDC. Each successful payment spends 0.001 test USDC.

```sh
go run ./cmd/x402-demo -mode pay -pay-to 0x55eCFa861042bF2e5Ba9D1BDbF4C07D3C253B4a9
```

验收：先出现 `allow`，再有 `HTTP 200`、`success=true` 和交易哈希。地址必须是你确认的收款地址；API 未发现风险不代表绝对安全。

Acceptance: `allow`, then `HTTP 200`, `success=true`, and a transaction hash. Confirm ownership of the recipient yourself; an absence of detected risk is not a safety guarantee.

**3. 风险阻断演示及字段解释：**见 [第三步操作说明](docs/step3-intercepta-gate.md)。

**3. Risk-blocking demo and field explanations:** see the [Step 3 guide](docs/step3-intercepta-gate.md).

## 1. 领取并保存配置 / Obtain and Save Configuration

从 [Intercepta 活动入口 / event page](https://intercepta.io/ethglobal) 领取 sandbox key。正常样例可用你自己控制的新地址；风险样例可用赞助商 Dashboard 或活动 Discord 提供的地址，并记录来源。文档里的示例地址不自动等于正常样例。

Obtain a sandbox key from the event page. Use a newly created address you control as the expected-normal fixture; use the sponsor dashboard or event Discord for the known-risk fixture, and record their provenance. An address shown in the documentation is not automatically a normal test fixture.

这个 Git 仓库当前位于外层 GoLand 目录的同名子目录。在包含本 README 的目录执行；其他电脑请替换为自己的仓库路径：

This Git repository currently lives inside a same-named subdirectory of the outer GoLand project. Run these commands in the directory containing this README. On another computer, replace the path with your own repository location:

```sh
cd /Users/ddy/GolandProjects/Decision402/Decision402
cp -n .env.example .env
chmod 600 .env
```

| 命令 / Command | 中文说明 | English explanation |
| --- | --- | --- |
| `cd /Users/ddy/GolandProjects/Decision402/Decision402` | 进入实际 Git 仓库。两层同名目录来自当前本地目录结构。 | Enter the actual Git repository. The repeated name reflects the current local folder structure. |
| `cp -n .env.example .env` | 把模板复制成本地配置；`-n` 表示已有 `.env` 时不覆盖。 | Copy the template to a local configuration file. `-n` prevents overwriting an existing `.env`. |
| `chmod 600 .env` | 将权限设置为只有当前用户可以读写。 | Set the file permissions so only its owner can read and write it. |

`.env.example` 是可以公开提交的空白模板；`.env` 保存真实配置，已被 `.gitignore` 忽略。以 `.` 开头的文件可能被文件管理器隐藏，可以在编辑器的项目文件列表中打开。

`.env.example` is the blank template that can be committed publicly. `.env` contains the real configuration and is excluded by `.gitignore`. Files starting with `.` may be hidden by the file manager; open them from your editor's project file list.

在编辑器中填写 `.env`，将以下占位值全部替换为真实值：

Edit `.env` in your editor and replace the following placeholders with real values:

```dotenv
INTERCEPTA_API_KEY=your_actual_sandbox_key
INTERCEPTA_NORMAL_ADDRESS=sponsor_provided_normal_mainnet_address
INTERCEPTA_RISK_ADDRESS=sponsor_provided_risky_mainnet_address
INTERCEPTA_ADDRESS_SOURCE=fixture_source_or_discord_message_url
```

| 配置项 / Variable | 中文说明 | English explanation |
| --- | --- | --- |
| `INTERCEPTA_API_KEY` | 赞助商发给你的真实 key。 | The actual key issued by the sponsor. |
| `INTERCEPTA_NORMAL_ADDRESS` | 你自己控制的预期正常地址。 | An address you control, expected to have no risk signals. |
| `INTERCEPTA_RISK_ADDRESS` | 赞助商提供的已知风险主网地址。 | A sponsor-provided mainnet address with known risk. |
| `INTERCEPTA_ADDRESS_SOURCE` | 样例来源，例如展位工作人员或 Discord 消息链接。 | Fixture provenance, such as the sponsor booth or a Discord message URL. |

Key 只保存在本地，不贴到聊天或提交到仓库。程序不需要钱包私钥。

Keep the key local; do not paste it into chat or commit it to the repository. The tool does not require a wallet private key.

`.env` 支持空行、整行 `#` 注释、`NAME=value` 和一对外围引号；不执行 shell，不展开 `$变量`，不支持行尾注释或多行值。已有进程环境变量优先。两个地址必须不同，使用 `0x` 开头的 40 位十六进制地址；本步骤不做 ENS 解析或地址校验和验证。

The `.env` parser supports blank lines, full-line `#` comments, `NAME=value`, and one pair of surrounding quotes. It does not execute shell code, expand `$variables`, or support inline comments or multiline values. Existing process environment variables take precedence. The two addresses must differ and contain `0x` followed by 40 hexadecimal characters. ENS resolution and address checksum validation are not implemented in this step.

## 2. 运行两次真实请求 / Run Two Live Requests

整个仓库现在需要 Go 1.24 或更新版本，因为第二步使用官方 x402 Go SDK；第一步的扫描代码本身只使用标准库。Go 1.23.2 在 `GOTOOLCHAIN=auto` 下可能自动下载所需工具链。

The repository now requires Go 1.24 or later because Step 2 uses the official x402 Go SDK. The Step 1 probe itself uses only the standard library. With Go 1.23.2, `GOTOOLCHAIN=auto` may download the required toolchain.

```sh
go run ./cmd/intercepta-probe
```

`go run` 会编译并启动 `cmd/intercepta-probe` 目录中的命令行程序。运行过程如下：

`go run` compiles and starts the command-line program in `cmd/intercepta-probe`. It performs these steps:

| 步骤 / Step | 中文说明 | English explanation |
| --- | --- | --- |
| 1 | 读取并检查 `.env` 或环境变量中的配置。 | Load and validate configuration from `.env` or environment variables. |
| 2 | 请求 Intercepta，扫描「预期正常」的主网地址。 | Call Intercepta to scan the mainnet address expected to be normal. |
| 3 | 再请求一次，扫描「预期有风险」的主网地址。 | Make a second call for the mainnet address expected to be risky. |
| 4 | 将实际响应、字段、HTTP 状态和耗时保存到本次结果目录。 | Save actual responses, fields, HTTP status codes, and elapsed time in this run's output directory. |

**这一步只查询地址风险，不会签名或转账，也不需要钱包私钥。** 如果尚未填写 key 和地址，程序会提示配置问题，此时不会调用 API。

**This step only queries address risk. It does not sign anything or transfer funds, and it needs no wallet private key.** If the key or addresses are missing, the program reports a configuration error without calling the API.

默认每次请求超时 15 秒，依次扫描 normal 和 risk，无自动重试。自定义参数：

Each request has a 15-second timeout by default. The normal and risk fixtures are scanned sequentially, with no automatic retries. To customize the settings:

```sh
go run ./cmd/intercepta-probe -env .env -timeout 20s -out artifacts
```

请在仓库根目录执行上述命令；`.env` 放在其他位置时，用 `-env` 指定实际路径。

Run these commands from the repository root. If `.env` is stored elsewhere, pass its actual path with `-env`.

## 3. 查看实际证据 / Inspect the Collected Evidence

`artifacts/` 是运行结果目录。配置验证通过并开始扫描后，程序自动创建 `artifacts/intercepta-时间-随机后缀/`，不覆盖旧记录。下面仅说明文件结构，不代表已经拿到真实响应：

`artifacts/` stores run results. Once configuration validation succeeds and scanning starts, the tool creates a new `artifacts/intercepta-timestamp-random-suffix/` directory without overwriting earlier runs. The example below illustrates the layout; it does not imply that live responses have already been collected:

```text
artifacts/
└── intercepta-<timestamp>-<random-suffix>/
    ├── normal-response.json
    ├── risk-response.json
    ├── report.json
    └── summary.md
```

| 文件 / File | 中文说明 | English explanation |
| --- | --- | --- |
| `normal-response.json` | 正常样例的实际响应正文；无效 JSON 或截断改用 `.txt`。 | Actual response body for the normal fixture; `.txt` is used for invalid JSON or truncated responses. |
| `risk-response.json` | 风险样例的实际响应正文；无效 JSON 或截断改用 `.txt`。 | Actual response body for the risk fixture; `.txt` is used for invalid JSON or truncated responses. |
| `report.json` | 时间、耗时、地址、状态码、错误、正文哈希、实际字段路径／类型／值。 | Timestamps, elapsed time, addresses, status codes, errors, body hashes, and actual JSON field paths, types, and values. |
| `summary.md` | 可读的对照表与人工审查事项。 | A readable comparison table and items for manual review. |

先打开 `summary.md` 看调用状态和耗时，再看 `report.json` 及对应正文，确认实际风险字段与原因。「正常／风险」是样例的预期标签，是否符合预期需要根据真实响应确认。

Start with `summary.md` to inspect call status and elapsed time, then review `report.json` and the response bodies for actual risk fields and reasons. “Normal” and “risk” describe fixture expectations; the real responses must confirm whether those expectations hold.

字段路径采用 JSON Pointer，例如 `/data/flags/0`。大整数保留原始精度。正文通常原样保存；若接口意外回显 key，会替换该 key 并标记 `secret_redacted`，哈希针对保存后的字节。单个响应最多保存 1 MiB，超出即标记失败和截断。报告不记录请求鉴权头，程序不跟随 HTTP 重定向。

Field paths use JSON Pointer, such as `/data/flags/0`. Large integers retain their original precision. Response bodies are normally preserved unchanged. If the API echoes the key, its exact occurrences are redacted and `secret_redacted` is set; hashes refer to the stored bytes. Each response is limited to 1 MiB; larger responses are marked as truncated and failed. Reports exclude request authentication headers, and the client does not follow HTTP redirects.

`duration_ms` 是客户端从准备请求到读取／处理响应的时间，包括网络延迟。两次测量只说明这次运行，不能当作平均性能或服务端处理时间。

`duration_ms` measures client-side time from request preparation through response reading and processing, including network latency. Two measurements describe this run only; they are not a performance average or a measurement of server-only processing time.

| 结果 / Result | 中文说明 | English explanation |
| --- | --- | --- |
| Exit code `0` | 收到两次完整的 2xx JSON 响应；仍需检查是否包含业务错误。 | Two complete 2xx JSON responses were collected; they still need review for application-level errors. |
| Exit code `1` | 请求或记录过程出错。HTTP 错误、超时、无效 JSON 会记录到报告。 | A request or recording operation failed. HTTP errors, timeouts, and invalid JSON are recorded in the report. |
| Exit code `2` | 启动参数或配置无效，不请求 API。 | Startup arguments or configuration are invalid; no API request is made. |

网络失败时可能没有响应正文文件。如果输出目录不可写，报告也可能无法保存。

A network failure may leave no response body file. If the output directory is not writable, the report may also be unavailable.

## 4. 置信度审查与验收 / Confidence Review and Acceptance Criteria

| 判断 / Claim | 中文：证据与边界 | English: evidence and limitations |
| --- | --- | --- |
| GET endpoint and `X-API-KEY` | 官方接口文档明确，置信度高。 | Explicitly documented by the official API reference; high confidence. |
| Local transport, recording and error handling | 自动测试通过只能确认被测试的程序行为。 | Passing local tests supports only the program behavior covered by those tests. |
| Normal/risky fixture findings | 已观察到评分 0／100 和风险标签，置信度高，仅适用于本次样例。 | Scores 0/100 and risk traits were observed; high confidence for these samples only. |
| Full sponsor prize eligibility | 未确认：仍需完整付款演示、Agent 流程及提交材料。 | Not established: full payment demo, agent workflow, and submission materials remain. |

`normal` / `risk` 是你提供样例时的**预期标签**。程序从不把标签当作 API 结论，也不假设存在 `allow`、`deny` 或某个特定的 `riskGroup` 字段。

`normal` / `risk` are **expected fixture labels supplied by you**. The program never treats them as API verdicts and does not assume that `allow`, `deny`, or a particular `riskGroup` field exists.

`collection_complete=true` 仅表示收到两次完整的 2xx JSON 响应。`verdicts_confirmed` 始终为 false，`risk_interpretation` 为 `unreviewed`。这是保留原始证据的工具；付款策略另见 `internal/probe/decision.go`。HTTP 200、空对象或缺少风险字段均不能直接解释为安全。

`collection_complete=true` only means two complete 2xx JSON responses were received. `verdicts_confirmed` remains false and `risk_interpretation` remains `unreviewed`. This is the raw-evidence tool; payment policy is implemented separately in `internal/probe/decision.go`. HTTP 200, an empty object, or missing risk fields must not automatically be interpreted as safe.

第一步完整验收 / Acceptance checklist for the complete first step:

- [x] 配置真实 key 和两个来源可追溯的主网地址。 / Configure a real key and two mainnet addresses with traceable provenance.
- [x] 实际执行两次请求，报告记录真实字段与耗时。 / Execute two live calls and record actual fields and elapsed time.
- [ ] 根据官方说明或工作人员回复确认风险与原因字段的含义。 / Confirm the meaning of risk and reason fields using official documentation or sponsor guidance.
- [x] 确认响应支持预期对照，否则换用官方确认的样例或继续排查。 / Verify that responses support the expected contrast; otherwise use sponsor-confirmed fixtures or investigate further.

这个端点没有文档列出的 `chainId` 请求参数，程序不自行添加。主网风险数据不能证明同地址在测试网的合约或资产安全。

The endpoint documentation does not list a `chainId` request parameter, so the tool does not invent one. Mainnet risk data does not establish that a contract or asset at the same address on a testnet is safe.

## 5. 各文件职责与流程 / File Responsibilities and Program Flow

以下路径相对于包含本 README 的 Git 仓库根目录。

All paths below are relative to the Git repository root containing this README.

| 文件 / File | 中文说明 | English explanation |
| --- | --- | --- |
| `go.mod` | 声明 Go 模块名称及 Go 版本要求。扫描代码使用标准库，付款代码依赖官方 x402 SDK。 | Declares the Go module and required Go version. The probe uses the standard library; payment uses the official x402 SDK. |
| `.env.example` | 空白模板，列出 key、两个地址及来源配置。 | Blank template listing the key, two addresses, and fixture source. |
| `.env` | 由模板复制得到的本地实际配置，不提交 Git。 | Local configuration copied from the template; excluded from Git. |
| `.buyer-key` | 可选测试网付款使用的本地测试钱包私钥文件，不提交 Git。 | Optional local test-wallet private key file for testnet payment; excluded from Git. |
| `artifacts/agents/` | 页面创建的 Agent 钱包密钥和模型设置，本地 0600 文件，不提交 Git。 | Local 0600 vault for UI-created agent wallet keys and model settings; excluded from Git. |
| `internal/agent/owner.go` | 钱包签名登录、创建 Agent、保存钱包和限制访问。 | Wallet-signature sign-in, agent creation, private wallet storage, and access checks. |
| `web/src/App.vue` | 连接 MetaMask、创建 Agent、充值、运行 Preview/Pay。 | Connects MetaMask, creates and funds agents, and runs Preview/Pay. |
| `.gitignore` | 忽略真实配置、运行报告、编译输出等本地文件。 | Excludes real configuration, run reports, build outputs, and other local files. |
| `cmd/intercepta-probe/main.go` | 程序入口：解析参数、加载配置、启动扫描和处理退出码。 | Entry point: parses flags, loads configuration, starts scanning, and handles exit codes. |
| `cmd/x402-demo/main.go` | 启动付费服务、查看 402 报价，或用测试钱包尝试付款。 | Starts the paid service, inspects a 402 offer, or attempts a test-wallet payment. |
| `cmd/x402-demo/main_test.go` | 测试付款前报价检查及测试钱包文件权限。 | Tests pre-payment offer checks and test-wallet file permissions. |
| `internal/probe/config.go` | 读取配置，检查 key、地址格式及两个地址是否重复。 | Loads configuration and checks the key, address format, and duplicate fixtures. |
| `internal/probe/client.go` | **实际 API 调用位于 `Client.Scan`**，处理鉴权、超时、状态码、正文和 key 脱敏。 | **The actual API call is in `Client.Scan`**, which handles authentication, timeouts, status codes, response bodies, and key redaction. |
| `internal/probe/decision.go` | 把真实风险字段转换为本项目的 allow / deny / hold 策略。 | Maps real risk fields to the project's allow / deny / hold policy. |
| `cmd/x402-demo/gate.go` | 签名前检查报价、扫描同一 payTo、输出原因并控制是否继续。 | Validates the quote, scans the same payTo before signing, logs reasons, and gates continuation. |
| `cmd/x402-demo/gate_test.go` | 验证 SDK 确实在生成授权前阻止；含可选真实 API 测试。 | Checks SDK abortion before authorization creation; includes opt-in live API tests. |
| `internal/probe/decision_test.go` | 验证风险字段、异常响应及失败时暂停策略。 | Tests risk fields, invalid responses, and fail-closed behavior. |
| `docs/step3-intercepta-gate.md` | 正常放行和风险阻断的操作步骤、策略与验证边界。 | Allow/block demo steps, policy, and validation limits. |
| `internal/probe/report.go` | 安排两次扫描，列出实际 JSON 字段，并保存响应及汇总。 | Orchestrates both scans, inventories actual JSON fields, and saves responses and reports. |
| `internal/probe/probe_test.go` | 使用本地 HTTP 服务和合成响应测试行为，不消耗 API 配额。 | Tests behavior with local HTTP servers and synthetic responses; consumes no API quota. |
| `README.md` | 使用方法、文件职责、结果解读及当前功能边界。 | Usage instructions, file responsibilities, result interpretation, and current limitations. |
| `docs/step1-validation.md` | 第一步真实扫描结果与验证边界。 | Records live probe results and validation limits. |
| `docs/step2-x402.md` | 第二步的简明操作说明和已验证／未验证状态。 | Short Step 2 instructions and observed/pending validation status. |

主要调用流程 / Main call flow:

```text
main.go — 程序入口 / entry point
  ├── config.go — 读取并检查配置 / load and validate configuration
  └── report.go — 安排两次扫描 / orchestrate two scans
        ├── client.go — 查询正常样例 / query the normal fixture
        ├── client.go — 查询风险样例 / query the risk fixture
        └── artifacts/ — 保存响应与报告 / save responses and reports
```

阅读代码时，先看 `main.go` 了解流程，再看 `client.go` 了解 API 请求，最后看 `report.go` 了解结果保存。

Start with `main.go` to understand the flow, then read `client.go` for the API request and `report.go` for result storage.

运行本地检查 / Run local checks:

```sh
go test ./...
go vet ./...
```

测试覆盖请求路径／鉴权头、两次结果保存、大整数、JSON Pointer、401/403/429/503、无效 JSON、超时、禁止重定向、响应大小上限、意外 key 回显脱敏、配置错误，以及第二次失败保留第一次证据。

Tests cover the request path and authentication header, saving both results, large integers, JSON Pointer, HTTP 401/403/429/503, invalid JSON, timeouts, redirect refusal, response size limits, redaction of an echoed key, configuration errors, and retaining the first result if the second call fails.

## 6. 提交时的 API 反馈 / API Feedback for Submission

以下反馈基于本次实际接入；未测量或未确认的事项如实标注。

The feedback below reflects this integration. Unmeasured or unconfirmed items are explicitly marked.

| 反馈 / Feedback | 中文 | English |
| --- | --- | --- |
| Time to first call | 从收到 key 到首次成功调用的开发耗时未记录。 | Development time from receiving the key to the first successful call was not recorded. |
| Request observations | 本轮两个样例均 HTTP 200，耗时约 1.82 秒／3.15 秒，见第三步记录。 | Both returned HTTP 200, taking about 1.82s / 3.15s in this run; see Step 3 evidence. |
| Confusing behavior | 主网风险数据与测试网支付的关系容易混淆；quick-scan 未列出 chainId 参数，逐链覆盖范围仍需确认。 | Mainnet risk data versus testnet payment was confusing; quick-scan lists no chainId parameter, and per-chain coverage needs clarification. |
| Missing information | 建议提供带 toxicScore / traits 的完整响应样例，以及超时、字段缺失时的处理建议。 | Suggested additions: complete toxicScore / traits response examples and guidance for timeouts and missing fields. |

原始报告默认被 Git 忽略。分享前检查内容，选取需要公开的证据。后续前端使用 TypeScript + Vue 3，合约如需要使用 Solidity；当前步骤是 Go 命令行接入验证。

Raw reports are excluded from Git by default. Review them before sharing and select the evidence to publish. The planned frontend uses TypeScript and Vue 3, with Solidity for contracts if needed. The current step is a Go command-line integration probe.


## 官方来源 / Official Sources

- [Quick Scan Address — 地址快速扫描](https://docs.web3antivirus.io/reference/quick-scan-address)
- [Getting Started — 接入指南](https://docs.web3antivirus.io/reference/getting-started-1)
- [ETHGlobal Tokyo: Intercepta — 赞助商奖项要求](https://ethglobal.com/events/tokyo2026/prizes/intercepta)

开发辅助披露 / Development assistance: [AI 使用说明](docs/ai-assistance.md).
