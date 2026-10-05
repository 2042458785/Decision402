<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import Dropdown from './Dropdown.vue'
import Scramble from './Scramble.vue'
type Policy={per_payment:string;task_budget:string;max_risk:number;preference:string}
type Request={id:string;agent_id:string;instruction:string;mode:string;policy:Policy}
type Agent={wallet_kind:string;session_address?:string;id:string;owner:string;name:string;wallet:string;model_url:string;model_name:string;locked:boolean;unlock_until?:string;has_model_key:boolean}
type EthereumProvider={request:(args:{method:string;params?:unknown[]})=>Promise<any>;on?:(event:string,handler:(accounts:string[])=>void)=>void;removeListener?:(event:string,handler:(accounts:string[])=>void)=>void}
declare global { interface Window { ethereum?: EthereumProvider } }
type Candidate={service:{id:string;name:string;pay_to:string;amount:string};level:number;eligible:boolean;reason:string;source:string;quote?:{amount:string};risk?:{toxicScore?:number;traits?:{name:string;description:string}[];duration_ms:number}}
type Task={request:Request;status:string;candidates:Candidate[];selected?:Candidate;events:{time:string;kind:string;data:unknown}[];summary:string;error?:string;payment_attempted:boolean;payment?:{settled:boolean;signed:boolean;transaction?:string;error?:string;data?:unknown}}
const instruction=ref('Get a sample Tokyo weather dataset. Choose a service using my budget and risk policy.')
const mode=ref('simulate')
const policy=ref<Policy>({per_payment:'0.10',task_budget:'0.10',max_risk:1,preference:'price'})
const task=ref<Task|null>(null), config=ref<{model:string;asset:string;smart_wallet_available:boolean}|null>(null), error=ref(''), sending=ref(false)
const owner=ref(''), agents=ref<Agent[]>([]), agentID=ref(''), agentName=ref('Tokyo buyer'), modelURL=ref('https://api.deepseek.com'), modelName=ref('deepseek-flash'), modelAPIKey=ref('')
const walletPassword=ref(''), passwordConfirm=ref(''), actionPassword=ref(''), backupFile=ref<File|null>(null), walletNotice=ref(''), clockNow=ref(Date.now())
let walletClock:ReturnType<typeof setInterval>|undefined
const walletLocked=computed(()=>!selectedAgent.value || selectedAgent.value.locked || !selectedAgent.value.unlock_until || Date.parse(selectedAgent.value.unlock_until)<=clockNow.value)
type SmartStatus={sessionKey:string;perPayment:string;dailyLimit:string;expiresAt:string;getRecipients:string[];reserved_today:string;gas_wei:string;chain_time:number}
const smartStatus=ref<SmartStatus|null>(null), smartBusy=ref(false), smartTx=ref(''), linkAddress=ref(''), restoreKind=ref('legacy')
const allowRecipients=ref(''), grantPer=ref('0.10'), grantDaily=ref('0.20'), grantHours=ref('24'), withdrawAmount=ref(''), gasAmount=ref('0.0002')
const smartAuthorized=computed(()=>!!smartStatus.value && smartStatus.value.sessionKey.toLowerCase()===selectedAgent.value?.session_address?.toLowerCase() && Number(smartStatus.value.expiresAt)*1000>clockNow.value)
const exactUSDC=(atomic:string)=>(BigInt(atomic)/1000000n).toString()+'.'+(BigInt(atomic)%1000000n).toString().padStart(6,'0')
const agentBusy=ref(false), funding=ref(false), walletError=ref(''), balance=ref(''), fundAmount=ref('10'), fundTx=ref('')
const selectedAgent=computed(()=>agents.value.find(a=>a.id===agentID.value))
const pending=ref<Request|null>(null)
const initializing=ref(true)
let timer:ReturnType<typeof setInterval>|undefined
const active=computed(()=>sending.value || ['queued','running'].includes(task.value?.status??''))
const statusNames:Record<string,string>={queued:'Queued',running:'Agent running',previewed:'Preview complete · No payment',settled:'Settled on testnet',held:'On hold',unknown:'Settlement unconfirmed',error:'Failed'}
const eventNames:Record<string,string>={model:'Agent reasoning and tool choice',tool:'Tool call',candidate:'Offer and risk screening',selection:'Policy selection',final_risk:'Final pre-signing check',authorization_reserved:'Payment authorization reserved',chain_reservation:'Onchain budget reservation'}
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
// The green circle leaves its column when the page scrolls past it: it runs
// along the top edge to the right, hits the wall, then rolls down it, turning
// as it goes. Its position is computed from the scroll offset rather than
// played as an animation, so scrolling back up runs it backwards exactly.
//
// It picks up from exactly where it sits in its column rather than from the
// left of the screen — otherwise it teleports across the page the moment it
// detaches, because its column is on the right.
// The circle's size now tracks the viewport, so the roller reads it from the
// element rather than assuming a fixed 200px — otherwise on a smaller laptop
// it would travel the wrong distance and stop short of the right edge.
const rollSize=ref(200)
const rolling=ref(false)
const rollX=ref(0)
const rollY=ref(0)
const rollSpin=ref(0)
let rollAnchor:HTMLElement|null=null
function onScroll(){
 if(!rollAnchor)rollAnchor=document.querySelector('.onboard-shape.circle')
 if(!rollAnchor){rolling.value=false;return}
 const box=rollAnchor.getBoundingClientRect()
 const drawnSize=parseFloat(getComputedStyle(rollAnchor).getPropertyValue('--shape'))
 if(drawnSize>0)rollSize.value=drawnSize
 const past=-box.top
 if(past<=0){rolling.value=false;rollSpin.value=0;return}
 rolling.value=true
 // The element is a full-width box with the circle drawn centred inside it,
 // so its left edge is well left of the circle itself. Measuring the box made
 // the roller jump leftwards the moment it appeared; this is where the circle
 // actually is.
 const drawn=Math.min(box.width,rollSize.value)
 const startX=box.left+(box.width-drawn)/2
 const acrossDistance=Math.max(0,window.innerWidth-drawn-startX)
 const downDistance=Math.max(1,window.innerHeight-drawn)
 const across=Math.min(acrossDistance,past)
 const down=Math.min(downDistance,Math.max(0,past-acrossDistance))
 rollX.value=startX+across
 rollY.value=down
 // one turn per circumference, so it reads as rolling rather than sliding
 rollSpin.value=((across+down)/(Math.PI*drawn))*360
}

