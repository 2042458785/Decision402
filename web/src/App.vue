<script setup lang="ts">
// The interface for the Decision402 agent. Every decision about money is made
// by the Go backend; this reads a task and shows what happened, step by step.
//
// The logic here is unchanged from the original: the same request shape, the
// same polling, the same retry rules. What changed is the presentation — the
// run reads as a sequence, and the decision leads.
import { computed, onMounted, onUnmounted, ref } from "vue";

type Policy = { per_payment: string; task_budget: string; max_risk: number; preference: string };
type Request = { id: string; instruction: string; mode: string; policy: Policy };
type Candidate = {
  service: { id: string; name: string; pay_to: string; amount: string };
  level: number;
  eligible: boolean;
  reason: string;
  source: string;
  quote?: { amount: string };
  risk?: { toxicScore?: number; traits?: { name: string; description: string }[]; duration_ms: number };
};
type Task = {
  request: Request;
  status: string;
  candidates: Candidate[];
  selected?: Candidate;
  events: { time: string; kind: string; data: unknown }[];
  summary: string;
  error?: string;
  payment_attempted: boolean;
  payment?: { settled: boolean; signed: boolean; transaction?: string; error?: string; data?: unknown };
};

const instruction = ref("Get me a Tokyo weather sample, choosing a provider within my budget and risk policy.");
const mode = ref("simulate");
const policy = ref<Policy>({ per_payment: "0.10", task_budget: "0.10", max_risk: 1, preference: "price" });
const task = ref<Task | null>(null),
  config = ref<{ model: string } | null>(null),
  error = ref(""),
  sending = ref(false);
const pending = ref<Request | null>(null);
let timer: ReturnType<typeof setInterval> | undefined;

const active = computed(() => sending.value || ["queued", "running"].includes(task.value?.status ?? ""));
const statusNames: Record<string, string> = {
  queued: "Queued",
  running: "Agent running",
  previewed: "Preview complete · nothing paid",
  settled: "Settled on testnet",
  held: "Held",
  unknown: "Settlement unconfirmed",
  error: "Failed",
};
const eventNames: Record<string, string> = {
  model: "Model understood the task and chose a tool",
  tool: "Tool call requested",
  candidate: "Quote and risk check",
  selection: "Policy selected a provider",
  final_risk: "Re-checked immediately before signing",
  authorization_reserved: "Payment authorisation reserved",
};
const money = (value: string) => (Number(value) / 1e6).toFixed(3);
const levels = ["no signal found", "advisory signal, authorised", "blocked by policy"];
const levelLabel = (n: number) => (n < 0 ? "unknown" : (levels[n] ?? String(n)));
const short = (a: string) => (a && a.length > 20 ? `${a.slice(0, 10)}…${a.slice(-8)}` : a);
const blockedPayment = computed(
  () => mode.value === "pay" && !!task.value?.payment_attempted && !task.value?.payment?.settled,
);

// Where the run has reached, for the sequence markers.
const stage = computed(() => {
  const t = task.value;
  if (!t) return -1;
  if (!["queued", "running"].includes(t.status)) return 4;
  if (t.selected) return 3;
  if (t.candidates.some((c) => c.risk)) return 2;
  if (t.candidates.length) return 1;
  return 0;
});
const beat = (i: number) => (stage.value > i ? "done" : stage.value === i ? "active" : "waiting");

// The headline. A preview that chose a provider would have paid, and says so
// rather than claiming a payment that never happened.
const verdict = computed(() => {
  const t = task.value;
  if (!t) return { word: "Pending", tone: "pending" };
  if (["queued", "running"].includes(t.status)) return { word: "Pending", tone: "pending" };
  if (t.payment?.settled) return { word: "Paid", tone: "ok" };
  if (t.selected) return { word: t.request.mode === "pay" ? "Authorised" : "Would authorise", tone: "ok" };
  if (t.candidates.some((c) => c.level >= 2)) return { word: "Blocked", tone: "bad" };
  return { word: "Held", tone: "warn" };
});

const excluded = computed(() => (task.value?.candidates ?? []).filter((c) => c.risk && !c.eligible));
const eligible = computed(() => (task.value?.candidates ?? []).filter((c) => c.eligible));

