# What to Build Next and Which Ideas to Try

Updated: 2026-10-03. [中文](<Think(SomeIdeas).md>)

## Direction first

**Make one real purchasing flow reliable before adding complex features.** We build our wallets, budgets, payments, and provider selection. Circle is a learning reference; we do not use its hosted services.

- Five responsibilities: set user rules, find services, select by rules, check risk, and execute payment. See [V1.0](<../Version/V1.0(ETHGlobalTokyo2026Hackathon)/V1.0.en.md>) for current behavior and [HowToImprove](../HowToImprove/HowToImprove.en.md) for tasks.
- Separate responsibilities inside the existing Go project first, without splitting into multiple services. Keep the full purchasing flow working after each change.
- Also improve key storage, cumulative spending records, failure recovery, and data checks. The interceptor is one part of risk checking, not the whole product. Its data source can be replaced later.
- Compare Circle using four questions: what do we do now, what does it offer, what is missing, and how will we check the change? It already has sanctions screening, so risk interception alone is not unique to us.
- Complex innovation can wait, but real tasks must be found early. Completing the feature list does not establish demand.

## Five ideas to try

Build the first two with the main plan. Consider the others once purchasing works reliably.

| Idea | What to do | How to check its value |
|---|---|---|
| Improve existing preview | Show provider, price, risk, and selection reason without signing or paying | Two developers understand it and want to continue trying it |
| Explain purchase results | Show what was bought, cost, data checks, and failure reasons | Users can understand failure records without our help |
| Let other agents call it | Offer one API that takes tasks and returns data and cost, within existing authorization | One developer can connect it to their own project |
| Track provider results | Record success, stale data, price, and time before changing selection | Records explain problems and later changes improve results |
| Share a demo for feedback | Show actual capabilities; record each task, problem, and reproduction steps | Feedback comes from real tasks, and users return |

## When to adjust

- No two services with suitable functions and payment methods within two days: change the use case before building a large marketplace.
- One fixed provider is enough: prioritize budgets and payment recovery over complex AI selection.
- Existing products already meet the need: ask what is missing before deciding what to build.

**Confidence so far:** Code limitations have evidence; these product ideas still need user trials. First check whether they reduce failures, manual work, and wasted spending. Commercial success is not established.
