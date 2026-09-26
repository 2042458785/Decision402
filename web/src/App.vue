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
let timer:ReturnType<typeof setInterval>|undefined
const active=computed(()=>sending.value || ['queued','running'].includes(task.value?.status??''))
const statusNames:Record<string,string>={queued:'Queued',running:'Agent running',previewed:'Preview complete · No payment',settled:'Settled on testnet',held:'On hold',unknown:'Settlement unconfirmed',error:'Failed'}
const eventNames:Record<string,string>={model:'Agent reasoning and tool choice',tool:'Tool call',candidate:'Offer and risk screening',selection:'Policy selection',final_risk:'Final pre-signing check',authorization_reserved:'Payment authorization reserved'}
const money=(value:string)=> (Number(value)/1e6).toFixed(3)
const levels=['0 · No risk signal detected','1 · Approved advisory signal','2 · Blocked by policy']
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
 }catch(e){error.value=e instanceof Error?e.message:'Connection failed'}
})
onUnmounted(()=>clearInterval(timer))
</script>

<template>
 <div class="shell">
  <header><a class="brand" href="/">D<span>402</span><i>Decision402</i></a><span class="network"><b></b> BASE SEPOLIA · TESTNET</span></header>
  <section class="intro"><div class="eyebrow">YOUR POLICY. AGENT ACTION.</div><h1>Let agents act.<br><span>Keep payments within your policy.</span></h1><p>Set a budget and risk policy. The agent understands your request; the policy engine screens services and checks again before signing.</p></section>
  <main>
   <section class="panel controls">
    <div class="section-title"><span class="step">01</span><h2>Set your policy and task</h2></div>
    <fieldset :disabled="active || !!pending"><label>Execution mode<select v-model="mode" @change="onModeChange"><option value="simulate">Policy simulation · No payment</option><option value="preview">Live API preview · No payment</option><option value="pay">Live testnet execution · Pays automatically</option></select></label>
    <p class="notice" v-if="mode==='simulate'">DeepSeek interprets the task. Offers and risk levels are simulated to compare C (level 1) with D (level 0). No Intercepta call or signature.</p>
    <p class="notice" v-else>Live DeepSeek, Intercepta, and x402 offers. A/B use a known-risk fixture and are blocked. C/D share a recipient with no detected risk signal; the policy chooses cheaper C. Level 1 is simulation-only. Unknown risk puts payment on hold.</p>
    <div class="row"><label>Per-payment cap · USDC<input v-model="policy.per_payment" inputmode="decimal" /></label><label>Task budget · USDC<input v-model="policy.task_budget" inputmode="decimal" /></label></div>
    <div class="row"><label>Allowed risk<select v-model.number="policy.max_risk"><option :value="0">Level 0 only</option><option v-if="mode==='simulate'" :value="1">Levels 0 + 1 (simulation only)</option></select></label><label>Selection priority<select v-model="policy.preference"><option value="price">Lowest price first</option><option value="risk">Lowest risk first</option></select></label></div>
    <p class="hint">Live modes accept only level 0; high risk is always blocked. Level 0 means no detected signal, not guaranteed safety. One payment per task; demo cap: 0.10 USDC.</p>
    <label>What should the agent do?<textarea v-model="instruction" rows="4" maxlength="2000"></textarea></label>
    <p class="hint">Only a static Tokyo weather sample is available, not live weather. Model calls consume DeepSeek credits.</p>
    </fieldset>
    <div v-if="mode==='pay'" class="pay-note">Clicking authorizes the backend to spend Base Sepolia test USDC under this task's policy. The wallet key stays in the local .buyer-key file.</div>
    <button class="primary" :disabled="active || !config || blockedPayment" @click="start">{{active?'Running…':pending?'Retry same task (no new payment)':mode==='pay'?'Authorize testnet payment':'Run agent'}} <span>→</span></button>
    <p v-if="blockedPayment" class="error">A previous payment is reserved but settlement is unconfirmed. Reconcile it before another payment.</p>
    <p v-if="error" class="error" role="alert">{{error}}</p>
    <div class="model">MODEL <span>{{config?.model??'Connecting to backend…'}}</span></div>
   </section>
   <section class="results">
    <div class="panel">
     <div class="section-title"><span class="step">02</span><h2>Moment of Decision</h2><span class="status" :class="task?.status">{{task?statusNames[task.status]:'Waiting for a task'}}</span></div>
     <template v-if="task">
      <div class="run-policy">Policy: per payment {{task.request.policy.per_payment}} / task budget {{task.request.policy.task_budget}} USDC · risk ≤ {{task.request.policy.max_risk}} · {{task.request.policy.preference==='price'?'Lowest price first':'Lowest risk first'}}<strong>{{task.request.mode==='simulate'?'Simulated offers and risk':task.request.mode==='preview'?'Live API · read only':'Live testnet payment'}}</strong></div>
      <div class="table-wrap"><table><thead><tr><th>Service</th><th>USDC</th><th>Risk level</th><th>Policy outcome</th></tr></thead><tbody><tr v-for="c in task.candidates" :key="c.service.id" :class="{selected:task.selected?.service.id===c.service.id}"><td><b>{{c.service.id}}</b></td><td>{{money(c.quote?.amount??c.service.amount)}}</td><td>{{c.level<0?'Unknown':levels[c.level]}}<small v-if="c.risk?.toxicScore!==undefined">Toxic Score {{c.risk.toxicScore}}/100</small></td><td><span :class="c.eligible?'allow':'deny'">{{task.selected?.service.id===c.service.id?'Selected':c.eligible?'Eligible':'Excluded'}}</span><small>{{displayReason(c)}}</small><small v-if="c.risk?.traits?.length">Risk reasons: {{c.risk.traits.map(t=>t.description).join('; ')}}</small></td></tr></tbody></table></div>
      <p v-if="!task.candidates.length" class="empty">Waiting for the agent and risk screening…</p>
      <div v-if="task.summary" class="answer"><label>{{task.candidates.length?'Backend decision · Filter → Rank → Execute':'Agent response'}}</label><p>{{displaySummary}}</p></div>
      <p v-if="task.error" class="error">{{task.error}}</p>
      <div v-if="task.payment" class="receipt"><b>{{task.payment.settled?'Settlement confirmed':'Settlement unconfirmed'}}</b><p v-if="task.payment.error">{{task.payment.error}}</p><a v-if="/^0x[0-9a-fA-F]{64}$/.test(task.payment.transaction??'')" :href="'https://sepolia.basescan.org/tx/'+task.payment.transaction" target="_blank" rel="noopener noreferrer">View Base Sepolia transaction ↗</a><pre v-if="task.payment.data">{{JSON.stringify(task.payment.data,null,2)}}</pre></div>
     </template>
     <div v-else class="empty"><div class="diagram"><span>Task</span> → <span>Policy</span> → <span>Screen</span> → <span>Execute</span></div><p>Set a policy to begin. Compare the simulation first,<br>then preview the live API and execute on testnet.</p></div>
    </div>
    <div class="panel timeline"><div class="section-title"><span class="step">03</span><h2>Execution log</h2><small>Expand to inspect raw fields</small></div><div v-if="!task?.events.length" class="empty">Offers, risk results, and payment decisions appear here.</div><details v-for="(e,i) in task?.events??[]" :key="i"><summary><span class="dot"></span>{{eventNames[e.kind]??e.kind}}<time>{{e.time.slice(11,19)}}</time></summary><pre>{{JSON.stringify(e.data,null,2)}}</pre></details></div>
   </section>
  </main>
  <footer>Decision402 · Local hackathon prototype <span>The model has no private key · Policy cannot be changed by the model · Static weather sample</span></footer>
 </div>
</template>
