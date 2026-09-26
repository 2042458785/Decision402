# 第二步：跑通 x402 报价 / Step 2: See an x402 Payment Offer

你现在可以先完成前两步，**不需要 Intercepta API key，也不需要钱包私钥**：启动一个付费数据服务，然后用购买端查看它的报价。真正付款是可选的第三步。

You can complete the first two actions **without an Intercepta API key or a wallet private key**: start a paid data service, then inspect its payment offer. Making a real testnet payment is an optional third action.

这个服务返回的是**静态演示数据**，不是实时天气。它只用于跑通 x402 流程。

The service returns **static demo data**, not live weather. It exists to test the x402 flow.

## 1. 启动卖方 / Start the Seller

打开第一个终端，在 Git 仓库根目录执行：

Open the first terminal in the Git repository root:

```sh
go run ./cmd/x402-demo -mode serve -pay-to 0x1111111111111111111111111111111111111111
```

`-pay-to` 是收款地址。上面的 `0x111…111` **只是示例**：只看报价时可暂用；实际付款前须换成你控制的 Base Sepolia 收款地址。保持这个终端运行。服务默认在 `127.0.0.1:4021`，访问 `/data` 需要付款。

`-pay-to` is the receiving address. `0x111…111` above is **only an example**: you may use it when inspecting the offer, but replace it with a Base Sepolia receiving address you control before making a real payment. Leave this terminal running. The service listens on `127.0.0.1:4021` by default, and `/data` requires payment.

## 2. 只看报价 / Inspect the Offer

打开第二个终端，仍在同一仓库根目录执行：

Open a second terminal in the same repository root:

```sh
go run ./cmd/x402-demo -mode inspect
```

预期看到 **HTTP 402**，以及 `network: eip155:84532`、USDC、`amount: 1000` 和 `payTo`。这里的 `1000` 是 USDC 最小单位，即 **0.001 USDC**。检查 `payTo` 是否与第一步的地址一致。**inspect 只看报价，不会签名或付款。**

Expect **HTTP 402** with `network: eip155:84532`, USDC, `amount: 1000`, and `payTo`. The amount is in USDC's smallest units: **0.001 USDC**. Check that `payTo` matches the address from step 1. **Inspect only reads the offer; it does not sign or pay.**

这条 402 报价已在 2026-09-26 JST 用官方 x402 Go SDK 实际跑出；单独一条报价不能证明付款成功。后续 Agent 版本已完成测试网付款，见[第四步验证记录](step4-validation.md)。

We observed this 402 offer with the official x402 Go SDK on 2026-09-26 JST. An offer alone does not establish payment; the subsequent Agent payment is documented in [Step 4 validation](step4-validation.md).

## 3. 可选：测试网付款 / Optional: Make a Testnet Payment

只有当你备好**单独的测试钱包**和 Base Sepolia 测试 USDC 时，才执行这一步。用本地编辑器把该测试钱包的十六进制私钥写进仓库根目录的 `.buyer-key`，文件中只放私钥，不要加 `KEY=`。然后运行：

Only do this after preparing a **separate test wallet** funded with Base Sepolia test USDC. In a local editor, put that wallet's hex private key in `.buyer-key` at the repository root. The file should contain only the key, without `KEY=`. Then run:

```sh
chmod 600 .buyer-key
go run ./cmd/x402-demo -mode pay -pay-to 0x1111111111111111111111111111111111111111
```

把 `-pay-to` 换成第一步相同的收款地址。购买端会在签名前检查网络、代币、收款地址和金额，并使用 `.env` 中的 API key 调用 Intercepta；请先完成配置。只有看到 **HTTP 200、成功结算回执和交易哈希**，才能说测试网付款跑通。`.buyer-key` 已被 `.gitignore` 忽略，不要提交或发送给别人。

Replace `-pay-to` with the same receiving address used in step 1. Before signing, the buyer checks the network, token, recipient, and amount, then calls Intercepta using the API key in `.env`; configure it first. Only **HTTP 200, a successful settlement receipt, and a transaction hash** establish that the testnet payment completed. `.buyer-key` is excluded by `.gitignore`; do not commit or share it.

## 现在的进度与下一步 / Current Status and Next Action

此前已完成一次真实 Base Sepolia 测试 USDC 结算。现已加入 Intercepta 签名前检查：正常／风险真实扫描已通过 SDK 钩子测试，新版本完整付款仍待实测。接下来按照[第三步操作说明](step3-intercepta-gate.md)演示放行和阻断。

An earlier Base Sepolia test-USDC payment settled successfully. Live Intercepta scans pass allow/block tests in the pre-signing SDK hook. The Agent version completed a screened testnet payment; this standalone CLI mode has not been rerun with the integration. Follow the [Step 3 guide](step3-intercepta-gate.md) next.

项目使用官方 x402 Go SDK，需要 Go 1.24 或更新版本。本机 Go 1.23.2 在 `GOTOOLCHAIN=auto` 下可能自动下载适用工具链。

The project uses the official x402 Go SDK and requires Go 1.24 or later. With local Go 1.23.2, `GOTOOLCHAIN=auto` may download a compatible toolchain automatically.

官方资料 / Official guides: [Seller quickstart](https://docs.x402.org/getting-started/quickstart-for-sellers), [Buyer quickstart](https://docs.x402.org/getting-started/quickstart-for-buyers).
