# 钱包：第一、二阶段改进

[English](Wallet.en.md) · 2026-10-05

## 第二阶段：用户控制

| 改了什么 | 现在怎样 |
|---|---|
| 智能钱包 | 新建 Agent 使用 ERC-1271 合约钱包；MetaMask 是唯一主控。当前没有接 ERC-4337 的 EntryPoint 或打包服务。 |
| 一次授权 | 用户用 MetaMask 签一笔授权交易，设置收款白名单、单笔额度、每日额度、有效期。首次部署另需一笔交易。 |
| 自动付款 | 后台只保存加密的受限钥匙。模型不能拿到它；主控私钥始终留在 MetaMask。付款仍要通过报价、风险和任务预算检查。 |
| 每日额度 | 每次付款先在链上占用额度，再签 x402 授权。多个任务共用当天额度；更新授权不会清零。 |
| 撤销 | `Revoke onchain` 由 MetaMask 发起。上链后，旧授权下尚未结算的付款签名失效；已经付出的款不能撤回。 |
| 提现 | `Withdraw with MetaMask` 把智能钱包的 USDC 转回主控地址；后台受限钥匙不能提现，也不能换收款地址。 |
| 后台停机时 | 保存页面上的 `Save standalone owner controls` 文件和合约地址。独立页面通过 MetaMask 撤销、提现，不调用项目后台。 |
| 备份 | 下载的 keystore 只备份受限钥匙。换电脑后选择 `Smart wallet session key` 恢复，再填写原合约地址绑定；MetaMask 的恢复资料要另行保管。 |
| 旧钱包 | 旧 EOA 地址、余额和加密文件保留，不会自动变成智能钱包。需要新建智能钱包，再由用户转入资金；旧钱包仍是第一阶段的保管方式。 |

| 怎么用 | 操作 |
|---|---|
| 1. 创建 | MetaMask 准备 Base Sepolia 测试 ETH → 连接钱包 → 填 Agent 信息和加密口令 → `Create agent + session key`。 |
| 2. 部署 | 点 `Deploy with MetaMask`，确认 Base Sepolia 部署交易；页面确认合约代码、主控地址和 USDC 合约后绑定。已有合约用 `Link existing wallet`。 |
| 3. 充值 | `Fund with MetaMask` 给智能钱包充测试 USDC；`Fund session gas` 给受限钥匙地址充少量测试 ETH，用于占用额度的交易。不要给这个地址充 USDC。 |
| 4. 授权 | 填商家收款地址、单笔／每日额度和有效期 → `Authorize with MetaMask` → 等待上链。当前页面单笔最多 0.10 USDC，每日最多 100 USDC，有效期最多 30 天。 |
| 5. 购买 | 输入口令解锁受限钥匙（最长 10 分钟），再提交付款任务。链上授权和本地解锁都有效时，才能自动付款。 |
| 6. 停止／取回 | 停止新签名用 `Lock now`；作废链上授权用 `Revoke onchain`；取回 USDC 用 `Withdraw with MetaMask`。 |
| 部署超时 | 先通过交易链接查结果；成功后复制合约地址并绑定，不要直接重复部署。 |
| 启动后台 | 默认 RPC 为 `https://sepolia.base.org`；可用进程环境变量 `DECISION402_RPC_URL` 换成自己的 Base Sepolia RPC。链不匹配时拒绝操作。 |