// The headline's first word can be pushed along its line, from where it starts
// to the right edge of the page's own content.
//
// It moves in steps rather than freely: each step lands with a short tick, so
// dragging it has a detent you can feel rather than being a smooth slide. The
// tick is the Vibration API where a device has one — on a desktop it is simply
// the stepping itself, which is what gives it the ratchet.
const dragWord=ref<HTMLElement|null>(null)
const wordX=ref(0)
const dragging=ref(false)
const STEP=26
let dragFrom=0,dragStartX=0,dragMax=0,lastNotch=0
function startDrag(e:PointerEvent){
 const el=dragWord.value
 if(!el)return
 const shell=el.closest('.shell') as HTMLElement|null
 if(!shell)return
 e.preventDefault()   // a drag is not a text selection
 const shellBox=shell.getBoundingClientRect()
 const pad=parseFloat(getComputedStyle(shell).paddingRight||'0')
 dragMax=Math.max(0,(shellBox.right-pad)-el.getBoundingClientRect().right+wordX.value)
 dragging.value=true
 dragFrom=wordX.value
 dragStartX=e.clientX
 lastNotch=Math.round(wordX.value/STEP)
 el.setPointerCapture(e.pointerId)
 const move=(ev:PointerEvent)=>{
  ev.preventDefault()
  const raw=Math.min(dragMax,Math.max(0,dragFrom+(ev.clientX-dragStartX)))
  const notch=Math.round(raw/STEP)
  const snapped=Math.min(dragMax,notch*STEP)
  if(notch!==lastNotch){
   lastNotch=notch
   navigator.vibrate?.(8)
  }
  wordX.value=snapped
 }
 const up=(ev:PointerEvent)=>{
  dragging.value=false
  el.releasePointerCapture?.(ev.pointerId)
  el.removeEventListener('pointermove',move)
  el.removeEventListener('pointerup',up)
  el.removeEventListener('pointercancel',up)
 }
 el.addEventListener('pointermove',move)
 el.addEventListener('pointerup',up)
 el.addEventListener('pointercancel',up)
}
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
 if(agentID.value){await refreshBalance();await refreshSmart()}
}
function chooseAgent(){smartStatus.value=null;smartTx.value='';linkAddress.value='';allowRecipients.value='';void refreshSmart();actionPassword.value='';walletNotice.value='';localStorage.setItem('decision402-agent',agentID.value);balance.value='';fundTx.value='';void refreshBalance()}
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
 walletError.value='';walletNotice.value='';agentBusy.value=true
 try{
  if(!owner.value)throw new Error('Connect your owner wallet first')
  if(walletPassword.value.length<12)throw new Error('Use at least 12 characters for the wallet password')
  if(walletPassword.value!==passwordConfirm.value)throw new Error('The two wallet passwords do not match')
  if(backupFile.value && backupFile.value.size>16384)throw new Error('Wallet backup is too large')
  let keystore:unknown
  if(backupFile.value){try{keystore=JSON.parse(await backupFile.value.text())}catch{throw new Error('Choose a valid encrypted keystore file')}}
  const response=await fetch(backupFile.value?'/api/agents/restore':'/api/agents',{method:'POST',headers:apiHeaders,body:JSON.stringify({wallet_kind:backupFile.value && restoreKind.value==='legacy'?'':'smart',name:agentName.value,model_url:modelURL.value,model_name:modelName.value,model_api_key:modelAPIKey.value.trim(),password:walletPassword.value,...(keystore?{keystore}:{})})})
  const result=await response.json()
  if(!response.ok)throw new Error(result.error??'Could not create agent')
  modelAPIKey.value=''
  agents.value.push(result as Agent);agentID.value=result.id;localStorage.setItem('decision402-agent',result.id);balance.value='0';fundTx.value=''
   smartStatus.value=null;walletNotice.value=result.wallet_kind==='smart'?'Session key saved and locked. Deploy a smart wallet below, or link your existing one.':'Ordinary wallet restored and locked. Download a backup.'
 }catch(e){walletError.value=e instanceof Error?e.message:'Could not create agent'}finally{agentBusy.value=false;walletPassword.value='';passwordConfirm.value='';modelAPIKey.value=''}
}
function selectBackup(event:Event){backupFile.value=(event.target as HTMLInputElement).files?.[0]??null}
async function walletAction(action:'unlock'|'lock'|'backup'){
 const agent=selectedAgent.value;if(!agent)return
 walletError.value='';walletNotice.value='';agentBusy.value=true
 try{
  const response=await fetch('/api/agents/'+agent.id+'/'+action,{method:'POST',headers:apiHeaders,body:JSON.stringify(action==='lock'?{}:{password:actionPassword.value})})
  if(!response.ok){const result=await response.json();throw new Error(result.error??'Wallet action failed')}
  if(action==='backup'){
   const blob=await response.blob(), url=URL.createObjectURL(blob), link=document.createElement('a')
   link.href=url;link.download='decision402-'+agent.id+'.keystore.json';document.body.appendChild(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),1000)
   walletNotice.value='Backup download started. Check your downloads and keep the password separately. The model API key is not included.'
  }else{
   const result=await response.json() as Agent
   agents.value=agents.value.map(a=>a.id===result.id?result:a)
   walletNotice.value=action==='lock'?'Wallet locked. New signatures are blocked.':'Unlocked for up to 10 minutes.'
  }
 }catch(e){walletError.value=e instanceof Error?e.message:'Wallet action failed'}finally{actionPassword.value='';agentBusy.value=false}
}
async function disconnectWallet(){
 walletError.value=''
 try{
  const response=await fetch('/api/auth/logout',{method:'POST',headers:apiHeaders,body:'{}'})
  if(!response.ok && response.status!==401)throw new Error('Could not lock wallets; try again')
  clearWalletUI()
 }catch(e){walletError.value=e instanceof Error?e.message:'Could not disconnect'}
}
function clearWalletUI(){smartStatus.value=null;linkAddress.value='';allowRecipients.value='';smartTx.value='';owner.value='';agents.value=[];agentID.value='';balance.value='';task.value=null;actionPassword.value='';walletPassword.value='';passwordConfirm.value='';modelAPIKey.value=''}
async function refreshBalance(){
 const agent=selectedAgent.value
 if(!agent?.wallet || !config.value)return
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
  if(!agent?.wallet || !config.value || !owner.value)throw new Error('Connect and create an agent first')
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

async function smartAPI(agent:Agent,action:string,body:unknown={}){
 const r=await fetch('/api/agents/'+agent.id+'/smart/'+action,{method:'POST',headers:apiHeaders,body:JSON.stringify(body)})
 const data=await r.json();if(!r.ok)throw new Error(data.error??'Smart wallet request failed');return data
}
async function refreshSmart(){
 const agent=selectedAgent.value;if(agent?.wallet_kind!=='smart' || !agent.wallet)return
 try{const r=await fetch('/api/agents/'+agent.id+'/smart'),data=await r.json();if(!r.ok)throw new Error(data.error??'Could not read authorization');if(agentID.value===agent.id)smartStatus.value=data}
 catch(e){if(agentID.value===agent.id){smartStatus.value=null;walletError.value=e instanceof Error?e.message:'Could not read authorization'}}
}
async function sendOwnerTransaction(tx:Record<string,string>){
 await ensureBaseSepolia()
 const accounts=await provider().request({method:'eth_accounts'}) as string[]
 if(tx.from.toLowerCase()!==owner.value.toLowerCase() || !accounts.some(a=>a.toLowerCase()===tx.from.toLowerCase()))throw new Error('MetaMask owner changed; connect again')
 const hash=await provider().request({method:'eth_sendTransaction',params:[tx]}) as string
 smartTx.value=hash
 for(let i=0;i<60;i++){
  const receipt=await provider().request({method:'eth_getTransactionReceipt',params:[hash]}) as {status:string;contractAddress?:string}|null
  if(receipt){if(receipt.status!=='0x1')throw new Error('Transaction failed onchain');return receipt}
  await new Promise(resolve=>setTimeout(resolve,1500))
 }
 throw new Error('Transaction is still pending. Check the link before sending again. For deployment, copy its contract address into Link existing wallet once mined.')
}
async function smartAction(action:'deploy'|'link'|'authorize'|'revoke'|'withdraw'|'gas'){
 const agent=selectedAgent.value;if(!agent || agent.wallet_kind!=='smart')return
 smartBusy.value=true;walletError.value='';walletNotice.value=''
 try{
  if(action==='link'){
   const updated=await smartAPI(agent,'link',{address:linkAddress.value.trim()}) as Agent
   agents.value=agents.value.map(a=>a.id===updated.id?updated:a)
   walletNotice.value='Wallet linked. The owner, USDC token, and contract code were checked.'
  }else if(action==='gas'){
   if(!agent.session_address || !/^0\.\d{1,8}$/.test(gasAmount.value))throw new Error('Enter between 0 and 0.01 test ETH')
   const units=BigInt(gasAmount.value.split('.')[1].padEnd(18,'0'))
   if(units<=0n || units>10000000000000000n)throw new Error('Use at most 0.01 test ETH')
   await sendOwnerTransaction({from:owner.value,to:agent.session_address,value:'0x'+units.toString(16),chainId:'0x14a34'})
   walletNotice.value='Session gas funded. This address pays reservation fees; deposit USDC into the smart wallet.'
  }else{
   let body:unknown={}
   if(action==='authorize'){
    const hours=Number(grantHours.value)
    if(!Number.isFinite(hours) || hours<=0 || hours>720)throw new Error('Choose an expiry within 720 hours')
    body={recipients:allowRecipients.value.split(/[\s,;]+/).filter(Boolean),per_payment:grantPer.value,daily_limit:grantDaily.value,expires_at:Math.floor(Date.now()/1000+hours*3600)}
   }
   if(action==='withdraw')body={amount:withdrawAmount.value}
   let tx:Record<string,string>
   if(action==='revoke' || action==='withdraw'){
    const data=action==='revoke'?'0xb6549f75':'0x2e1a7d4d'+usdcUnits(withdrawAmount.value).toString(16).padStart(64,'0')
    tx={from:agent.owner,to:agent.wallet,value:'0x0',data,chainId:'0x14a34'}
   }else tx=await smartAPI(agent,action,body)
   if(tx.from.toLowerCase()!==agent.owner.toLowerCase() || (action!=='deploy' && tx.to.toLowerCase()!==agent.wallet.toLowerCase()) || tx.chainId!=='0x14a34')throw new Error('Unexpected transaction destination')
   const receipt=await sendOwnerTransaction(tx)
   if(action==='deploy'){
    if(!receipt.contractAddress)throw new Error('No deployed contract address in receipt')
    linkAddress.value=receipt.contractAddress
    const updated=await smartAPI(agent,'link',{address:receipt.contractAddress}) as Agent
    agents.value=agents.value.map(a=>a.id===updated.id?updated:a)
    walletNotice.value='Smart wallet deployed and linked. Fund USDC, fund session gas, then authorize recipients.'
   }else walletNotice.value=action==='authorize'?'Authorization confirmed onchain. Unlock the session key to run tasks.':action==='revoke'?'Revocation confirmed. Unsettled signatures from this authorization are now invalid.':'Test USDC returned to the owner wallet.'
  }
  await refreshSmart();await refreshBalance()
 }catch(e){walletError.value=e instanceof Error?e.message:'Smart wallet action failed'}finally{smartBusy.value=false}
}

function walletChanged(accounts:string[]){
 if(owner.value && !accounts.some(a=>a.toLowerCase()===owner.value.toLowerCase())){void disconnectWallet();walletError.value='Wallet account changed. Connect again.'}
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
 walletClock=setInterval(()=>{clockNow.value=Date.now()},1000)
 window.ethereum?.on?.('accountsChanged',walletChanged)
 try{const r=await fetch('/api/config');if(!r.ok)throw new Error('Backend is not running');config.value=await r.json()
  const session=await fetch('/api/auth/me')
  if(session.ok){const current=await session.json();const accounts=window.ethereum?await window.ethereum.request({method:'eth_accounts'}) as string[]:[];if(accounts?.some(a=>a.toLowerCase()===current.owner.toLowerCase())){owner.value=current.owner;await fetchAgents()}}
  const saved=localStorage.getItem('decision402-pending');if(saved)pending.value=JSON.parse(saved)
  const id=new URLSearchParams(location.search).get('task')??localStorage.getItem('decision402-last');if(id && owner.value){await getTask(id);if(task.value){policy.value={...task.value.request.policy,max_risk:task.value.request.mode==='simulate'?task.value.request.policy.max_risk:0};mode.value=task.value.request.mode;instruction.value=task.value.request.instruction;agentID.value=task.value.request.agent_id??agentID.value}if(['queued','running'].includes(task.value?.status??''))watchTask(id)}
 }catch(e){error.value=e instanceof Error?e.message:'Connection failed'}finally{initializing.value=false}
})
// The roller reads the scroll position directly; passive, because it never
// prevents the scroll it is following.
onMounted(()=>{window.addEventListener('scroll',onScroll,{passive:true});window.addEventListener('resize',onScroll);onScroll()})
onUnmounted(()=>{clearInterval(timer);clearInterval(walletClock);actionPassword.value='';walletPassword.value='';passwordConfirm.value='';modelAPIKey.value='';window.ethereum?.removeListener?.('accountsChanged',walletChanged);window.removeEventListener('scroll',onScroll);window.removeEventListener('resize',onScroll)})
</script>

