<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import Dropdown from './Dropdown.vue'
import Scramble from './Scramble.vue'
type Policy={per_payment:string;task_budget:string;max_risk:number;preference:string}
type Request={id:string;agent_id:string;instruction:string;mode:string;policy:Policy}
type Agent={id:string;owner:string;name:string;wallet:string;model_url:string;model_name:string}
type EthereumProvider={request:(args:{method:string;params?:unknown[]})=>Promise<any>;on?:(event:string,handler:(accounts:string[])=>void)=>void;removeListener?:(event:string,handler:(accounts:string[])=>void)=>void}
declare global { interface Window { ethereum?: EthereumProvider } }
type Candidate={service:{id:string;name:string;pay_to:string;amount:string};level:number;eligible:boolean;reason:string;source:string;quote?:{amount:string};risk?:{toxicScore?:number;traits?:{name:string;description:string}[];duration_ms:number}}
type Task={request:Request;status:string;candidates:Candidate[];selected?:Candidate;events:{time:string;kind:string;data:unknown}[];summary:string;error?:string;payment_attempted:boolean;payment?:{settled:boolean;signed:boolean;transaction?:string;error?:string;data?:unknown}}
const instruction=ref('Get a sample Tokyo weather dataset. Choose a service using my budget and risk policy.')
const mode=ref('simulate')
const policy=ref<Policy>({per_payment:'0.10',task_budget:'0.10',max_risk:1,preference:'price'})
const task=ref<Task|null>(null), config=ref<{model:string;asset:string}|null>(null), error=ref(''), sending=ref(false)
const owner=ref(''), agents=ref<Agent[]>([]), agentID=ref(''), agentName=ref('Tokyo buyer'), modelURL=ref('https://api.deepseek.com'), modelName=ref('deepseek-flash'), modelAPIKey=ref('')
const agentBusy=ref(false), funding=ref(false), walletError=ref(''), balance=ref(''), fundAmount=ref('10'), fundTx=ref('')
const selectedAgent=computed(()=>agents.value.find(a=>a.id===agentID.value))
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
// A step that is merely 'not done' cannot be told apart from one that never
// started, so the sequence could never show anything working. The first step
// that is not done, while the task is still running, is the one in progress.
const flowSteps=computed(()=>{
 const steps=[
  {label:'Discover',done:!!task.value?.candidates.length,active:false},
  {label:'Screen',done:!!task.value?.candidates.some(c=>c.risk),active:false},
  {label:'Select',done:!!task.value?.selected,active:false},
  {label:'Settle',done:task.value?.request.mode==='pay'?!!task.value?.payment?.settled:!!task.value?.selected,active:false},
 ]
 if(active.value){const next=steps.find(s=>!s.done);if(next)next.active=true}
 return steps
})
const canStartFresh=computed(()=>!initializing.value&&!active.value&&!pending.value&&!(task.value?.payment_attempted&&!task.value?.payment?.settled))
const apiHeaders={'Content-Type':'application/json','X-Decision402':'local-ui'}
function provider():EthereumProvider { if(!window.ethereum)throw new Error('Install or enable MetaMask in this browser');return window.ethereum }
async function ensureBaseSepolia(){
 const wallet=provider(), chain=await wallet.request({method:'eth_chainId'})
 if(chain!=='0x14a34')await wallet.request({method:'wallet_switchEthereumChain',params:[{chainId:'0x14a34'}]})
 if(await wallet.request({method:'eth_chainId'})!=='0x14a34')throw new Error('Switch MetaMask to Base Sepolia first')
}
async function fetchAgents(){
 const r=await fetch('/api/agents');if(!r.ok)throw new Error('Wallet sign-in expired; connect again')
 agents.value=await r.json()
 if(!agentID.value)agentID.value=localStorage.getItem('decision402-agent')??''
 if(!agents.value.some(a=>a.id===agentID.value))agentID.value=agents.value[0]?.id??''
 if(agentID.value)await refreshBalance()
}
function chooseAgent(){localStorage.setItem('decision402-agent',agentID.value);balance.value='';fundTx.value='';void refreshBalance()}
async function connectWallet(){
 walletError.value='';agentBusy.value=true
 try{
  const wallet=provider(), accounts=await wallet.request({method:'eth_requestAccounts'}) as string[]
  if(!accounts?.[0])throw new Error('No wallet account selected')
  await ensureBaseSepolia()
  const address=accounts[0]
  const challengeResponse=await fetch('/api/auth/challenge?address='+encodeURIComponent(address))
  if(!challengeResponse.ok)throw new Error('Could not request wallet sign-in')
  const challenge=await challengeResponse.json()
  const signature=await wallet.request({method:'personal_sign',params:[challenge.message,address]}) as string
  const session=await fetch('/api/auth/session',{method:'POST',headers:apiHeaders,body:JSON.stringify({address,nonce:challenge.nonce,signature})})
  if(!session.ok)throw new Error('Wallet signature was not accepted')
  owner.value=address
  await fetchAgents()
 }catch(e){walletError.value=e instanceof Error?e.message:'Could not connect wallet'}finally{agentBusy.value=false}
}
async function createAgent(){
 walletError.value='';agentBusy.value=true
 try{
  if(!owner.value)throw new Error('Connect your owner wallet first')
  const response=await fetch('/api/agents',{method:'POST',headers:apiHeaders,body:JSON.stringify({name:agentName.value,model_url:modelURL.value,model_name:modelName.value,model_api_key:modelAPIKey.value.trim()})})
  const result=await response.json()
  if(!response.ok)throw new Error(result.error??'Could not create agent')
  modelAPIKey.value=''
  agents.value.push(result as Agent);agentID.value=result.id;localStorage.setItem('decision402-agent',result.id);balance.value='0';fundTx.value=''
 }catch(e){walletError.value=e instanceof Error?e.message:'Could not create agent'}finally{agentBusy.value=false}
}
async function refreshBalance(){
 const agent=selectedAgent.value
 if(!agent || !config.value)return
 try{
  await ensureBaseSepolia()
  const data='0x70a08231'+agent.wallet.slice(2).toLowerCase().padStart(64,'0')
  const value=await provider().request({method:'eth_call',params:[{to:config.value.asset,data},'latest']}) as string
  const atomic=BigInt(value)
  balance.value=(atomic/1000000n).toString()+'.'+(atomic%1000000n).toString().padStart(6,'0')
 }catch(e){walletError.value=e instanceof Error?e.message:'Could not read test USDC balance'}
}
function usdcUnits(value:string):bigint{
 if(!/^\d{1,3}(\.\d{1,6})?$/.test(value))throw new Error('Enter a USDC amount with up to six decimal places')
 const [whole,fraction='']=value.split('.')
 const amount=BigInt(whole)*1000000n+BigInt(fraction.padEnd(6,'0'))
 if(amount<=0n || amount>100000000n)throw new Error('Choose between 0 and 100 test USDC')
 return amount
}
async function fundAgent(){
 walletError.value='';funding.value=true
 try{
  const agent=selectedAgent.value
  if(!agent || !config.value || !owner.value)throw new Error('Connect and create an agent first')
  await ensureBaseSepolia()
  const accounts=await provider().request({method:'eth_accounts'}) as string[]
  if(!accounts?.some(a=>a.toLowerCase()===owner.value.toLowerCase()))throw new Error('MetaMask account changed; connect again')
  const amount=usdcUnits(fundAmount.value)
  const data='0xa9059cbb'+agent.wallet.slice(2).toLowerCase().padStart(64,'0')+amount.toString(16).padStart(64,'0')
  const tx=await provider().request({method:'eth_sendTransaction',params:[{from:owner.value,to:config.value.asset,data}]}) as string
  fundTx.value=tx
  for(let i=0;i<60;i++){
   const receipt=await provider().request({method:'eth_getTransactionReceipt',params:[tx]}) as {status:string}|null
   if(receipt){if(receipt.status!=='0x1')throw new Error('The funding transaction failed onchain');await refreshBalance();return}
   await new Promise(resolve=>setTimeout(resolve,1500))
  }
  throw new Error('Funding is pending. Check the transaction link, then refresh the balance.')
 }catch(e){walletError.value=e instanceof Error?e.message:'Funding failed'}finally{funding.value=false}
}
function walletChanged(accounts:string[]){
 if(owner.value && !accounts.some(a=>a.toLowerCase()===owner.value.toLowerCase())){owner.value='';agents.value=[];agentID.value='';balance.value='';task.value=null;walletError.value='Wallet account changed. Connect again.'}
}
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
  if(!selectedAgent.value)throw new Error('Connect your wallet and create an agent first')
  const request=pending.value??{id:crypto.randomUUID(),agent_id:selectedAgent.value.id,instruction:instruction.value,mode:mode.value,policy:{...policy.value,max_risk:mode.value==='simulate'?policy.value.max_risk:0}}
  pending.value=request;localStorage.setItem('decision402-pending',JSON.stringify(request));localStorage.setItem('decision402-last',request.id)
  const response=await fetch('/api/tasks',{method:'POST',headers:apiHeaders,body:JSON.stringify(request)})
  const result=await response.json()
  if(!response.ok){if(response.status<500){pending.value=null;localStorage.removeItem('decision402-pending')}throw new Error(result.error??'Could not create the task')}
  task.value=result
  history.replaceState(null,'','?task='+encodeURIComponent(request.id))
  if(['queued','running'].includes(result.status))watchTask(request.id)
  else{pending.value=null;localStorage.removeItem('decision402-pending')}
 }catch(e){error.value=e instanceof Error?e.message:'Connection failed. Retry will use the same task ID.'}finally{sending.value=false}
}
onMounted(async()=>{
 window.ethereum?.on?.('accountsChanged',walletChanged)
 try{const r=await fetch('/api/config');if(!r.ok)throw new Error('Backend is not running');config.value=await r.json()
  const session=await fetch('/api/auth/me')
  if(session.ok){const current=await session.json();const accounts=window.ethereum?await window.ethereum.request({method:'eth_accounts'}) as string[]:[];if(accounts?.some(a=>a.toLowerCase()===current.owner.toLowerCase())){owner.value=current.owner;await fetchAgents()}}
  const saved=localStorage.getItem('decision402-pending');if(saved)pending.value=JSON.parse(saved)
  const id=new URLSearchParams(location.search).get('task')??localStorage.getItem('decision402-last');if(id && owner.value){await getTask(id);if(task.value){policy.value={...task.value.request.policy,max_risk:task.value.request.mode==='simulate'?task.value.request.policy.max_risk:0};mode.value=task.value.request.mode;instruction.value=task.value.request.instruction;agentID.value=task.value.request.agent_id??agentID.value}if(['queued','running'].includes(task.value?.status??''))watchTask(id)}
 }catch(e){error.value=e instanceof Error?e.message:'Connection failed'}finally{initializing.value=false}
})
onUnmounted(()=>{clearInterval(timer);window.ethereum?.removeListener?.('accountsChanged',walletChanged)})
</script>

