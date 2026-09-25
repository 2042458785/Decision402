# Decision402 — 第一步：Intercepta API 调用验证 / Step 1: Intercepta Live API Probe

本 README 按章节提供中英对照。命令和代码示例共用一份。

This README provides Chinese and English explanations in each section. Commands and code examples are shared.

用 Go 分别扫描一个「预期正常」和一个「预期有风险」的主网地址，保存真实响应、字段、HTTP 状态和客户端耗时，为之后的 x402 签名前检查提供实际接口依据。

This Go tool scans two mainnet addresses: one expected to be normal and one expected to be risky. It records actual responses, JSON fields, HTTP status codes, and client-side elapsed time to inform a future x402 check before signing.

**当前阶段：调用程序与本地行为测试。真实正常／风险结论，必须等拿到 key、官方样例并完成实际调用后确认。** 本地测试使用合成响应，不是赞助商 API 调用证据。

**Current stage: a request collection tool with local behavior tests. Actual normal/risky findings remain unconfirmed until a real key and sponsor-provided fixtures are used in live calls.** Local tests use synthetic responses and are not evidence of live sponsor API integration.

## 1. 领取并保存配置 / Obtain and Save Configuration

从 [Intercepta 活动入口 / event page](https://intercepta.io/ethglobal) 领取 sandbox key。向展位工作人员或活动 Discord 索取一个正常主网地址和一个已知风险主网地址，并记录来源。文档里的示例地址不自动等于正常样例。

Obtain a sandbox key from the event page. Ask the sponsor booth or event Discord for a normal mainnet address and a known-risk mainnet address, and record their source. An address shown in the documentation is not automatically a normal test fixture.

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
| `INTERCEPTA_NORMAL_ADDRESS` | 赞助商提供的预期正常主网地址。 | A sponsor-provided mainnet address expected to be normal. |
| `INTERCEPTA_RISK_ADDRESS` | 赞助商提供的已知风险主网地址。 | A sponsor-provided mainnet address with known risk. |
| `INTERCEPTA_ADDRESS_SOURCE` | 样例来源，例如展位工作人员或 Discord 消息链接。 | Fixture provenance, such as the sponsor booth or a Discord message URL. |

Key 只保存在本地，不贴到聊天或提交到仓库。程序不需要钱包私钥。

Keep the key local; do not paste it into chat or commit it to the repository. The tool does not require a wallet private key.

`.env` 支持空行、整行 `#` 注释、`NAME=value` 和一对外围引号；不执行 shell，不展开 `$变量`，不支持行尾注释或多行值。已有进程环境变量优先。两个地址必须不同，使用 `0x` 开头的 40 位十六进制地址；本步骤不做 ENS 解析或地址校验和验证。

The `.env` parser supports blank lines, full-line `#` comments, `NAME=value`, and one pair of surrounding quotes. It does not execute shell code, expand `$variables`, or support inline comments or multiline values. Existing process environment variables take precedence. The two addresses must differ and contain `0x` followed by 40 hexadecimal characters. ENS resolution and address checksum validation are not implemented in this step.

## 2. 运行两次真实请求 / Run Two Live Requests

需要 Go 1.23 或更新版本。只使用 Go 标准库，无需下载第三方依赖。

Requires Go 1.23 or later. The tool uses only the Go standard library; no third-party dependencies are needed.

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
| Normal/risky fixture findings | 需要真实响应和字段含义，目前未确认。 | Requires live responses and confirmed field semantics; not yet established. |
| Full sponsor prize eligibility | 本步骤不能支持，后续还要接入实际付款前的决策。 | This step alone does not establish eligibility; checks must later control an actual payment flow. |

`normal` / `risk` 是你提供样例时的**预期标签**。程序从不把标签当作 API 结论，也不假设存在 `allow`、`deny` 或某个特定的 `riskGroup` 字段。

`normal` / `risk` are **expected fixture labels supplied by you**. The program never treats them as API verdicts and does not assume that `allow`, `deny`, or a particular `riskGroup` field exists.

`collection_complete=true` 仅表示收到两次完整的 2xx JSON 响应。`verdicts_confirmed` 始终为 false，`risk_interpretation` 为 `unreviewed`。拿到真实字段后，再设计经过验证的策略映射。HTTP 200、空对象或缺少风险字段均不能直接解释为安全。

`collection_complete=true` only means two complete 2xx JSON responses were received. `verdicts_confirmed` remains false and `risk_interpretation` remains `unreviewed`. Build a validated policy mapping after inspecting real fields. HTTP 200, an empty object, or missing risk fields must not automatically be interpreted as safe.

第一步完整验收 / Acceptance checklist for the complete first step:

- [ ] 配置真实 key 和两个来源可追溯的主网地址。 / Configure a real key and two mainnet addresses with traceable provenance.
- [ ] 实际执行两次请求，报告记录真实字段与耗时。 / Execute two live calls and record actual fields and elapsed time.
- [ ] 根据官方说明或工作人员回复确认风险与原因字段的含义。 / Confirm the meaning of risk and reason fields using official documentation or sponsor guidance.
- [ ] 确认响应支持预期对照，否则换用官方确认的样例或继续排查。 / Verify that responses support the expected contrast; otherwise use sponsor-confirmed fixtures or investigate further.

这个端点没有文档列出的 `chainId` 请求参数，程序不自行添加。主网风险数据不能证明同地址在测试网的合约或资产安全。

The endpoint documentation does not list a `chainId` request parameter, so the tool does not invent one. Mainnet risk data does not establish that a contract or asset at the same address on a testnet is safe.

## 5. 各文件职责与流程 / File Responsibilities and Program Flow

以下路径相对于包含本 README 的 Git 仓库根目录。

All paths below are relative to the Git repository root containing this README.

| 文件 / File | 中文说明 | English explanation |
| --- | --- | --- |
| `go.mod` | 声明 Go 模块名称及 Go 版本要求。目前只使用标准库。 | Declares the Go module and required Go version. Only the standard library is used. |
| `.env.example` | 空白模板，列出 key、两个地址及来源配置。 | Blank template listing the key, two addresses, and fixture source. |
| `.env` | 由模板复制得到的本地实际配置，不提交 Git。 | Local configuration copied from the template; excluded from Git. |
| `.gitignore` | 忽略真实配置、运行报告、编译输出等本地文件。 | Excludes real configuration, run reports, build outputs, and other local files. |
| `cmd/intercepta-probe/main.go` | 程序入口：解析参数、加载配置、启动扫描和处理退出码。 | Entry point: parses flags, loads configuration, starts scanning, and handles exit codes. |
| `internal/probe/config.go` | 读取配置，检查 key、地址格式及两个地址是否重复。 | Loads configuration and checks the key, address format, and duplicate fixtures. |
| `internal/probe/client.go` | **实际 API 调用位于 `Client.Scan`**，处理鉴权、超时、状态码、正文和 key 脱敏。 | **The actual API call is in `Client.Scan`**, which handles authentication, timeouts, status codes, response bodies, and key redaction. |
| `internal/probe/report.go` | 安排两次扫描，列出实际 JSON 字段，并保存响应及汇总。 | Orchestrates both scans, inventories actual JSON fields, and saves responses and reports. |
| `internal/probe/probe_test.go` | 使用本地 HTTP 服务和合成响应测试行为，不消耗 API 配额。 | Tests behavior with local HTTP servers and synthetic responses; consumes no API quota. |
| `README.md` | 使用方法、文件职责、结果解读及当前功能边界。 | Usage instructions, file responsibilities, result interpretation, and current limitations. |
| `docs/step1-validation.md` | 已通过的检查及尚未完成的真实 API 验证记录。 | Records completed checks and outstanding live API validation. |

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

以下四项待真实调用后填写，尚不是实际体验。提交前替换为真实记录。

Complete the four items below after live calls. They are placeholders, not observed experience; replace them with real findings before submission.

| 反馈 / Feedback | 中文：待填写 | English: to be completed |
| --- | --- | --- |
| Time to first call | 从收到 key 到首次成功调用耗时：待实测。 | Time from receiving the key to the first successful call: not yet measured. |
| Request observations | 两个样例的客户端耗时及 HTTP 状态：待实测，引用报告。 | Client-side duration and HTTP status for both fixtures: not yet measured; reference the report. |
| Confusing behavior | 最困惑的字段／行为及工作人员解释：待记录。 | Confusing fields or behavior and the sponsor's explanation: to be recorded. |
| Missing information | 缺少的文档／能力或具体建议：待记录，没有则如实填写。 | Missing documentation, capabilities, or specific suggestions: to be recorded; state honestly if none. |

原始报告默认被 Git 忽略。分享前检查内容，选取需要公开的证据。后续前端使用 TypeScript + Vue 3，合约如需要使用 Solidity；当前步骤是 Go 命令行接入验证。

Raw reports are excluded from Git by default. Review them before sharing and select the evidence to publish. The planned frontend uses TypeScript and Vue 3, with Solidity for contracts if needed. The current step is a Go command-line integration probe.


## 官方来源 / Official Sources

- [Quick Scan Address — 地址快速扫描](https://docs.web3antivirus.io/reference/quick-scan-address)
- [Getting Started — 接入指南](https://docs.web3antivirus.io/reference/getting-started-1)
- [ETHGlobal Tokyo: Intercepta — 赞助商奖项要求](https://ethglobal.com/events/tokyo2026/prizes/intercepta)