<template>
 <div class="shell">
  <header class="topbar">
   <a class="brand" href="/" aria-label="Decision402 home">
    
    <span>Decision<span class="brand-number">402</span><small>AGENT PAYMENT CONTROL</small></span><img class="brand-mark" src="/decision402-mark.png" alt="" width="40" height="40" />
   </a>
   <div class="header-right"><span class="connection"><i :class="{online:!!config}"></i>{{owner?shortAddress(owner):config?'Connect owner wallet':'Connecting'}}</span><span class="network"><i></i>Base Sepolia <b>TESTNET</b></span></div>
  </header>

  <section class="intro wide">
   <div class="intro-copy"><div class="eyebrow"><span></span>YOUR POLICY. AGENT ACTION.</div><h1>
     <span class="drag-word" ref="dragWord" :style="{transform:'translateX('+wordX+'px)'}"
       @pointerdown="startDrag" @dragstart.prevent @selectstart.prevent :class="{dragging:dragging}"
       title="Drag me"><Scramble text="Autonomy." :tail="5" :delay="260" /></span>
     <br><span class="line-two"><Scramble text="Within bounds." :tail="7" :delay="620" /></span>
    </h1></div>
   </section>

  <section class="onboarding" aria-label="Set up your agent wallet">
   <div class="onboard-title"><span class="eyebrow">YOUR AGENT WORKSPACE</span><h2>Give an agent its own wallet.</h2><p>Keep control in MetaMask. Create a smart wallet, choose who the agent may pay, and set its limits. Base Sepolia test funds only.</p></div>
   <div class="onboard-grid">
    <div class="onboard-col"><div class="onboard-card"><span class="onboard-step">01 / OWNER</span><h3>Connect MetaMask</h3><p>Sign a message to prove ownership. No transaction is sent at this step.</p><button class="onboard-button" :disabled="agentBusy" @click="connectWallet">{{owner?'Switch / reconnect':'Connect wallet'}} ↗</button><code v-if="owner">{{owner}}</code><button v-if="owner" class="ghost-button" @click="disconnectWallet">Lock wallets and disconnect</button></div><div class="onboard-shape tri" aria-hidden="true"></div></div>
    <div class="onboard-col"><div class="onboard-card"><span class="onboard-step">02 / AGENT</span><h3>Create your buyer</h3><div class="onboard-fields"><label>Agent name<input v-model="agentName" maxlength="40" placeholder="Tokyo buyer" /></label><label>Model API URL<input v-model="modelURL" spellcheck="false" /></label><label>Model<Dropdown v-model="modelName" :options="[{value:'deepseek-flash',label:'DeepSeek Flash'},{value:'deepseek-v4-pro',label:'DeepSeek V4 Pro'}]" /></label><label>DeepSeek API key <small>optional if configured on server</small><input v-model="modelAPIKey" type="password" autocomplete="off" placeholder="Server default or your own key" /></label><label>Wallet password<input v-model="walletPassword" type="password" autocomplete="new-password" minlength="12" maxlength="1024" placeholder="At least 12 characters" /></label><label>Repeat password<input v-model="passwordConfirm" type="password" autocomplete="new-password" maxlength="1024" /></label><label>Restore an encrypted wallet <small>optional; use the backup's password</small><input type="file" accept=".json,application/json" @change="selectBackup" /></label><label v-if="backupFile">Backup type<Dropdown v-model="restoreKind" :options="[{value:'legacy',label:'Ordinary Agent wallet'},{value:'smart',label:'Smart wallet session key'}]" /></label><small>New agents use a smart wallet. The backup protects its session key; your MetaMask keeps owner control.</small></div><button class="onboard-button" :disabled="!owner || agentBusy" @click="createAgent">{{agentBusy?'Working…':backupFile?'Restore key':'Create agent + session key'}} ↗</button></div></div>
    <div class="onboard-col"><div class="onboard-card"><span class="onboard-step">03 / FUND</span><h3>Fund the agent</h3><label v-if="agents.length">Choose agent<Dropdown v-model="agentID" @change="chooseAgent" :options="agents.map(a=>({value:a.id,label:a.name+' · '+(a.wallet?shortAddress(a.wallet):'Needs deployment'),short:a.name}))" /></label><div v-if="selectedAgent" class="agent-address"><small>AGENT WALLET · BASE SEPOLIA</small><code>{{selectedAgent.wallet || 'Deploy the smart wallet below first'}}</code><span>Balance: {{balance || '—'}} test USDC</span></div><p v-else>Create an agent to get its deposit address.</p><label>Amount · test USDC<input v-model="fundAmount" inputmode="decimal" /></label><div class="onboard-actions"><button class="onboard-button" :disabled="!selectedAgent?.wallet || funding || smartBusy" @click="fundAgent">{{funding?'Waiting for confirmation…':'Fund with MetaMask'}} ↗</button><button class="ghost-button" :disabled="!selectedAgent?.wallet" @click="refreshBalance">Refresh balance</button></div><a v-if="fundTx" :href="'https://sepolia.basescan.org/tx/'+fundTx" target="_blank" rel="noopener noreferrer">View funding transaction ↗</a></div><div class="onboard-shape circle" :class="{away:rolling}" aria-hidden="true"></div></div>
   </div>
   <div v-if="selectedAgent" class="wallet-security">
    <h3>Wallet security · {{walletLocked?'Locked':'Unlocked'}}</h3>
    <p>Unlock lasts up to 10 minutes. Locking stops new signatures. Use Revoke onchain to stop unsettled smart wallet payments.</p>
    <label>Wallet password<input v-model="actionPassword" type="password" autocomplete="off" maxlength="1024" /></label>
    <div class="onboard-actions">
     <button class="onboard-button" :disabled="agentBusy" @click="walletAction('unlock')">Unlock</button>
     <button class="ghost-button" @click="walletAction('lock')">Lock now</button>
     <button class="ghost-button" :disabled="agentBusy" @click="walletAction('backup')">Download encrypted backup</button>
    </div>
   </div>
   <div v-if="selectedAgent?.wallet_kind==='smart'" class="wallet-security smart-controls">
    <h3>Smart wallet · {{selectedAgent.wallet?(smartAuthorized?'Authorized':'Not authorized'):'Needs deployment'}}</h3>
    <p>Your MetaMask controls this wallet. The agent can only pay approved recipients within the limits below.</p><a href="/wallet-control.html" download="decision402-owner-controls.html">Save standalone owner controls ↗</a>
    <div v-if="!selectedAgent.wallet" class="onboard-fields">
     <button class="onboard-button" :disabled="smartBusy" @click="smartAction('deploy')">Deploy with MetaMask</button>
     <label>Or link an existing Decision402 smart wallet<input v-model="linkAddress" placeholder="0x…" spellcheck="false" /></label>
     <button class="ghost-button" :disabled="smartBusy || !linkAddress" @click="smartAction('link')">Link existing wallet</button>
    </div>
    <template v-else>
     <div v-if="smartStatus" class="smart-status">
      <span>Onchain cap: {{exactUSDC(smartStatus.perPayment)}} USDC / payment · {{exactUSDC(smartStatus.dailyLimit)}} / UTC day</span>
      <span>Reserved today: {{exactUSDC(smartStatus.reserved_today)}} USDC · Expires: {{Number(smartStatus.expiresAt)?new Date(Number(smartStatus.expiresAt)*1000).toLocaleString():'No active grant'}}</span>
      <span>Approved recipients: {{smartStatus.getRecipients.join(', ') || 'None'}}</span>
      <span>Session gas: {{(Number(smartStatus.gas_wei)/1e18).toFixed(8)}} test ETH</span>
     </div>
     <p>Daily limits reset at 00:00 UTC (08:00 in China). Failed payment reservations still count today. Each purchase also uses test ETH to reserve its budget onchain.</p>
     <div class="onboard-fields">
      <label>Recipient whitelist · one address per line<textarea v-model="allowRecipients" rows="3" placeholder="Paste merchant pay_to addresses" spellcheck="false"></textarea></label>
      <div class="row"><label>Per payment · USDC<input v-model="grantPer" inputmode="decimal" /></label><label>Per UTC day · USDC<input v-model="grantDaily" inputmode="decimal" /></label><label>Expires after · hours<input v-model="grantHours" inputmode="decimal" /></label></div>
     </div>
     <div class="onboard-actions">
      <button class="onboard-button" :disabled="smartBusy" @click="smartAction('authorize')">Authorize with MetaMask</button>
      <button class="ghost-button" :disabled="smartBusy" @click="smartAction('revoke')">Revoke onchain</button>
      <button class="ghost-button" :disabled="smartBusy" @click="refreshSmart">Refresh authorization</button>
     </div>
     <p>Authorization and revocation take effect after the transaction is mined. Revocation cannot undo a completed payment.</p>
     <div class="row"><label>Session gas · test ETH<input v-model="gasAmount" inputmode="decimal" /></label><button class="ghost-button" :disabled="smartBusy" @click="smartAction('gas')">Fund session gas</button></div>
     <small>Session address · gas only: <code>{{selectedAgent.session_address}}</code></small>
     <div class="row"><label>Return to owner · test USDC<input v-model="withdrawAmount" inputmode="decimal" /></label><button class="onboard-button" :disabled="smartBusy || !withdrawAmount" @click="smartAction('withdraw')">Withdraw with MetaMask</button></div>
     <small>Withdrawal recipient: {{selectedAgent.owner}}</small>
    </template>
    <a v-if="smartTx" :href="'https://sepolia.basescan.org/tx/'+smartTx" target="_blank" rel="noopener noreferrer">View wallet transaction ↗</a>
   </div>
   <p v-if="walletNotice" role="status">{{walletNotice}}</p>
   <p v-if="walletError" class="error" role="alert">{{walletError}}</p>
   <p class="onboard-disclaimer">Local testnet prototype. Wallet keys and personal model keys are encrypted on disk. Smart wallet owner control stays in MetaMask. Export your encrypted wallet backup before moving this workspace. Use test USDC only.</p>
  </section>
  <section class="ask">
   <form class="ask-field" @submit.prevent="start">
    <input v-model="instruction" maxlength="2000" :disabled="active || !!pending"
      placeholder="What should the agent buy?" aria-label="What should the agent buy" />
    <button type="submit" :disabled="initializing || active || !config || !selectedAgent || blockedPayment || (mode==='pay' && selectedAgent.wallet_kind==='smart' && (!selectedAgent.wallet || !smartAuthorized)) || (walletLocked && (mode==='pay' || selectedAgent.has_model_key))">
     <span>{{active?'Working':pending?'Retry':mode==='pay'?'Authorize payment':'Send'}}</span>
     <i :class="{spinner:active}" aria-hidden="true">{{active?'':'→'}}</i>
    </button>
   </form>
   <div class="ask-meta">
    <span v-if="!selectedAgent" class="ask-need">Create an agent above before sending a task.</span>
    <span v-else-if="mode==='pay' && selectedAgent.wallet_kind==='smart' && !smartAuthorized">Deploy and authorize your smart wallet before payment.</span>
    <span v-else-if="walletLocked && (mode==='pay' || selectedAgent.has_model_key)">Unlock the agent wallet above before sending this task.</span><span v-else>Up to <b>{{policy.per_payment}}</b> USDC per payment · <b>{{policy.task_budget}}</b> total · risk &le; <b>{{policy.max_risk}}</b> · <b>{{policy.preference==='price'?'price first':'risk first'}}</b></span>
    <span class="ask-mode">{{mode==='simulate'?'POLICY SIMULATION':mode==='preview'?'LIVE PREVIEW · NO PAYMENT':'TESTNET EXECUTION'}}</span>
   </div>
  </section>
  <div class="workspace-heading"><div><span class="eyebrow">PAYMENT WORKSPACE</span><p>From an instruction to an accountable decision.</p></div><div class="workspace-actions"><span class="workspace-note">Mainnet risk data <span>↔</span> Testnet settlement</span><button class="new-task" :disabled="!canStartFresh" @click="newTask"><span aria-hidden="true">+</span> New task</button></div></div>
  <main>
   <section class="panel controls">
    <div class="section-title"><div><h2>Define the boundaries</h2><p>Your authorization comes first.</p></div></div>
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
     <div class="section-title"><div><h2>The moment of decision</h2><p>{{task?(active?'Execution in progress':'Recorded task result'):'Watch your policy become action.'}}</p></div><span class="status" :class="task?.status" role="status">{{task?statusNames[task.status]:'Ready when you are'}}</span></div>
     <div class="flow-strip"><div v-for="(item,index) in flowSteps" :key="item.label" :class="{done:item.done,working:item.active}"><span class="flow-orb">{{item.done?'✓':''}}</span><b>{{item.label}}</b></div></div>
     <template v-if="task">
      <div class="run-policy"><span class="run-mode">{{taskModeLabel}}</span><span>≤ {{task.request.policy.per_payment}} / payment</span><span>≤ {{task.request.policy.task_budget}} total</span><span>Risk ≤ {{task.request.policy.max_risk}}</span><span>{{task.request.policy.preference==='price'?'Price first':'Risk first'}}</span></div>
      <div v-if="task.candidates.length" class="decision-metrics"><div><strong>{{task.candidates.length}}</strong><span>offers evaluated</span></div><div><strong class="metric-blocked">{{excludedCount}}</strong><span>excluded by policy</span></div><div><strong class="metric-eligible">{{eligibleCount}}</strong><span>eligible options</span></div></div>
      <div v-if="task.candidates.length" class="candidate-list">
       <div class="candidate-head"><span>SERVICE / RECIPIENT</span><span>PRICE · USDC</span><span>RISK CHECK</span><span>DECISION</span></div>
       <article v-for="(c,ci) in task.candidates" :key="c.service.id" class="candidate"
        :style="{animationDelay:(ci*110)+'ms'}"
        :class="{chosen:task.selected?.service.id===c.service.id,
                 rejected:!!c.risk&&!c.eligible,
                 screening:!c.risk,
                 settled:!!c.risk}">
        <div class="candidate-row">
         <div class="provider"><span class="provider-avatar">{{c.service.id}}</span><div><b>Provider {{c.service.id}}</b><small>{{shortAddress(c.service.pay_to)}}</small></div></div>
         <div class="quote-price">{{money(c.quote?.amount??c.service.amount)}}</div>
         <div class="risk-cell"><span class="risk-badge" :class="!c.risk?'checking':c.level<0?'unknown':c.level>=2?'high':c.level===1?'advisory':'clear'"><i></i>{{!c.risk?'Checking':c.level<0?'Unknown':c.level>=2?'Blocked':c.level===1?'Advisory':'No signal'}}</span><small>{{c.risk?.toxicScore!==undefined?'Score '+c.risk.toxicScore+'/100':'Level '+c.level+(task.request.mode==='simulate'?' · simulated':'')}}</small></div>
         <div class="outcome"><span :class="c.eligible?'allow':'deny'">{{task.selected?.service.id===c.service.id?'✓ Selected':c.eligible?'Eligible':'× Excluded'}}</span></div>
        </div>
        <div class="candidate-reason"><span class="reason-mark">{{c.eligible?'↳':'⊘'}}</span><span>{{displayReason(c)}}</span></div>
        <details v-if="c.risk?.traits?.length" class="risk-details"><summary><span v-for="trait in c.risk.traits" :key="trait.name" class="trait-tag">{{traitLabel(trait.name)}}</span><span class="detail-link">View risk evidence ↗</span></summary><p v-for="trait in c.risk.traits" :key="trait.name">{{trait.description}}</p></details>
       </article>
      </div>
      <div v-else class="empty loading-state"><span class="orbit" aria-hidden="true"></span><h3>Asking each provider to quote.</h3><p>Every recipient becomes known here, and not before.</p></div>
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
     <div class="section-title"><div><h2>Execution trail</h2><p>The evidence behind the decision.</p></div><span class="event-count">{{task?.events.length??0}} EVENTS</span></div>
     <div v-if="!task?.events.length" class="trail-empty"><span>↳</span>Agent calls, risk checks, and payment records will appear here.</div>
     <details v-for="(e,i) in task?.events??[]" :key="i" class="event"><summary><span class="event-index">{{String(i+1).padStart(2,'0')}}</span><span class="event-name">{{eventNames[e.kind]??e.kind}}</span><time>{{e.time.slice(11,19)}}</time><span class="event-expand">+</span></summary><pre>{{JSON.stringify(e.data,null,2)}}</pre></details>
    </div>
   </section>
  </main>
  <div v-show="rolling" class="roller" aria-hidden="true"
    :style="{transform:'translate3d('+rollX+'px,'+rollY+'px,0) rotate('+rollSpin+'deg)'}"></div>
   <footer><a href="/" class="footer-brand">Decision402<span>Designed for delegated decisions.</span></a><span>Local prototype · Model has no private key · Static sample data</span></footer>
 </div>
</template>
