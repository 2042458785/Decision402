# Service Discovery: Current State and Improvement Plan

[中文](<Service Discovery.md>)

## How it works now

| Step | Current behavior |
|---|---|
| Understand the task | DeepSeek calls `find_services`; only Tokyo weather requests are accepted. |
| Find sellers | The backend uses local A/B/C/D entries hard-coded in `app.go`; it does not search external sellers. All four return a fixed “Tokyo 23°C” sample. |
| Read quotes | Simulation uses made-up quotes. Preview and payment send GET to the local endpoints and read HTTP 402. The code accepts only one payment option. |
| Choose a seller | Go checks the quote, screens the payee, and ranks candidates by budget and price/risk preference. The model cannot set the payee. |

Code: [seller list and discovery](<../../../../../internal/agent/app.go>) · [model tools](<../../../../../internal/agent/model.go>) · [quote checks](<../../../../../internal/agent/payment.go>)

## Current problems

| Problem | What happens |
|---|---|
| No real data | All four sellers return a fixed sample, so they cannot answer a real weather question. |
| Tokyo weather only | The tool cannot find another place or kind of service. |
| Sellers are hard-coded | Adding one requires a code change; the list has no service type or request parameters. |
| Limited quote support | Quote requests use GET and require exactly one payment option. Some real sellers require POST or offer multiple payment methods. |

## Planned improvements

| Change | How | Done when |
|---|---|---|
| Add the first real seller | Pick one task; check the endpoint, inputs, HTTP 402 quote, and Base Sepolia test-USDC support; make a small test purchase. Weather can test payment integration first. | The paid response contains real data with the right place, time, and required fields. A 402 quote alone is not enough. |
| Keep our own service list | Record seller, service type, endpoint, GET/POST method, inputs, and supported payment methods. | The backend loads sellers from the list. The live 402 quote remains authoritative for payment. |
| Match requests to services | The model identifies the need; the backend searches the integrated list. | If none matches, it says “not supported yet” without inventing a seller or paying. |
| Handle real quotes | Support the required GET/POST request. With multiple payment options, choose only one matching the current chain, USDC, and budget; recheck before signing. | A mismatched chain, token, amount, or payee stops payment. |
| Add another seller for the same need | After the first works, integrate a second seller providing the same kind of data. | Both deliver comparable data before the existing price/risk rules compare them. |
| Use catalogs for leads | Search the [Circle service directory](https://developers.circle.com/agent-stack/agent-marketplace/discovery-api), then verify each seller. | A listing does not prove our wallet can pay or that the data is useful. |
