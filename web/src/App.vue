<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
type Policy={per_payment:string;task_budget:string;max_risk:number;preference:string}
type Request={id:string;instruction:string;mode:string;policy:Policy}
type Candidate={service:{id:string;name:string;pay_to:string;amount:string};level:number;eligible:boolean;reason:string;source:string;quote?:{amount:string};risk?:{toxicScore?:number;traits?:{name:string;description:string}[];duration_ms:number}}
type Task={request:Request;status:string;candidates:Candidate[];selected?:Candidate;events:{time:string;kind:string;data:unknown}[];summary:string;error?:string;payment_attempted:boolean;payment?:{settled:boolean;signed:boolean;transaction?:string;error?:string;data?:unknown}}
const instruction=ref('Get a sample Tokyo weather dataset. Choose a service using my budget and risk policy.')
const mode=ref('simulate')
const policy=ref<Policy>({per_payment:'0.10',task_budget:'0.10',max_risk:1,preference:'price'})
const task=ref<Task|null>(null), config=ref<{model:string}|null>(null), error=ref(''), sending=ref(false)
const pending=ref<Request|null>(null)
const initializing=ref(true)
let timer:ReturnType<typeof setInterval>|undefined
const active=computed(()=>sending.value || ['queued','running'].includes(task.value?.status??''))
const statusNames:Record<string,string>={queued:'Queued',running:'Agent running',previewed:'Preview complete · No payment',settled:'Settled on testnet',held:'On hold',unknown:'Settlement unconfirmed',error:'Failed'}
const eventNames:Record<string,string>={model:'Agent reasoning and tool choice',tool:'Tool call',candidate:'Offer and risk screening',selection:'Policy selection',final_risk:'Final pre-signing check',authorization_reserved:'Payment authorization reserved'}
const money=(value:string)=> (Number(value)/1e6).toFixed(3)
const blockedPayment=computed(()=>mode.value==='pay' && !!task.value?.payment_attempted && !task.value?.payment?.settled)
const containsHan=(value:string)=>/[\u3400-\u9fff]/u.test(value)
function displayReason(c:Candidate):string{
 if(!containsHan(c.reason))return c.reason
 if(c.eligible)return 'Within the authorized budget and risk level'
 if(c.level>=2)return 'Blocked by policy'
 if(c.level<0)return 'Risk information unavailable; payment on hold'
 return 'Outside the authorized risk level or budget'
}
const displaySummary=computed(()=>{
 const t=task.value
 if(!t)return ''
 if(!containsHan(t.summary))return t.summary
 if(!t.candidates.length)return 'This archived task has no English decision summary. Run a new task to generate one.'
 const categories=[
  [t.candidates.filter(c=>!c.eligible&&c.level>=2),'blocked for risk'],
  [t.candidates.filter(c=>!c.eligible&&c.level<0),'held for missing risk data'],
  [t.candidates.filter(c=>!c.eligible&&c.level>=0&&c.level<2),'outside policy or budget'],
  [t.candidates.filter(c=>c.eligible),'eligible'],
 ] as const
 const screening=categories.filter(([items])=>items.length).map(([items,label])=>items.map(c=>c.service.id).join('/')+' '+label).join('; ')
 const chosen=t.selected
 const selection=chosen?.quote
  ? (t.request.policy.preference==='risk'?'risk first; price breaks ties':'price first')+'; chose '+chosen.service.id+' ('+money(chosen.quote.amount).replace(/0+$/,'').replace(/\.$/,'')+' USDC).'
  : 'no eligible service.'
 const result=t.status==='settled'?'Base Sepolia testnet payment settled.'
  :t.status==='unknown'?'Payment authorization was signed, but settlement is unconfirmed.'
  :t.request.mode==='simulate'?'Policy simulation; no signature or payment.'
  :t.request.mode==='preview'?'Live API preview; no signature or payment.'
  :'No payment was made.'
 return 'Screening: '+screening+'. Selection: '+selection+' '+result
})
const eligibleCount=computed(()=>task.value?.candidates.filter(c=>c.eligible).length??0)
const excludedCount=computed(()=>task.value?.candidates.filter(c=>!c.eligible).length??0)
const taskModeLabel=computed(()=>task.value?.request.mode==='simulate'?'SIMULATED INPUTS':task.value?.request.mode==='preview'?'LIVE API · NO PAYMENT':'TESTNET EXECUTION')
const shortAddress=(value:string)=>value.slice(0,6)+'…'+value.slice(-4)
const traitLabel=(value:string)=>value.replace(/_/g,' ')
const flowSteps=computed(()=>[
 {label:'Discover',done:!!task.value?.candidates.length},
 {label:'Screen',done:!!task.value?.candidates.length},
 {label:'Select',done:!!task.value?.selected},
 {label:'Settle',done:!!task.value?.payment?.settled},
])
const canStartFresh=computed(()=>!initializing.value&&!active.value&&!pending.value&&!(task.value?.payment_attempted&&!task.value?.payment?.settled))
function newTask(){
 if(!canStartFresh.value)return
 task.value=null
 error.value=''
 mode.value='preview'
 policy.value={per_payment:'0.10',task_budget:'0.10',max_risk:0,preference:'price'}
 instruction.value='Get a sample Tokyo weather dataset. Choose a service using my budget and risk policy.'
 localStorage.removeItem('decision402-last')
 history.replaceState(null,'',location.pathname)
}
function onModeChange(){if(mode.value!=='simulate')policy.value.max_risk=0}
async function getTask(id:string){
 const response=await fetch('/api/tasks/'+encodeURIComponent(id))
 if(!response.ok) throw new Error(response.status===404?'Task not found. You can retry the original request.':'Could not load the task')
 task.value=await response.json()
 if(!['queued','running'].includes(task.value!.status)){clearInterval(timer);pending.value=null;localStorage.removeItem('decision402-pending')}
}
function watchTask(id:string){clearInterval(timer);timer=setInterval(()=>getTask(id).catch(e=>{error.value=e.message}),1500)}
async function start(){
 error.value='';sending.value=true
 try{
  // Retry an uncertain POST with the SAME immutable request ID and payload.
  const request=pending.value??{id:crypto.randomUUID(),instruction:instruction.value,mode:mode.value,policy:{...policy.value,max_risk:mode.value==='simulate'?policy.value.max_risk:0}}
  pending.value=request;localStorage.setItem('decision402-pending',JSON.stringify(request));localStorage.setItem('decision402-last',request.id)
  const response=await fetch('/api/tasks',{method:'POST',headers:{'Content-Type':'application/json','X-Decision402':'local-ui'},body:JSON.stringify(request)})
  const result=await response.json()
  if(!response.ok){if(response.status<500){pending.value=null;localStorage.removeItem('decision402-pending')}throw new Error(result.error??'Could not create the task')}
  task.value=result
  history.replaceState(null,'','?task='+encodeURIComponent(request.id))
  if(['queued','running'].includes(result.status))watchTask(request.id)
  else{pending.value=null;localStorage.removeItem('decision402-pending')}
 }catch(e){error.value=e instanceof Error?e.message:'Connection failed. Retry will use the same task ID.'}finally{sending.value=false}
}
onMounted(async()=>{
 try{const r=await fetch('/api/config');if(!r.ok)throw new Error('Backend is not running');config.value=await r.json()
  const saved=localStorage.getItem('decision402-pending');if(saved)pending.value=JSON.parse(saved)
  const id=new URLSearchParams(location.search).get('task')??localStorage.getItem('decision402-last');if(id){await getTask(id);if(task.value){policy.value={...task.value.request.policy,max_risk:task.value.request.mode==='simulate'?task.value.request.policy.max_risk:0};mode.value=task.value.request.mode;instruction.value=task.value.request.instruction}if(['queued','running'].includes(task.value?.status??''))watchTask(id)}
 }catch(e){error.value=e instanceof Error?e.message:'Connection failed'}finally{initializing.value=false}
})
onUnmounted(()=>clearInterval(timer))
</script>

