<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
type Policy={per_payment:string;task_budget:string;max_risk:number;preference:string}
type Request={id:string;instruction:string;mode:string;policy:Policy}
type Candidate={service:{id:string;name:string;pay_to:string;amount:string};level:number;eligible:boolean;reason:string;source:string;quote?:{amount:string};risk?:{toxicScore?:number;traits?:{name:string;description:string}[];duration_ms:number}}
type Task={request:Request;status:string;candidates:Candidate[];selected?:Candidate;events:{time:string;kind:string;data:unknown}[];summary:string;error?:string;payment_attempted:boolean;payment?:{settled:boolean;signed:boolean;transaction?:string;error?:string;data?:unknown}}
const instruction=ref('帮我获取一份东京天气数据，按照我预设的预算和风险偏好选择服务。')
const mode=ref('simulate')
const policy=ref<Policy>({per_payment:'0.10',task_budget:'0.10',max_risk:1,preference:'price'})
const task=ref<Task|null>(null), config=ref<{model:string}|null>(null), error=ref(''), sending=ref(false)
const pending=ref<Request|null>(null)
let timer:ReturnType<typeof setInterval>|undefined
const active=computed(()=>sending.value || ['queued','running'].includes(task.value?.status??''))
const statusNames:Record<string,string>={queued:'等待执行',running:'Agent 执行中',previewed:'预览完成 · 未付款',settled:'测试网已结算',held:'已暂停',unknown:'结算待核对',error:'执行失败'}
const eventNames:Record<string,string>={model:'模型理解与工具选择',tool:'请求调用工具',candidate:'报价与风险检查',selection:'策略选定服务',final_risk:'签名前再次检查',authorization_reserved:'记录付款授权'}
const money=(value:string)=> (Number(value)/1e6).toFixed(3)
const levels=['0 · 未发现风险信号','1 · 已授权的轻微信号','2 · 策略阻断']
const blockedPayment=computed(()=>mode.value==='pay' && !!task.value?.payment_attempted && !task.value?.payment?.settled)
function onModeChange(){if(mode.value!=='simulate')policy.value.max_risk=0}
async function getTask(id:string){
 const response=await fetch('/api/tasks/'+encodeURIComponent(id))
 if(!response.ok) throw new Error(response.status===404?'尚未创建任务，可重试原请求。':'读取任务失败')
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
  if(!response.ok){if(response.status<500){pending.value=null;localStorage.removeItem('decision402-pending')}throw new Error(result.error??'创建任务失败')}
  task.value=result
  history.replaceState(null,'','?task='+encodeURIComponent(request.id))
  if(['queued','running'].includes(result.status))watchTask(request.id)
  else{pending.value=null;localStorage.removeItem('decision402-pending')}
 }catch(e){error.value=e instanceof Error?e.message:'连接失败；重试会使用相同任务 ID'}finally{sending.value=false}
}
onMounted(async()=>{
 try{const r=await fetch('/api/config');if(!r.ok)throw new Error('后端未启动');config.value=await r.json()
  const saved=localStorage.getItem('decision402-pending');if(saved)pending.value=JSON.parse(saved)
  const id=new URLSearchParams(location.search).get('task')??localStorage.getItem('decision402-last');if(id){await getTask(id);if(task.value){policy.value={...task.value.request.policy,max_risk:task.value.request.mode==='simulate'?task.value.request.policy.max_risk:0};mode.value=task.value.request.mode;instruction.value=task.value.request.instruction}if(['queued','running'].includes(task.value?.status??''))watchTask(id)}
 }catch(e){error.value=e instanceof Error?e.message:'连接失败'}
})
onUnmounted(()=>clearInterval(timer))
</script>