function onModeChange() {
  if (mode.value !== "simulate") policy.value.max_risk = 0;
}
async function getTask(id: string) {
  const response = await fetch("/api/tasks/" + encodeURIComponent(id));
  if (!response.ok)
    throw new Error(response.status === 404 ? "That task no longer exists. Send the request again." : "Could not read the task.");
  task.value = await response.json();
  if (!["queued", "running"].includes(task.value!.status)) {
    clearInterval(timer);
    pending.value = null;
    localStorage.removeItem("decision402-pending");
  }
}
function watchTask(id: string) {
  clearInterval(timer);
  // The backend moves through quoting, screening and policy in well under a
  // second each. A 1500ms poll could miss a whole stage: the sequence would
  // jump from quoting straight to the decision, and the step in between would
  // never be seen working. Sampling more often lets each step show its state.
  //
  // Each poll is scheduled only once the previous one has answered, so a slow
  // response delays the next request instead of stacking another on top of it.
  const tick = () => {
    getTask(id)
      .then(() => {
        if (["queued", "running"].includes(task.value?.status ?? "")) {
          timer = setTimeout(tick, 450);
        }
      })
      .catch((e) => {
        error.value = e.message;
      });
  };
  timer = setTimeout(tick, 250);
}
async function start() {
  error.value = "";
  sending.value = true;
  try {
    // Retry an uncertain POST with the SAME immutable request ID and payload.
    const request =
      pending.value ?? {
        id: crypto.randomUUID(),
        instruction: instruction.value,
        mode: mode.value,
        policy: { ...policy.value, max_risk: mode.value === "simulate" ? policy.value.max_risk : 0 },
      };
    pending.value = request;
    localStorage.setItem("decision402-pending", JSON.stringify(request));
    localStorage.setItem("decision402-last", request.id);
    const response = await fetch("/api/tasks", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Decision402": "local-ui" },
      body: JSON.stringify(request),
    });
    const result = await response.json();
    if (!response.ok) {
      if (response.status < 500) {
        pending.value = null;
        localStorage.removeItem("decision402-pending");
      }
      throw new Error(result.error ?? "Could not create the task.");
    }
    task.value = result;
    history.replaceState(null, "", "?task=" + encodeURIComponent(request.id));
    if (["queued", "running"].includes(result.status)) watchTask(request.id);
    else {
      pending.value = null;
      localStorage.removeItem("decision402-pending");
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : "Connection failed; a retry reuses the same task ID.";
  } finally {
    sending.value = false;
  }
}
onMounted(async () => {
  try {
    const r = await fetch("/api/config");
    if (!r.ok) throw new Error("The backend is not running.");
    config.value = await r.json();
    const saved = localStorage.getItem("decision402-pending");
    if (saved) pending.value = JSON.parse(saved);
    const id = new URLSearchParams(location.search).get("task") ?? localStorage.getItem("decision402-last");
    if (id) {
      await getTask(id);
      if (task.value) {
        policy.value = {
          ...task.value.request.policy,
          max_risk: task.value.request.mode === "simulate" ? task.value.request.policy.max_risk : 0,
        };
        mode.value = task.value.request.mode;
        instruction.value = task.value.request.instruction;
      }
      if (["queued", "running"].includes(task.value?.status ?? "")) watchTask(id);
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : "Connection failed.";
  }
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <div class="site">
    <header class="site-header">
      <div class="header-inner">
        <a class="wordmark" href="/">decision<span>402</span></a>
        <div class="network-note">
          <span>Payment Network: <strong>Base Sepolia</strong></span>
          <span class="network-separator">/</span>
          <span>Risk Data: <strong>Mainnet</strong></span>
        </div>
      </div>
    </header>

    <main class="main">
      <div class="page-heading">
        <h1>Payment gate<span class="heading-period">.</span></h1>
      </div>

      <!-- what the agent is asked to do -->
      <div class="ask-bar">
        <form class="ask-field" @submit.prevent="start">
          <input v-model="instruction" :disabled="active || !!pending" maxlength="2000"
            placeholder="What should the agent buy?" aria-label="What should the agent buy" />
          <button type="submit" :disabled="active || !config || blockedPayment">
            {{ active ? "Working" : pending ? "Retry" : "Send" }}
            <span aria-hidden="true">→</span>
          </button>
        </form>
        <div class="ask-meta">
          <span>
            Up to <strong>{{ policy.per_payment }}</strong> USDC ·
            <strong>{{ policy.task_budget }}</strong> total · level ≤
            <strong>{{ policy.max_risk }}</strong> ·
            <strong>{{ policy.preference === "price" ? "price first" : "risk first" }}</strong>
          </span>
          <span class="model" v-if="config">model {{ config.model }}</span>
        </div>
        <p v-if="error" class="error-message" role="alert">{{ error }}</p>
        <p v-if="blockedPayment" class="error-message" role="alert">
          The previous task reserved an authorisation without a confirmed settlement.
          Check the record before paying again — this is not retried automatically.
        </p>
      </div>

      <!-- the run, as a sequence -->
      <section v-if="task" class="story">
        <ol class="beats">
          <li class="beat" :class="beat(0)">
            <span class="beat-rail" aria-hidden="true"><span class="beat-orb"><span class="orb-face"><template v-if="beat(0) === 'done'"><svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5" /></svg></template><template v-else>1</template></span></span></span>
            <div class="beat-body">
              <span class="beat-label">You asked for</span>
              <p class="said">{{ task.request.instruction }}</p>
              <p class="said-meta">
                {{ task.request.mode === "simulate" ? "Policy simulation · simulated quotes and risk"
                  : task.request.mode === "preview" ? "Live API preview · nothing signed"
                  : "Testnet execution · signs and pays" }}
              </p>
            </div>
          </li>

          <li class="beat" :class="beat(1)">
            <span class="beat-rail" aria-hidden="true"><span class="beat-orb"><span class="orb-face"><template v-if="beat(1) === 'done'"><svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5" /></svg></template><template v-else>2</template></span></span></span>
            <div class="beat-body">
              <span class="beat-label">Providers quoted</span>
              <ul v-if="task.candidates.length" class="quotes">
                <li v-for="c in task.candidates" :key="c.service.id"
                  :class="c.risk ? 'has-screen' : ''">
                  <span class="q-name">{{ c.service.name }}</span>
                  <span class="q-price">{{ money(c.quote?.amount ?? c.service.amount) }} <small>USDC</small></span>
                  <span class="q-addr"><code>{{ short(c.service.pay_to) }}</code></span>
                </li>
              </ul>
              <p v-else class="pending-line">Waiting for quotes. No recipient is known yet.</p>
            </div>
          </li>

          <li class="beat" :class="beat(2)">
            <span class="beat-rail" aria-hidden="true"><span class="beat-orb"><span class="orb-face"><template v-if="beat(2) === 'done'"><svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5" /></svg></template><template v-else>3</template></span></span></span>
            <div class="beat-body">
              <span class="beat-label">Recipients screened</span>
              <ul v-if="task.candidates.some((c) => c.risk)" class="screens">
                <li v-for="c in task.candidates" :key="c.service.id"
                  :class="c.risk ? (c.level < 0 ? 'unknown' : c.level >= 2 ? 'risky' : 'clear') : 'waiting'">
                  <span class="s-name">{{ c.service.name }}</span>
                  <span class="s-score">
                    <span v-if="c.risk?.toxicScore !== undefined" class="score-pop">{{ c.risk.toxicScore }}</span>
                    <em v-else-if="c.risk">no verdict</em>
                    <span v-else class="scan-bar" aria-label="screening" />
                  </span>
                  <span class="s-traits">
                    <code v-for="t in c.risk?.traits ?? []" :key="t.name">{{ t.name }}</code>
                  </span>
                </li>
              </ul>
              <p v-else class="pending-line">Checking each recipient's mainnet risk record…</p>
              <p v-if="task.candidates.some((c) => c.risk)" class="screened-line">
                Level 0 is {{ levelLabel(0) }}; level 2 is {{ levelLabel(2) }}.
              </p>
              <p class="risk-disclosure">
                Mainnet risk record only. Level 0 means no signal was found in this scan,
                which is not a guarantee of safety, and says nothing about Base Sepolia history.
              </p>
            </div>
          </li>

          <li class="beat" :class="beat(3)">
            <span class="beat-rail" aria-hidden="true"><span class="beat-orb"><span class="orb-face"><template v-if="beat(3) === 'done'"><svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5" /></svg></template><template v-else>4</template></span></span></span>
            <div class="beat-body">
              <span class="beat-label">Policy applied</span>
              <template v-if="excluded.length || eligible.length">
                <p v-for="c in excluded" :key="c.service.id" class="ruled-out">
                  <strong>{{ c.service.id }}</strong> {{ c.reason }}
                </p>
                <p v-if="eligible.length" class="ruled-in">
                  {{ eligible.length }} provider{{ eligible.length === 1 ? "" : "s" }} passed:
                  {{ eligible.map((c) => c.service.id).join(", ") }}.
                </p>
                <p v-if="task.selected" class="chose">
                  Selected <strong>{{ task.selected.service.id }}</strong> at
                  {{ money(task.selected.quote?.amount ?? task.selected.service.amount) }} USDC.
                  {{ task.selected.reason }}
                </p>
              </template>
              <p v-else class="pending-line">Applying your authorisation…</p>
            </div>
          </li>
        </ol>

        <!-- the decision -->
        <div class="verdict" :class="verdict.tone" aria-live="polite">
          <span class="v-label">Decision</span>
          <strong class="v-word">{{ verdict.word }}</strong>
          <p class="v-why">{{ task.summary || "The backend is still working through the checks above." }}</p>
          <p v-if="task.error" class="v-error">{{ task.error }}</p>
          <p v-if="task.selected && task.request.mode !== 'pay'" class="v-rule">
            This run was a {{ task.request.mode === "simulate" ? "policy simulation" : "live preview" }}.
            No signature was created and no funds moved.
          </p>

          <div v-if="task.payment" class="settle">
            <p class="v-money">
              <span class="square-mark" aria-hidden="true" />
              {{ task.payment.settled ? "Settled on Base Sepolia"
                 : task.payment.signed ? "Signed. Settlement unconfirmed."
                 : "No signature. No funds moved." }}
              <a v-if="/^0x[0-9a-fA-F]{64}$/.test(task.payment.transaction ?? '')"
                :href="'https://sepolia.basescan.org/tx/' + task.payment.transaction"
                target="_blank" rel="noopener noreferrer">View transaction ↗</a>
            </p>
            <p v-if="task.payment.error" class="v-error">{{ task.payment.error }}</p>
            <div v-if="task.payment.data" class="delivered">
              <span class="d-head">What you received</span>
              <pre>{{ JSON.stringify(task.payment.data, null, 2) }}</pre>
            </div>
          </div>
        </div>
      </section>

      <section v-else class="story idle-story">
        <p class="idle-line">
          Nothing is authorised yet. The backend holds the signing key, so the agent
          cannot pay anyone by itself.
        </p>
        <p class="idle-sub">
          Set your authorisation below and send a task. Every step is shown, up to the
          moment money either moves or does not.
        </p>
      </section>

      <!-- the authorisation -->
      <section class="limits">
        <div class="limits-summary">
          <div class="limits-title"><span class="tiny-label">YOUR AUTHORISATION</span></div>
          <p>
            <strong>{{ policy.per_payment }}</strong> USDC / payment
            <span class="divider">·</span>
            <strong>{{ policy.task_budget }}</strong> USDC / task
            <span class="divider">·</span>
            level ≤ <strong>{{ policy.max_risk }}</strong>
          </p>
        </div>

        <fieldset class="limits-editor" :disabled="active || !!pending">
          <div class="pref-block">
            <span class="tiny-label">MODE <em>what this run is allowed to do</em></span>
            <div class="leanings">
              <label :class="mode === 'simulate' ? 'picked' : ''">
                <input type="radio" value="simulate" v-model="mode" @change="onModeChange" />
                <strong>Policy simulation</strong>
                <span>Real model, simulated quotes and risk. No Intercepta call and no signature.
                  The only mode where an advisory level 1 can be shown.</span>
              </label>
              <label :class="mode === 'preview' ? 'picked' : ''">
                <input type="radio" value="preview" v-model="mode" @change="onModeChange" />
                <strong>Live API preview</strong>
                <span>Real quotes and real Intercepta screening, with no signature and no
                  payment. Only level 0 is accepted.</span>
              </label>
              <label :class="mode === 'pay' ? 'picked' : ''">
                <input type="radio" value="pay" v-model="mode" @change="onModeChange" />
                <strong>Testnet execution</strong>
                <span>Everything above, and the backend signs and pays in Base Sepolia test
                  USDC. Only level 0 is accepted.</span>
              </label>
            </div>
          </div>

          <div class="limit-fields">
            <label>Per payment <span>USDC</span>
              <input v-model="policy.per_payment" inputmode="decimal" />
            </label>
            <label>Task budget <span>USDC</span>
              <input v-model="policy.task_budget" inputmode="decimal" />
            </label>
          </div>
          <p class="hint">The demo ceiling is 0.10 USDC, and one task can make at most one payment.</p>

          <div class="pref-block">
            <span class="tiny-label">RISK LEVEL ACCEPTED <em>recommended 0</em></span>
            <p class="pref-help">
              Level 0 means no signal was found in the scan. Level 1 is an advisory signal
              and is only offered in simulation, because the backend accepts nothing above 0
              once real money is involved. Level 2 is always excluded.
            </p>
            <div class="leanings levels">
              <label :class="policy.max_risk === 0 ? 'picked' : ''">
                <input type="radio" :value="0" v-model.number="policy.max_risk" />
                <strong>Level 0 — {{ levelLabel(0) }}</strong>
              </label>
              <label v-if="mode === 'simulate'" :class="policy.max_risk === 1 ? 'picked' : ''">
                <input type="radio" :value="1" v-model.number="policy.max_risk" />
                <strong>Level 1 — {{ levelLabel(1) }}</strong>
                <span>Simulation only.</span>
              </label>
            </div>
          </div>

          <div class="pref-block">
            <span class="tiny-label">WHEN PRICE AND RISK DISAGREE</span>
            <p class="pref-help">
              Two providers pass your rules, one cheaper and one cleaner. You decide this
              now so the agent does not have to guess later.
            </p>
            <div class="leanings">
              <label :class="policy.preference === 'price' ? 'picked' : ''">
                <input type="radio" value="price" v-model="policy.preference" />
                <strong>Price first</strong>
                <span>Take the lowest price among providers that passed. Ties broken by lower risk.</span>
              </label>
              <label :class="policy.preference === 'risk' ? 'picked' : ''">
                <input type="radio" value="risk" v-model="policy.preference" />
                <strong>Risk first</strong>
                <span>Take the lowest risk among providers that passed. Ties broken by lower price.</span>
              </label>
            </div>
          </div>

          <p class="fixed-rules">
            Level 2 is refused at any price. An unknown or failed scan is held, never treated
            as clean. Base Sepolia USDC only. The model cannot change this authorisation or
            hold the signing key.
          </p>
        </fieldset>

        <p v-if="mode === 'pay'" class="pay-note">
          Sending authorises the backend to spend Base Sepolia test USDC under these settings,
          from the local .buyer-key wallet.
        </p>
      </section>

      <!-- the backend's own record -->
      <section v-if="task?.events.length" class="log">
        <div class="limits-title">
          <span class="tiny-label">BACKEND RECORD</span>
          <span class="status-chip">{{ statusNames[task.status] ?? task.status }}</span>
        </div>
        <details v-for="(e, i) in task.events" :key="i">
          <summary>
            <span class="dot" />{{ eventNames[e.kind] ?? e.kind }}
            <time>{{ e.time.slice(11, 19) }}</time>
          </summary>
          <pre>{{ JSON.stringify(e.data, null, 2) }}</pre>
        </details>
      </section>

      <footer class="site-footer">
        <span>Decision402 · local prototype</span>
        <span>
          The model holds no private key. The authorisation cannot be changed by the model.
          Static Tokyo weather sample, not live weather.
        </span>
      </footer>
    </main>
  </div>
</template>