<template>
 <div class="shell">
  <header class="topbar">
   <a class="brand" href="/" aria-label="Decision402 home">
    <span class="brand-mark"><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 3 21 7v6c0 4-5 7-9 9-4-2-9-5-9-9V7l9-4Z" stroke="currentColor" stroke-width="1.6"/><path d="m8 12 3 3 5-6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg></span>
    <span>Decision<span class="brand-number">402</span><small>AGENT PAYMENT CONTROL</small></span>
   </a>
   <div class="header-right"><span class="connection"><i :class="{online:!!config}"></i>{{config?'Workspace connected':'Connecting'}}</span><span class="network"><i></i>Base Sepolia <b>TESTNET</b></span></div>
  </header>

  <section class="intro">
   <div class="intro-copy"><div class="eyebrow"><span></span>YOUR POLICY. AGENT ACTION.</div><h1>Autonomy.<br><span>Within bounds.</span></h1><p>Let your agent find the right service.<br>Keep every payment inside your rules.</p></div>
   <div class="architecture" aria-label="Task intent passes through the owner's policy before payment authorization">
    <div class="architecture-top"><span>THE AUTHORIZATION BOUNDARY</span><span class="tiny-cross">+</span></div>
    <div class="architecture-flow">
     <div class="arch-node"><span class="node-icon">↗</span><b>Agent intent</b><small>Understand the task</small></div>
     <span class="arch-line"></span>
     <div class="arch-node policy-node"><span class="node-icon"><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5 5h14v14H5zM9 1v8M15 1v8M9 15v8M15 15v8M1 9h8M15 9h8M1 15h8M15 15h8" stroke="currentColor" stroke-width="1.5"/></svg></span><b>Your policy</b><small>Screen · filter · rank</small></div>
     <span class="arch-line"></span>
     <div class="arch-node"><span class="node-icon">✓</span><b>Payment</b><small>Recheck before signing</small></div>
    </div>
    <div class="architecture-bottom"><span><i></i>Owner-authorized execution</span><span>x402</span></div>
   </div>
  </section>

  <div class="workspace-heading"><div><span class="eyebrow">PAYMENT WORKSPACE</span><p>From an instruction to an accountable decision.</p></div><div class="workspace-actions"><span class="workspace-note">Mainnet risk data <span>↔</span> Testnet settlement</span><button class="new-task" :disabled="!canStartFresh" @click="newTask"><span aria-hidden="true">+</span> New task</button></div></div>
  <main>
   <section class="panel controls">
    <div class="section-title"><span class="step">01</span><div><h2>Define the boundaries</h2><p>Your authorization comes first.</p></div></div>
    <fieldset :disabled="active || !!pending">
     <label>Execution mode<select v-model="mode" @change="onModeChange"><option value="simulate">Policy simulation · No payment</option><option value="preview">Live API preview · No payment</option><option value="pay">Live testnet execution · Pays automatically</option></select></label>
     <div class="mode-note" :class="{simulation:mode==='simulate'}"><i></i><span v-if="mode==='simulate'">Simulated offers and risk. Real agent reasoning. No signature or payment.</span><span v-else-if="mode==='preview'">Real offers and Intercepta screening. Preview the decision without paying.</span><span v-else>Real screening and a Base Sepolia payment, within the policy below.</span></div>
     <div class="control-divider"><span>SPENDING LIMITS</span><span>USDC</span></div>
     <div class="row"><label>Per-payment cap<input v-model="policy.per_payment" inputmode="decimal" /></label><label>Total task budget<input v-model="policy.task_budget" inputmode="decimal" /></label></div>
     <div class="row"><label>Allowed risk<select v-model.number="policy.max_risk"><option :value="0">Level 0 only</option><option v-if="mode==='simulate'" :value="1">Levels 0 + 1</option></select></label><label>Selection priority<select v-model="policy.preference"><option value="price">Lowest price first</option><option value="risk">Lowest risk first</option></select></label></div>
     <p class="hint">High risk is always blocked. Level 0 means no detected signal, not guaranteed safety. Demo cap: 0.10 USDC.</p>
     <div class="control-divider"><span>AGENT INSTRUCTION</span><span>↗</span></div>
     <label class="task-label">What should the agent do?<textarea v-model="instruction" rows="4" maxlength="2000"></textarea></label>
     <div class="dataset-label"><span class="sample-icon">◈</span><div>Tokyo weather sample<small>Static demo data · one purchase per task</small></div></div>
    </fieldset>
    <div v-if="mode==='pay'" class="pay-note">This action authorizes spending Base Sepolia test USDC under the policy above.</div>
    <button class="primary" :disabled="initializing || active || !config || blockedPayment" @click="start"><span>{{active?'Agent working…':pending?'Retry same task':mode==='pay'?'Authorize testnet payment':'Run agent'}}</span><span :class="{spinner:active}" aria-hidden="true">{{active?'':'↗'}}</span></button>
    <p v-if="blockedPayment" class="error">A previous payment has unconfirmed settlement. Reconcile it before another payment.</p>
    <p v-if="error" class="error" role="alert">{{error}}</p>
    <div class="model"><span><i></i>{{config?.model??'Connecting…'}}</span><small>Model calls use API credits</small></div>
    <details class="scope-details"><summary>About this demo <span>+</span></summary><p>Four local endpoints serve the same static sample. In live modes, A/B share a known-risk fixture; C/D share a recipient with no detected risk signal. Level 1 is simulation-only. Unknown risk puts payment on hold.</p></details>
   </section>

   <section class="results">
    <div class="panel decision-panel">
     <div class="section-title"><span class="step">02</span><div><h2>The moment of decision</h2><p>{{task?(active?'Execution in progress':'Recorded task result'):'Watch your policy become action.'}}</p></div><span class="status" :class="task?.status" role="status">{{task?statusNames[task.status]:'Ready when you are'}}</span></div>
     <div class="flow-strip"><div v-for="(item,index) in flowSteps" :key="item.label" :class="{done:item.done}"><span>{{item.done?'✓':String(index+1).padStart(2,'0')}}</span>{{item.label}}</div></div>
     <template v-if="task">
      <div class="run-policy"><span class="run-mode">{{taskModeLabel}}</span><span>≤ {{task.request.policy.per_payment}} / payment</span><span>≤ {{task.request.policy.task_budget}} total</span><span>Risk ≤ {{task.request.policy.max_risk}}</span><span>{{task.request.policy.preference==='price'?'Price first':'Risk first'}}</span></div>
      <div v-if="task.candidates.length" class="decision-metrics"><div><strong>{{task.candidates.length}}</strong><span>offers evaluated</span></div><div><strong class="metric-blocked">{{excludedCount}}</strong><span>excluded by policy</span></div><div><strong class="metric-eligible">{{eligibleCount}}</strong><span>eligible options</span></div></div>
      <div v-if="task.candidates.length" class="candidate-list">
       <div class="candidate-head"><span>SERVICE / RECIPIENT</span><span>PRICE · USDC</span><span>RISK CHECK</span><span>DECISION</span></div>
       <article v-for="c in task.candidates" :key="c.service.id" class="candidate" :class="{chosen:task.selected?.service.id===c.service.id,rejected:!c.eligible}">
        <div class="candidate-row">
         <div class="provider"><span class="provider-avatar">{{c.service.id}}</span><div><b>Provider {{c.service.id}}</b><small>{{shortAddress(c.service.pay_to)}}</small></div></div>
         <div class="quote-price">{{money(c.quote?.amount??c.service.amount)}}</div>
         <div class="risk-cell"><span class="risk-badge" :class="c.level<0?'unknown':c.level>=2?'high':c.level===1?'advisory':'clear'"><i></i>{{c.level<0?'Unknown':c.level>=2?'Blocked':c.level===1?'Advisory':'No signal'}}</span><small>{{c.risk?.toxicScore!==undefined?'Score '+c.risk.toxicScore+'/100':'Level '+c.level+(task.request.mode==='simulate'?' · simulated':'')}}</small></div>
         <div class="outcome"><span :class="c.eligible?'allow':'deny'">{{task.selected?.service.id===c.service.id?'✓ Selected':c.eligible?'Eligible':'× Excluded'}}</span></div>
        </div>
        <div class="candidate-reason"><span class="reason-mark">{{c.eligible?'↳':'⊘'}}</span><span>{{displayReason(c)}}</span></div>
        <details v-if="c.risk?.traits?.length" class="risk-details"><summary><span v-for="trait in c.risk.traits" :key="trait.name" class="trait-tag">{{traitLabel(trait.name)}}</span><span class="detail-link">View risk evidence ↗</span></summary><p v-for="trait in c.risk.traits" :key="trait.name">{{trait.description}}</p></details>
       </article>
      </div>
      <div v-else class="empty loading-state"><span class="orbit" aria-hidden="true"></span><h3>Following your instruction.</h3><p>Waiting for the agent and service screening.</p></div>
      <div v-if="task.summary" class="answer" :class="{hasSelection:!!task.selected}"><div class="answer-top"><span>DECISION EXPLAINED</span><span>FILTER → RANK → EXECUTE</span></div><div v-if="task.selected" class="selection-headline"><span>Provider {{task.selected.service.id}}</span><b>{{money(task.selected.quote?.amount??task.selected.service.amount)}} <small>USDC</small></b></div><p>{{displaySummary}}</p></div>
      <p v-if="task.error" class="error" role="alert">{{task.error}}</p>
      <div v-if="task.payment" class="receipt" :class="{unconfirmed:!task.payment.settled}">
       <div class="receipt-top"><span class="receipt-check">{{task.payment.settled?'✓':'!'}}</span><div><b>{{task.payment.settled?'Settlement confirmed':'Settlement unconfirmed'}}</b><small>Base Sepolia · test USDC</small></div><a v-if="/^0x[0-9a-fA-F]{64}$/.test(task.payment.transaction??'')" :href="'https://sepolia.basescan.org/tx/'+task.payment.transaction" target="_blank" rel="noopener noreferrer">View transaction ↗</a></div>
       <p v-if="task.payment.error" class="error">{{task.payment.error}}</p>
       <details v-if="task.payment.data" class="response-details"><summary>Returned sample data <span>+</span></summary><pre>{{JSON.stringify(task.payment.data,null,2)}}</pre></details>
      </div>
     </template>
     <div v-else class="empty welcome-state"><div class="empty-art" aria-hidden="true"><span class="empty-ring"></span><span class="empty-core">↗</span><i class="satellite one">✓</i><i class="satellite two">×</i></div><div class="eyebrow">AUTONOMY WITH PERMISSION</div><h3>A clear reason for every payment.</h3><p>Set your limits and give the agent a task.<br>See which services qualify, which are blocked,<br>and why your policy chooses the next step.</p><div class="empty-legend"><span><i></i>Risk screening</span><span><i></i>Budget enforcement</span><span><i></i>Pre-signing check</span></div></div>
    </div>
    <div class="panel timeline">
     <div class="section-title"><span class="step">03</span><div><h2>Execution trail</h2><p>The evidence behind the decision.</p></div><span class="event-count">{{task?.events.length??0}} EVENTS</span></div>
     <div v-if="!task?.events.length" class="trail-empty"><span>↳</span>Agent calls, risk checks, and payment records will appear here.</div>
     <details v-for="(e,i) in task?.events??[]" :key="i" class="event"><summary><span class="event-index">{{String(i+1).padStart(2,'0')}}</span><span class="event-name">{{eventNames[e.kind]??e.kind}}</span><time>{{e.time.slice(11,19)}}</time><span class="event-expand">+</span></summary><pre>{{JSON.stringify(e.data,null,2)}}</pre></details>
    </div>
   </section>
  </main>
  <footer><a href="/" class="footer-brand">Decision402<span>Designed for delegated decisions.</span></a><span>Local prototype · Model has no private key · Static sample data</span></footer>
 </div>
</template>