| 要注意什么 | 具体情况 |
|---|---|
| 日期边界 | 每日额度按 UTC 零点重置，即北京时间 08:00。昨天预留、尚未结算的签名，跨日后失效。 |
| 失败与费用 | 已占用的额度当天不退回，即使后来没买成。每次购买多一笔链上交易，需要测试 ETH。受限钥匙能控制其自身的 ETH，所以只充少量手续费。 |
| 付款中断 | 占用额度的交易发送前保存哈希；签付款前另存付款记录。手续费不足或占用记录写入失败，不会误记为已签付款。USDC 付款结果未知时，仍会阻止后续付款。 |
| Circle 的区别 | Circle 使用 MPC 密钥份额；这版使用“MetaMask 主控 + 受限钥匙”，没有实现 Circle 的 MPC。[Circle 说明](https://developers.circle.com/agent-stack/agent-wallets) |
| 使用范围 | 本地测试已覆盖合约和 x402 扣款；尚未用用户的 MetaMask 在公共 Base Sepolia 部署，也未验证公共 facilitator 的完整付款。只用于测试，未经过独立合约审计。 |

| 检查 | 结果 |
|---|---|
| 合约测试 | 白名单、单笔／每日限额、过期、跨日、重复付款、换授权不清零、撤销旧签名、非主控不能提现。 |
| x402 本地实付 | Anvil 本地区块链：HTTP 402 → 链上占额度 → 合约验签 → 测试代币扣款 → 返回结算结果，已通过。 |
| 后端 | 完整 Go 测试及 race 检查通过，包含智能钱包备份恢复、越权请求、错误链拒绝和原有钱包回归测试。 |
| 页面 | 构建通过；使用模拟 MetaMask 和 Anvil，验证了创建、部署、绑定、充值、授权、撤销、提现及独立控制页提现。没有操作真实钱包。 |

代码：[`DecisionWallet.sol`](../../../../../contracts/src/DecisionWallet.sol) · [`smart_wallet.go`](../../../../../internal/agent/smart_wallet.go) · [`smart_signer.go`](../../../../../internal/agent/smart_signer.go) · [`smart_wallet_test.go`](../../../../../internal/agent/smart_wallet_test.go)

复查命令：在 `contracts` 执行 `npm ci --ignore-scripts`、`forge test`；项目根目录执行 `DECISION402_SMART_TEST=1 DECISION402_LIVE_TEST=0 go test -race ./...` 和 `npm --prefix web run build`。本地链测试需要 Anvil；合约使用 Solidity 0.8.37、OpenZeppelin 5.4.0。修改合约后运行 `python3 contracts/scripts/export.py` 更新内嵌 ABI 和字节码。

## 第一阶段：加密与锁定（旧 EOA 钱包）

| 改了什么 | 现在怎样 |
|---|---|
| 私钥加密 | 用 go-ethereum V3 keystore 和用户口令加密；文件权限仍为 `0600`。口令不保存。 |
| DeepSeek Key | 个人 Key 用同一口令加密，不再明文写进 Agent JSON。共用 Key 只从进程环境变量 `DEEPSEEK_API_KEY` 读取。 |
| 锁定／解锁 | 默认锁定；解锁最长 10 分钟，只对当前登录会话生效。手动锁定、退出登录、到期或重启后不能再签名。 |
| 签名 | 报价、预算和风险检查通过后才签；签名时临时解密私钥，用完清理。锁定也能挡住已开始、尚未签名的任务。 |
| 备份／恢复 | 输入口令导出加密 keystore；用文件和原口令恢复同一个钱包地址。钱包备份不含模型 Key，恢复时重新填写。 |
| 旧钱包迁移 | 页面点击 `Encrypt old wallet`，输入两遍新口令。先验证加密文件能解开，再保存并删除旧 `.key`；地址和个人模型 Key 保留。中途退出可继续迁移。 |
| 防止泄露 | 接口只返回公开资料；口令和 Key 输入框遮挡内容，操作后清空，不写入浏览器存储。模型响应若包含 API Key，直接丢弃。 |

| 怎么用 | 操作 |
|---|---|
| 新 Agent | 连接钱包 → 填信息和口令 → 创建 → 充值测试币 → 解锁 → 执行任务。 |
| 已有 Agent | 先点 `Encrypt old wallet`；迁移前不能执行任务。旧文件不会仅因更新代码就自动加密。 |
| 换电脑 | 下载加密备份并单独记好口令 → 新电脑连接钱包 → 选择备份文件 → 输入原口令和模型设置 → `Restore wallet`。忘记口令无法恢复。 |
| 原来在 `.env` 放了 DeepSeek Key | 改为页面输入个人 Key，或配置进程环境变量；自行移除旧 `.env` 中的值。其他 `.env` 设置照常使用。 |

| 检查 | 结果 |
|---|---|
| 后端测试 | `DECISION402_LIVE_TEST=0 go test ./...` 通过；使用本机模拟服务，没有真实付款。 |
| 并发测试 | `DECISION402_LIVE_TEST=0 go test -race ./internal/agent` 通过。覆盖锁定与签名同时发生、锁定取消等待中的解锁。 |
| 恢复和权限 | 错误口令、其他用户、其他登录会话均被拒绝；备份恢复后能签名；重启锁定；迁移中断后能继续。 |
| 付款检查 | 报价变化、风险不符、写记录失败、签名前锁定都不能付款；正常路径仍通过 x402 模拟卖方测试。 |
| 页面 | 构建通过；用本地测试钱包操作过创建、错误口令、解锁和锁定，检查了输入清空及按钮状态。备份接口及恢复通过测试；内置浏览器未确认下载文件落盘。 |

| 还要知道 | 具体限制 |
|---|---|
| 解锁期间 | 服务器内存保留临时解密能力，用户口令不保留。锁定清理这份能力；Go 无法保证擦除运行库产生的所有内存副本。 |
| 已经签名的付款 | 锁定不能撤回它，仍要继续核对付款结果。 |
| 旧 CLI | 独立的 `cmd/x402-demo` 仍是早期演示；它的 `.buyer-key`／`.seller-key` 没有迁入 Agent 钱包。新版主程序不读取这些文件。 |

代码：[`vault.go`](../../../../../internal/agent/vault.go) · [`wallet_api.go`](../../../../../internal/agent/wallet_api.go) · [`vault_test.go`](../../../../../internal/agent/vault_test.go) · [`App.vue`](../../../../../web/src/App.vue)