<template>
 <div class="shell">
  <header><a class="brand" href="/">D<span>402</span><i>Decision402</i></a><span class="network"><b></b> BASE SEPOLIA · TESTNET</span></header>
  <section class="intro"><div class="eyebrow">YOUR POLICY. AGENT ACTION.</div><h1>让 Agent 做事，<br><span>让授权掌管付款。</span></h1><p>预设预算和风险偏好。Agent 理解任务，策略引擎筛选服务，签名前再次检查。</p></section>
  <main>
   <section class="panel controls">
    <div class="section-title"><span class="step">01</span><h2>设置授权与任务</h2></div>
    <fieldset :disabled="active || !!pending"><label>执行模式 / Mode<select v-model="mode" @change="onModeChange"><option value="simulate">策略模拟 · 不付款</option><option value="preview">真实 API 预览 · 不付款</option><option value="pay">真实测试网执行 · 自动付款</option></select></label>
    <p class="notice" v-if="mode==='simulate'">真实 DeepSeek 理解任务；报价和风险为模拟数据，用于比较 C（等级 1）与 D（等级 0）。无 Intercepta 请求、无签名。</p>
    <p class="notice" v-else>真实 DeepSeek + Intercepta + x402 报价。当前 A/B 使用高风险样例，会被阻断；C/D 共用正常收款地址，策略选择价格更低的 C。没有真实轻微信号，等级 1 仅在模拟模式演示。未知风险暂停。</p>
    <div class="row"><label>单次上限 · USDC<input v-model="policy.per_payment" inputmode="decimal" /></label><label>任务总预算 · USDC<input v-model="policy.task_budget" inputmode="decimal" /></label></div>
    <div class="row"><label>允许的风险<select v-model.number="policy.max_risk"><option :value="0">仅等级 0</option><option v-if="mode==='simulate'" :value="1">等级 0 + 1（仅模拟）</option></select></label><label>选择优先级<select v-model="policy.preference"><option value="price">价格优先</option><option value="risk">风险优先</option></select></label></div>
    <p class="hint">真实模式只允许等级 0；高风险永远排除。等级 0 表示未发现信号，不代表绝对安全。每任务最多一笔付款；演示预算上限 0.10 USDC。</p>
    <label>你希望 Agent 做什么？<textarea v-model="instruction" rows="4" maxlength="2000"></textarea></label>
    <p class="hint">当前目录仅提供东京静态天气样例，不是实时天气。模型调用会消耗 DeepSeek 账户额度。</p>
    </fieldset>
    <div v-if="mode==='pay'" class="pay-note">点击即授权后端按本次设置花费 Base Sepolia 测试 USDC。钱包使用本地 .buyer-key。</div>
    <button class="primary" :disabled="active || !config || blockedPayment" @click="start">{{active?'执行中…':pending?'重试原任务（不会新建付款）':mode==='pay'?'授权并执行测试网付款':'运行 Agent'}} <span>→</span></button>
    <p v-if="blockedPayment" class="error">上一任务已预留授权但未确认结算。请核对记录，不要重复付款。</p>
    <p v-if="error" class="error" role="alert">{{error}}</p>
    <div class="model">MODEL <span>{{config?.model??'连接后端中…'}}</span></div>
   </section>
   <section class="results">
    <div class="panel">
     <div class="section-title"><span class="step">02</span><h2>Moment of Decision</h2><span class="status" :class="task?.status">{{task?statusNames[task.status]:'等待任务'}}</span></div>
     <template v-if="task">
      <div class="run-policy">本次授权：单次 {{task.request.policy.per_payment}} / 总计 {{task.request.policy.task_budget}} USDC · 风险 ≤ {{task.request.policy.max_risk}} · {{task.request.policy.preference==='price'?'价格优先':'风险优先'}}<strong>{{task.request.mode==='simulate'?'模拟报价与风险':task.request.mode==='preview'?'真实 API · 只读':'真实测试网付款'}}</strong></div>
      <div class="table-wrap"><table><thead><tr><th>服务</th><th>USDC</th><th>风险等级</th><th>策略结果</th></tr></thead><tbody><tr v-for="c in task.candidates" :key="c.service.id" :class="{selected:task.selected?.service.id===c.service.id}"><td><b>{{c.service.id}}</b></td><td>{{money(c.quote?.amount??c.service.amount)}}</td><td>{{c.level<0?'未知':levels[c.level]}}<small v-if="c.risk?.toxicScore!==undefined">Toxic Score {{c.risk.toxicScore}}/100</small></td><td><span :class="c.eligible?'allow':'deny'">{{task.selected?.service.id===c.service.id?'已选中':c.eligible?'可接受':'排除'}}</span><small>{{c.reason}}</small><small v-if="c.risk?.traits?.length">风险原因：{{c.risk.traits.map(t=>t.description).join('；')}}</small></td></tr></tbody></table></div>
      <p v-if="!task.candidates.length" class="empty">正在等待模型理解任务与服务扫描…</p>
      <div v-if="task.summary" class="answer"><label>{{task.candidates.length?'后端决策说明 · 过滤 → 筛选 → 执行':'AGENT 说明'}}</label><p>{{task.summary}}</p></div>
      <p v-if="task.error" class="error">{{task.error}}</p>
      <div v-if="task.payment" class="receipt"><b>{{task.payment.settled?'已确认结算':'未确认结算'}}</b><p v-if="task.payment.error">{{task.payment.error}}</p><a v-if="/^0x[0-9a-fA-F]{64}$/.test(task.payment.transaction??'')" :href="'https://sepolia.basescan.org/tx/'+task.payment.transaction" target="_blank" rel="noopener noreferrer">查看 Base Sepolia 交易 ↗</a><pre v-if="task.payment.data">{{JSON.stringify(task.payment.data,null,2)}}</pre></div>
     </template>
     <div v-else class="empty"><div class="diagram"><span>任务</span> → <span>策略</span> → <span>检查</span> → <span>执行</span></div><p>左侧设置策略后开始。先比较模拟场景，<br>再用真实 API 预览，最后执行测试网付款。</p></div>
    </div>
    <div class="panel timeline"><div class="section-title"><span class="step">03</span><h2>执行记录</h2><small>可展开查看真实字段</small></div><div v-if="!task?.events.length" class="empty">每次报价、风险结果和付款决策都会记录在这里。</div><details v-for="(e,i) in task?.events??[]" :key="i"><summary><span class="dot"></span>{{eventNames[e.kind]??e.kind}}<time>{{e.time.slice(11,19)}}</time></summary><pre>{{JSON.stringify(e.data,null,2)}}</pre></details></div>
   </section>
  </main>
  <footer>Decision402 · Local hackathon prototype <span>模型不持有私钥 · 用户授权不可由模型修改 · 静态天气样例</span></footer>
 </div>
</template>