<template>
 <div class="shell">
  <header class="topbar">
   <a class="brand" href="/" aria-label="Decision402 home">
    <span class="brand-mark"><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 3 21 7v6c0 4-5 7-9 9-4-2-9-5-9-9V7l9-4Z" stroke="currentColor" stroke-width="1.6"/><path d="m8 12 3 3 5-6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg></span>
    <span>Decision<span class="brand-number">402</span><small>AGENT PAYMENT CONTROL</small></span>
   </a>
   <div class="header-right"><span class="connection"><i :class="{online:!!config}"></i>{{owner?shortAddress(owner):config?'Connect owner wallet':'Connecting'}}</span><span class="network"><i></i>Base Sepolia <b>TESTNET</b></span></div>
  </header>

  <section class="intro wide">
   <div class="intro-copy"><div class="eyebrow"><span></span>YOUR POLICY. AGENT ACTION.</div><h1><Scramble text="Autonomy." :tail="5" :delay="260" /><br><span><Scramble text="Within bounds." :tail="7" :delay="620" /></span></h1></div>
   </section>

  <section class="onboarding" aria-label="Set up your agent wallet">
   <div class="onboard-title"><span class="eyebrow">YOUR AGENT WORKSPACE</span><h2>Give an agent its own wallet.</h2><p>Connect an owner wallet, create an agent, and fund its Base Sepolia test USDC wallet. Your agent pays only after the policy and risk checks pass.</p></div>
   <div class="onboard-grid">
    <div class="onboard-col"><div class="onboard-card"><span class="onboard-step">01 / OWNER</span><h3>Connect MetaMask</h3><p>Sign a message to prove ownership. No transaction is sent at this step.</p><button class="onboard-button" :disabled="agentBusy" @click="connectWallet">{{owner?'Switch / reconnect':'Connect wallet'}} ↗</button><code v-if="owner">{{owner}}</code></div><div class="onboard-shape tri" aria-hidden="true"></div></div>
    <div class="onboard-col"><div class="onboard-card"><span class="onboard-step">02 / AGENT</span><h3>Create your buyer</h3><div class="onboard-fields"><label>Agent name<input v-model="agentName" maxlength="40" placeholder="Tokyo buyer" /></label><label>Model API URL<input v-model="modelURL" spellcheck="false" /></label><label>Model<Dropdown v-model="modelName" :options="[{value:'deepseek-flash',label:'DeepSeek Flash'},{value:'deepseek-v4-pro',label:'DeepSeek V4 Pro'}]" /></label><label>DeepSeek API key <small>optional if configured on server</small><input v-model="modelAPIKey" type="password" autocomplete="off" placeholder="Server default or your own key" /></label></div><button class="onboard-button" :disabled="!owner || agentBusy" @click="createAgent">{{agentBusy?'Working…':'Create agent + wallet'}} ↗</button></div></div>
    <div class="onboard-col"><div class="onboard-card"><span class="onboard-step">03 / FUND</span><h3>Fund the agent</h3><label v-if="agents.length">Choose agent<Dropdown v-model="agentID" @change="chooseAgent" :options="agents.map(a=>({value:a.id,label:a.name+' · '+shortAddress(a.wallet),short:a.name}))" /></label><div v-if="selectedAgent" class="agent-address"><small>AGENT WALLET · BASE SEPOLIA</small><code>{{selectedAgent.wallet}}</code><span>Balance: {{balance || '—'}} test USDC</span></div><p v-else>Create an agent to get its deposit address.</p><label>Amount · test USDC<input v-model="fundAmount" inputmode="decimal" /></label><div class="onboard-actions"><button class="onboard-button" :disabled="!selectedAgent || funding" @click="fundAgent">{{funding?'Waiting for confirmation…':'Fund with MetaMask'}} ↗</button><button class="ghost-button" :disabled="!selectedAgent" @click="refreshBalance">Refresh balance</button></div><a v-if="fundTx" :href="'https://sepolia.basescan.org/tx/'+fundTx" target="_blank" rel="noopener noreferrer">View funding transaction ↗</a></div><div class="onboard-shape circle" aria-hidden="true"></div></div>
   </div>
   <p v-if="walletError" class="error" role="alert">{{walletError}}</p>
   <p class="onboard-disclaimer">Local testnet prototype. The backend keeps the agent wallet key in a private local file; back up <code>artifacts/agents/</code> before moving or deleting this workspace. Never fund it with mainnet assets.</p>
  </section>
  <section class="ask">
   <form class="ask-field" @submit.prevent="start">
    <input v-model="instruction" maxlength="2000" :disabled="active || !!pending"
      placeholder="What should the agent buy?" aria-label="What should the agent buy" />
    <button type="submit" :disabled="initializing || active || !config || !selectedAgent || blockedPayment">
     <span>{{active?'Working':pending?'Retry':mode==='pay'?'Authorize payment':'Send'}}</span>
     <i :class="{spinner:active}" aria-hidden="true">{{active?'':'→'}}</i>
    </button>
   </form>
   <div class="ask-meta">
    <span v-if="!selectedAgent" class="ask-need">Create an agent below before sending a task.</span>
    <span v-else>Up to <b>{{policy.per_payment}}</b> USDC per payment · <b>{{policy.task_budget}}</b> total · risk &le; <b>{{policy.max_risk}}</b> · <b>{{policy.preference==='price'?'price first':'risk first'}}</b></span>
    <span class="ask-mode">{{mode==='simulate'?'POLICY SIMULATION':mode==='preview'?'LIVE PREVIEW · NO PAYMENT':'TESTNET EXECUTION'}}</span>
   </div>
  </section>
  <div class="workspace-heading"><div><span class="eyebrow">PAYMENT WORKSPACE</span><p>From an instruction to an accountable decision.</p></div><div class="workspace-actions"><span class="workspace-note">Mainnet risk data <span>↔</span> Testnet settlement</span><button class="new-task" :disabled="!canStartFresh" @click="newTask"><span aria-hidden="true">+</span> New task</button></div></div>
  <main>
   <section class="panel controls">
    <div class="section-title"><span class="step">01</span><div><h2>Define the boundaries</h2><p>Your authorization comes first.</p></div></div>
    <fieldset :disabled="active || !!pending">
     <div class="mode-picker">
      <span class="tiny-label">EXECUTION MODE</span>
      <div class="modes">
       <label v-for="m in [
         {id:'simulate',name:'Policy simulation',note:'Simulated offers and risk',spend:'No payment'},
         {id:'preview',name:'Live API preview',note:'Real quotes, real screening',spend:'No payment'},
         {id:'pay',name:'Testnet execution',note:'Signs and settles on Base Sepolia',spend:'Spends test USDC'}
       ]" :key="m.id" :class="[mode===m.id?'picked':'', m.id==='pay'?'is-pay':'']">
        <input type="radio" name="mode" :value="m.id" v-model="mode" @change="onModeChange" />
        <b>{{m.name}}</b>
        <small>{{m.note}}</small>
        <em>{{m.spend}}</em>
       </label>
      </div>
     </div>
     <div class="control-divider"><span>SPENDING LIMITS</span><span>USDC</span></div>
     <div class="row"><label>Per-payment cap<input v-model="policy.per_payment" inputmode="decimal" /></label><label>Total task budget<input v-model="policy.task_budget" inputmode="decimal" /></label></div>
     <div class="row"><label>Allowed risk<Dropdown v-model="policy.max_risk" :options="mode==='simulate'?[{value:0,label:'Level 0 only'},{value:1,label:'Levels 0 + 1'}]:[{value:0,label:'Level 0 only'}]" /></label><label>Selection priority<Dropdown v-model="policy.preference" :options="[{value:'price',label:'Lowest price first'},{value:'risk',label:'Lowest risk first'}]" /></label></div>
     <p class="hint">High risk is always blocked. Level 0 means no detected signal, not guaranteed safety. Demo cap: 0.10 USDC.</p>
     <div class="dataset-label"><span class="sample-icon">◈</span><div>Tokyo weather sample<small>Static demo data · one purchase per task</small></div></div>
    </fieldset>
    <div v-if="mode==='pay'" class="pay-note">This action authorizes spending Base Sepolia test USDC under the policy above.</div>
    <p v-if="!selectedAgent" class="hint">Connect an owner wallet and create an agent to run a task.</p>
    <p v-if="blockedPayment" class="error">A previous payment has unconfirmed settlement. Reconcile it before another payment.</p>
    <p v-if="error" class="error" role="alert">{{error}}</p>
    <div class="model"><span><i></i>{{selectedAgent?.model_name??config?.model??'Connecting…'}}</span><small>Model calls use API credits</small></div>
    <details class="scope-details"><summary>About this demo <span>+</span></summary><p>Four local endpoints serve the same static sample. In live modes, A/B share a known-risk fixture; C/D share a recipient with no detected risk signal. Level 1 is simulation-only. Unknown risk puts payment on hold.</p></details>
   </section>

   <section class="results">
    <div class="panel decision-panel">
     <div class="section-title"><span class="step">02</span><div><h2>The moment of decision</h2><p>{{task?(active?'Execution in progress':'Recorded task result'):'Watch your policy become action.'}}</p></div><span class="status" :class="task?.status" role="status">{{task?statusNames[task.status]:'Ready when you are'}}</span></div>
     <div class="flow-strip"><div v-for="(item,index) in flowSteps" :key="item.label" :class="{done:item.done,working:item.active}"><span class="flow-orb">{{item.done?'✓':String(index+1).padStart(2,'0')}}</span><b>{{item.label}}</b></div></div>
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
