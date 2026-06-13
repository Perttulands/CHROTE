/* ============================================================================
   CHROTE · FORMATIONS — INTERACTIVE DESIGN PROTOTYPE
   ----------------------------------------------------------------------------
   ⚠  EVERYTHING DATA-RELATED HERE IS MOCKED for feel: the agent roster, the
      terminals, every run, and every report/diff. Each mock is wrapped in a
      `MOCK` banner — search this file for "MOCK" before wiring a backend.

   ----------------------------------------------------------------------------
   PURPOSE OF THIS DOC
   This is a design prototype, not a spec. The notes below describe HOW THE
   FRONTEND IS MEANT TO WORK and, from that, WHAT A BACKEND WOULD HAVE TO PROVIDE
   for it to be real and useful. They are intentionally REQUIREMENTS, not a
   schema or architecture — model the data and services however you like, as long
   as the behaviours described here can be satisfied. Where we say "a node has an
   input", that is a UI affordance with a meaning, not a column.

   ----------------------------------------------------------------------------
   HOW THIS IS ACTUALLY OPERATED (critical context for the data model)
   The flows are AGENT-DRIVEN and primarily set up and run through a COMMAND-LINE
   INTERFACE, with the real definitions living as FILES ON DISK (the source of
   truth). Coding/orchestration agents are what actually author missions, wire
   formations, and configure gates — e.g. a gate's "Code" check is the tests /
   lints / type-checks an agent wired up, not something a human typed here.

   THIS UI IS AN INSPECTION + LIGHT-TWEAK SURFACE, not the authoring tool. Its job
   is to SURFACE these complicated flows and make them DIGESTIBLE for a human: see
   the shape of a mission's chain, what each step is, where work is waiting or
   blocked, what a gate checks and how it routed, and to nudge a thing (rename,
   re-wire, change a verdict/criterion, kick or cancel a run). We do NOT expect a
   human to hand-write most goals or hand-configure most gates.

   Implications for the backend / data model (note the requirements; the design
   work itself is deliberately deferred):
   • The on-disk file representation is canonical. This UI must READ that
     representation and render it faithfully, and WRITE BACK the small set of
     human edits — without clobbering agent-authored detail it doesn't surface.
     So the model needs a stable identity for every node/port/edge that round-
     trips between files, CLI, and this UI (edits are diffs against existing
     definitions, not full rewrites from the canvas).
   • Anything the CLI/agents can express, the file format must capture; this UI
     renders a (possibly partial) VIEW of it. Fields this UI never shows must
     survive an edit untouched. Expect the file model to be richer than the UI.
   • The same flow may be observed/edited here while agents mutate it via the CLI;
     the UI should reflect external changes (live-ish) and reconcile concurrent
     edits. Treat the canvas as one client of a shared, file-backed model.
   • "Run", "cancel", "set verdict", "edit criterion/objective", "re-wire" are the
     human actions this surface needs to issue against that system — define them
     as operations the CLI/engine already (or should) support.
   Goal of these notes: NOT to design the CLI, but to be thorough about what the
   system must be able to do when driven from a CLI, and what has to be true for
   this inspection surface to work against it in practice.


   An infinite canvas of NODES wired together into a directed graph (a DAG).
   The whole modality is: create elements, then drag-to-connect their connectors.
   A run flows work along the wires; checkpoints (gates / verification) decide
   whether work proceeds, and where it goes on success vs failure.

   ELEMENTS
   • Mission — the ENTRY POINT. Carries an objective (free text) and a single
     OUTPUT connector. Starting a mission kicks off the whole downstream chain.
     A mission "wraps" its chain (every node reachable from it); the UI surfaces
     that chain as a panel but does not otherwise nest the nodes.
       → backend: a run is launched against a mission; the mission's objective is
         the seed input handed to the first step. The engine must resolve the
         reachable sub-graph and execute it (see RUN MODEL).
   • Formation — a coordination unit. type ∈ {solo, peer, flow, orchestrated}
     (this only changes how its agent slots are arranged/▸ how work is divided).
     Holds: ordered agent SLOTS, a manual BRIEF (goal · bead · file/link refs),
     an optional in-line VERIFICATION, and N INPUT and N OUTPUT connectors.
       → backend: the type is a hint about execution style (one agent · peers
         synthesising · sequential pipeline · a controller delegating). Slots map
         to live agent sessions. The brief + arriving inputs form the task.
   • Slot — a role inside a formation; may reference one agent. In orchestrated
     formations exactly one slot is the controller. Slots are added/removed live.
     The same agent may sit in MANY slots/formations (placement is a reference,
     not ownership / not exclusive).
   • Gate — a first-class checkpoint node BETWEEN formations. Has one input, a
     PASS output and a FAIL output, and an optional JUDGE. Its check is any
     combination of kinds: Code (programmatic: build/test/lint/types), Human (a
     person reviews), Formation (a JUDGE formation runs and its result decides).
       → backend: on reaching a gate, evaluate the criterion via the chosen
         kind(s) and emit a verdict. PASS routes work down the pass wire(s); FAIL
         routes down the fail wire(s). An UNWIRED fail output = block (stop). A
         fail wire pointing back to an earlier node = a pushback/revise loop
         (the engine must tolerate such cycles via the wire, while the auto-route
         graph itself stays acyclic).
   • Verification — an in-formation check (same kinds as a gate, minus its own
     wiring). Runs at the END of a formation's work; on fail it either blocks or
     pushes back to that formation's own agents with feedback.
   • Connection — a directed edge from a specific OUTPUT port of one node to a
     specific INPUT port of another. Ports are addressable: a formation may have
     several inputs (a JOIN) and several outputs (a fan-out / distinct results);
     a gate's outputs are pass/fail. Users may also hand-route a wire (its lane
     is a presentation detail, not data).
       → backend: persist edges as (fromNode, fromPort) → (toNode, toPort). The
         set of edges is the execution graph; detect cycles, expose a topological
         order for cascade.

   RUN MODEL (async, streaming — all mocked here as setTimeout theatre)
   • A run starts from a mission (or any single node, for testing) and CASCADES:
     when a node finishes, its output flows along each outgoing wire.
   • JOIN / multi-input: a formation with 2+ incoming connections MUST NOT start
     until EVERY input has arrived. The UI shows per-input "waiting/ready" and a
     "waiting · N/M inputs" state. → backend: a node is runnable only when all of
     its in-edges have delivered.
   • A node's OUTPUT is produced BY a run (never authored): status ∈ {idle,
     running, done, needs-review, blocked}, a human-readable report, artifacts
     [{name,type,ref}], diffs [{file, unified patch}], producing agents, timing.
   • Gates and verification produce a VERDICT (pass/fail) that steers routing.
   • Everything is live: the engine should stream per-step/per-slot events
     (state, logs, terminal) and support start / cancel; downstream steps run as
     their inputs become ready.

   WHAT THE BACKEND MUST PROVIDE (integration points — all mocked here)
   • Agent/session source (e.g. tmux socket): the live roster + each agent's
     state (attached/idle/busy/dead). Agents are LIVE, discovered, not authored.
   • A terminal stream per agent (e.g. ttyd / websocket) for the popup terminals.
   • Bead resolution: read a work-item by id, attach as input, write results back.
   • A run engine: launch a mission/node, stream events, finalise each Output,
     evaluate gates/verifications, honour pass/fail routing + join readiness,
     cancel.
   • An artifact / diff store for real reports, PRs, files, patches.

   PERSISTENCE / OUT OF SCOPE
   • Persist the GRAPH: missions, formations (+slots, briefs, verification),
     gates (+judge), and all port-addressed connections, plus canvas positions.
     Agents are live (from the session source), not persisted.
   • Identity/auth and multi-user concurrency (who may view/edit/run, conflict
     handling) are OUT OF SCOPE in this prototype and must be designed for real.

   ----------------------------------------------------------------------------
   MOCK INVENTORY — replace every item (search "MOCK"):
     #1 AV / AGENTS ...... fake roster + portrait art  → live session source
     #2 feed() ........... canned terminal lines        → real ttyd/ws stream
     #3 seed() ........... demo graph + objective        → empty or loaded state
     #4 mockReport() ..... fabricated report/diffs       → run-engine output
     #5 run timings ...... setTimeout theatre            → real async run events
     #6 verdicts ......... hard-coded gate/verify pass   → real check results
   ============================================================================ */

const svgns = 'http://www.w3.org/2000/svg';

/* ▼▼ MOCK #1 — avatar art (dieselpunk portraits used as stand-in agent faces) ▼▼ */
const AV={fox:'../assets/bg_fox.png',wolf:'../assets/bg_wolf.png',hawk:'../assets/bg_hawk.png',badger:'../assets/bg_badger.png',crew:'../assets/bg_crew.png',convoy:'../assets/bg_convoy.png',polecat:'../assets/bg_polecat.png'};

/* ===== AGENTS =====  ▼▼ MOCK #1 — replace with the live tmux-socket session list ▼▼ */
const AGENTS=[
  {id:'conductor',  role:'Conductor', av:AV.fox,     state:'on'},
  {id:'codex',      role:'Codex',     av:AV.convoy,  state:'on'},
  {id:'claude-code',role:'Claude Code',av:AV.wolf,   state:'on'},
  {id:'scout',      role:'Scout',     av:AV.polecat, state:'on'},
  {id:'mason',      role:'Builder',   av:AV.badger,  state:'idle'},
  {id:'witness',    role:'Witness',   av:AV.hawk,    state:'on'},
  {id:'refiner',    role:'Refiner',   av:AV.crew,    state:'on'},
  {id:'pico',       role:'Worker',    av:AV.polecat, state:'idle'},
];
const agent=id=>AGENTS.find(a=>a.id===id);

/* ===== FORMATION TYPES ===== */
const TYPES={
  solo:{name:'Solo', tag:'Do the thing.', color:'#737373'},
  peer:{name:'Peer', tag:'Work together · challenge · synthesize.', color:'#4ade80'},
  flow:{name:'Flow', tag:'A, then B, then C.', color:'#6e87a8'},
  orchestrated:{name:'Orchestrated', tag:'One controller decides what happens next.', color:'#6b9fff'},
};

/* ===== STATE + IDS ===== */
let sid=0, fid=0, gid=0;
const newSid=()=>'s'+(++sid), newFid=()=>'f'+(++fid), newGid=()=>'g'+(++gid);
let formations=[];     // [{id,type,title,x,y,input,output,verification,slots[]}]
let connections=[];    // [{from:nodeId,to:nodeId}]
let missions=[];       // [{id,x,y,title,goal}] — entry points that start a run
let gateNodes=[];      // [{id,x,y,kinds[],onFail,criterion,verdict}]

/* ===== GATES & VERIFICATION (shared shape) ===== */
const GATE_KINDS={
  code:{name:'Code',  ic:'<path d="M9 8l-4 4 4 4M15 8l4 4-4 4"/>'},
  human:{name:'Human', ic:'<circle cx="12" cy="8" r="3.4"/><path d="M5 20c0-3.6 3.1-6 7-6s7 2.4 7 6"/>'},
  formation:{name:'Formation', ic:'<rect x="3" y="5" width="8" height="6" rx="1.3"/><rect x="13" y="13" width="8" height="6" rx="1.3"/><path d="M11 8h3a3 3 0 013 3v2"/>'},
};
/* portcullis = gate · shield-check = verification */
const GATE_SVG  ='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 21V10a8 8 0 0116 0v11"/><path d="M3 21h18M8 21V9M12 21V8M16 21V9"/></svg>';
const VERIFY_SVG='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><path d="M12 3l7 3v5c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6z"/><path d="M9 12l2 2 4-4"/></svg>';

function gateData(kinds){return {kinds:(kinds&&kinds.length?kinds.slice():['code']),onFail:'block',criterion:'work is accepted before it proceeds',verdict:'pass'};}
function makeGateNode(kinds,x,y){return Object.assign({id:newGid(),x,y},gateData(kinds));}
function makeVerification(kinds){return Object.assign({id:newGid()},gateData(kinds));}
/* ---- JUDGE = real, reconnectable connections through the gate's 'judge' port ----
   A gate's judge socket is BOTH a send-origin (gate→entry) and a return-target
   (exit→gate). Between them can sit a CHAIN of formations wired normally, so a
   judge can be one formation (a loop) or several in sequence. */
function isJudgeConn(c){return c&&(c.fromPort==='judge'||c.toPort==='judge');}
function judgeSend(g){return connections.find(c=>c.from===g.id&&c.fromPort==='judge');}   // gate → entry
function judgeReturn(g){return connections.find(c=>c.to===g.id&&c.toPort==='judge');}      // exit → gate
function judgeEntry(g){const s=judgeSend(g);return s?nodeById(s.to):null;}
function gateHasJudge(g){return !!(judgeSend(g)||judgeReturn(g));}
/* a 'formation' check shows the judge entry's name when one is connected */
function gateKindNames(g){
  return g.kinds.map(k=>{
    if(k==='formation'){const jf=judgeEntry(g);return jf?('↻ '+nodeTitle(jf)):'Formation';}
    return GATE_KINDS[k]?GATE_KINDS[k].name:k;
  }).join(' · ')||'Gate';
}
function gateLabel(g){return gateKindNames(g);}
function clearJudgeConns(g){connections=connections.filter(c=>!(isJudgeConn(c)&&(c.from===g.id||c.to===g.id)));}
/* attach a single-formation judge (the classic loop): gate → f.in, f.out → gate */
function attachJudge(g,fid){
  clearJudgeConns(g); const f=nodeById(fid); if(!f)return;
  connections.push({from:g.id,fromPort:'judge',to:fid,toPort:defIn(fid),judge:true});
  connections.push({from:fid,fromPort:defOut(fid),to:g.id,toPort:'judge',judge:true});
  syncJudgeKind(g);
}
function detachJudge(g){clearJudgeConns(g);syncJudgeKind(g);}
/* set (or replace) the RETURN exit of a gate's judge to a formation output;
   if no entry exists yet, also create the send so the loop is complete */
function setJudgeReturn(g,fromId,fromPort){
  connections=connections.filter(c=>!(c.to===g.id&&c.toPort==='judge'));
  if(!judgeSend(g)) connections.push({from:g.id,fromPort:'judge',to:fromId,toPort:defIn(fromId),judge:true});
  connections.push({from:fromId,fromPort:fromPort||defOut(fromId),to:g.id,toPort:'judge',judge:true});
  syncJudgeKind(g);
}
/* keep the 'formation' check kind in sync with whether a judge is wired */
function syncJudgeKind(g){
  const has=gateHasJudge(g), i=g.kinds.indexOf('formation');
  if(has&&i<0)g.kinds.push('formation');
  if(!has&&i>=0){g.kinds.splice(i,1);if(!g.kinds.length)g.kinds.push('code');}
}
function gateBadge(g){return g.onFail==='pushback'
  ?'<span class="gate-badge pb vbadge" title="on fail: push back">↩</span>'
  :'<span class="gate-badge bl vbadge" title="on fail: block">⊘</span>';}
function stateClass(g){return g&&g._state?(' '+g._state):'';}

function makeFormation(type,title,slotDefs,x,y){
  return {id:newFid(),type,title:title||TYPES[type].name,x,y,
    input:{goal:'',files:[],beadId:null}, output:null, verification:null,
    inputs:[{id:newSid(),label:'Input'}], outputs:[{id:newSid(),label:'Output'}],
    slots:slotDefs.map(d=>({id:newSid(),label:d.label,ctrl:!!d.ctrl,agentId:d.agentId||null}))};
}
/* default first input / output port id for a node (gates use fixed names) */
function defIn(id){const n=nodeById(id);return n?(isGate(n)?'in':(n.inputs&&n.inputs[0]?n.inputs[0].id:'in')):'in';}
function defOut(id){const n=nodeById(id);return n?(isGate(n)?'pass':isMission(n)?'out':(n.outputs&&n.outputs[0]?n.outputs[0].id:'out')):'out';}

/* ===== NODE HELPERS (formation OR gate) ===== */
const isGate=n=>!!n&&gateNodes.indexOf(n)>=0;
const isMission=n=>!!n&&missions.indexOf(n)>=0;
function makeMission(title,x,y){return {id:newGid(),x,y,title:title||'New mission',goal:''};}
function nodeById(id){return formations.find(n=>n.id===id)||gateNodes.find(n=>n.id===id)||missions.find(n=>n.id===id)||null;}
function nodeTitle(n){return n?(isGate(n)?gateKindNames(n):n.title):'';}
function nodeCardEl(id){return boardInner.querySelector('[data-node="'+id+'"]');}
function deleteNode(id){
  formations=formations.filter(n=>n.id!==id);
  gateNodes=gateNodes.filter(n=>n.id!==id);
  missions=missions.filter(n=>n.id!==id);
  connections=connections.filter(c=>c.from!==id&&c.to!==id);
}

/* ===== UNDO (basic) ===== */
let undoStack=[];
function snapState(){return JSON.stringify({formations,connections,gateNodes,missions,sid,fid,gid});}
function pushUndo(){undoStack.push(snapState());if(undoStack.length>60)undoStack.shift();}
function doUndo(){
  if(!undoStack.length)return;
  const s=JSON.parse(undoStack.pop());
  formations=s.formations; connections=s.connections; gateNodes=s.gateNodes; missions=s.missions||[];
  sid=s.sid; fid=s.fid; gid=s.gid;
  running.clear(); closePop(); closeMenu();
  rerender();
}
window.addEventListener('keydown',e=>{
  if((e.ctrlKey||e.metaKey)&&!e.shiftKey&&(e.key==='z'||e.key==='Z')){
    const t=e.target; if(t&&(t.tagName==='TEXTAREA'||t.tagName==='INPUT'))return;
    e.preventDefault(); doUndo();
  }
});

/* ===== SEED =====  ▼▼ MOCK #3 — demo formations + gate + wiring + goal ▼▼ */
function seed(){
  const mission = makeMission('Improve session search',150,228);
  const frame   = makeFormation('solo','Frame the goal',[{label:'Agent'}],470,150);
  const research= makeFormation('peer','Research huddle',[{label:'Peer'},{label:'Peer'}],840,135);
  const ship    = makeFormation('flow','Ship a change',[{label:'Plan'},{label:'Execute'},{label:'Push'}],1700,150);
  const triage  = makeFormation('orchestrated','Triage desk',[{label:'Orchestrator',ctrl:true},{label:'Agent'},{label:'Agent'},{label:'Agent'}],470,560);
  formations=[frame,research,ship,triage];
  missions=[mission];

  mission.goal='Make session search fuzzy and keyboard-first';
  frame.input.beadId='bd-204';

  assignDirect('conductor', frame.id, frame.slots[0].id);
  assignDirect('codex', research.id, research.slots[0].id);
  assignDirect('claude-code', research.id, research.slots[1].id);
  assignDirect('scout', ship.id, ship.slots[0].id);
  assignDirect('refiner', ship.id, ship.slots[1].id);
  assignDirect('mason', ship.id, ship.slots[2].id);
  assignDirect('conductor', triage.id, triage.slots[0].id);
  assignDirect('witness', triage.id, triage.slots[1].id);

  // a standalone GATE node between research and ship — its PASS output feeds ship
  const gate=makeGateNode(['human','code'], 1334, 262);
  gate.criterion='research is sound and the plan is safe to build';
  gateNodes=[gate];

  connections=[
    {from:mission.id, fromPort:'out', to:frame.id, toPort:frame.inputs[0].id},
    {from:frame.id, fromPort:frame.outputs[0].id, to:research.id, toPort:research.inputs[0].id},
    {from:research.id, fromPort:research.outputs[0].id, to:gate.id, toPort:'in'},
    {from:gate.id, fromPort:'pass', to:ship.id, toPort:ship.inputs[0].id},
  ];

  // a VERIFICATION inside the peer huddle
  research.verification=makeVerification(['code']);
  research.verification.criterion='both reads converge on a recommendation';
}

/* ===== ROSTER ===== */
function deployedSet(){const s=new Set();formations.forEach(f=>f.slots.forEach(sl=>{if(sl.agentId)s.add(sl.agentId);}));return s;}
function renderRoster(){
  const dep=deployedSet();
  const list=document.getElementById('rosterList');list.innerHTML='';
  AGENTS.forEach(a=>{
    const el=document.createElement('div');el.className='ragent'+(dep.has(a.id)?' deployed':'');el.dataset.agent=a.id;
    el.innerHTML='<img class="av" src="'+a.av+'"/><div class="ri"><div class="n">'+a.id+'</div><div class="r">'+a.role+'</div></div><span class="sd '+a.state+'"></span>';
    el.addEventListener('pointerdown',e=>beginPointer(e,a.id,null));
    el.addEventListener('contextmenu',e=>{e.preventDefault();menuAgent(e,a.id,null);});
    list.appendChild(el);
  });
  document.getElementById('rosterSub').textContent=AGENTS.length+' agents · '+dep.size+' deployed';
}

/* ===== BOARD: pan + zoom ===== */
const viewport=document.getElementById('viewport');
const world=document.getElementById('world');
const boardInner=world;
const wiresSVG=document.getElementById('wires');

let scale=1, tx=40, ty=40;
const MINS=0.3, MAXS=1.9;
function applyView(){world.style.transform='translate('+tx+'px,'+ty+'px) scale('+scale+')';const zl=document.getElementById('zoomLevel');if(zl)zl.textContent=Math.round(scale*100)+'%';}
function screenToWorld(cx,cy){const r=viewport.getBoundingClientRect();return {x:(cx-r.left-tx)/scale,y:(cy-r.top-ty)/scale};}
function zoomAround(px,py,ns){ns=Math.max(MINS,Math.min(MAXS,ns));const r=viewport.getBoundingClientRect();const wx=(px-r.left-tx)/scale,wy=(py-r.top-ty)/scale;scale=ns;tx=(px-r.left)-wx*scale;ty=(py-r.top)-wy*scale;applyView();}
function fitView(){
  const cards=[...world.querySelectorAll('.formation,.gatecard,.missioncard')];
  if(!cards.length){scale=1;tx=40;ty=40;applyView();return;}
  let minX=1e9,minY=1e9,maxX=-1e9,maxY=-1e9;
  cards.forEach(c=>{const n=nodeById(c.dataset.node);if(!n)return;minX=Math.min(minX,n.x);minY=Math.min(minY,n.y);maxX=Math.max(maxX,n.x+c.offsetWidth);maxY=Math.max(maxY,n.y+c.offsetHeight);});
  const vr=viewport.getBoundingClientRect();const pad=56;
  scale=Math.max(0.3,Math.min(1.1, Math.min(vr.width/(maxX-minX+pad*2), vr.height/(maxY-minY+pad*2))));
  tx=(vr.width-(maxX-minX)*scale)/2 - minX*scale;
  ty=(vr.height-(maxY-minY)*scale)/2 - minY*scale;
  world.classList.add('smooth');applyView();setTimeout(()=>world.classList.remove('smooth'),380);
}
let panning=false,panSX,panSY,panTX,panTY;
viewport.addEventListener('pointerdown',e=>{
  if(e.button!==0)return;
  if(e.target.closest('.formation,.gatecard,.missioncard,.term,.ctxmenu,.ghost'))return;
  panning=true;panSX=e.clientX;panSY=e.clientY;panTX=tx;panTY=ty;viewport.classList.add('panning');
});
window.addEventListener('pointermove',e=>{if(!panning)return;tx=panTX+(e.clientX-panSX);ty=panTY+(e.clientY-panSY);applyView();});
window.addEventListener('pointerup',()=>{if(panning){panning=false;viewport.classList.remove('panning');}});
viewport.addEventListener('wheel',e=>{e.preventDefault();zoomAround(e.clientX,e.clientY,scale*(e.deltaY<0?1.12:0.892));},{passive:false});
document.getElementById('zoomIn').addEventListener('click',()=>{const r=viewport.getBoundingClientRect();zoomAround(r.left+r.width/2,r.top+r.height/2,scale*1.2);});
document.getElementById('zoomOut').addEventListener('click',()=>{const r=viewport.getBoundingClientRect();zoomAround(r.left+r.width/2,r.top+r.height/2,scale/1.2);});
document.getElementById('zoomFit').addEventListener('click',fitView);

function findSlot(fId,sId){const f=formations.find(x=>x.id===fId);return f&&f.slots.find(s=>s.id===sId);}

/* ===== SLOT + BODY MARKUP ===== */
function slotHTML(f,sl,extra){
  const a=sl.agentId?agent(sl.agentId):null;
  const inner=a?'<img src="'+a.av+'"/>':'<span class="plus">+</span>';
  const who=a?'<div class="who">'+a.id+'</div>':'';
  const badge=extra&&extra.step?'<span class="badge">'+extra.step+'</span>':'';
  return '<div class="slot '+(a?'filled':'empty')+' '+(sl.ctrl?'ctrl':'')+'" data-fid="'+f.id+'" data-sid="'+sl.id+'">'+
      '<div class="slot-ring">'+badge+inner+'</div>'+
      '<div class="slot-label">'+sl.label+'</div>'+who+
    '</div>';
}
function arrowSVG(){return '<div class="flow-arrow"><svg viewBox="0 0 26 12" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M0 6h22"/><path d="M19 2l4 4-4 4"/></svg></div>';}

function bodyHTML(f){
  if(f.type==='solo'){
    return '<div class="solo-body">'+slotHTML(f,f.slots[0])+'</div>';
  }
  if(f.type==='peer'){
    return '<div class="huddle"><span class="hl">peers · no hierarchy</span><div class="peers-row">'+f.slots.map(s=>slotHTML(f,s)).join('')+'</div></div>';
  }
  if(f.type==='flow'){
    let h='<div class="flow-row">';
    f.slots.forEach((s,i)=>{h+=slotHTML(f,s,{step:i+1});if(i<f.slots.length-1)h+=arrowSVG();});
    h+='</div>';return h;
  }
  if(f.type==='orchestrated'){
    const ctrl=f.slots.find(s=>s.ctrl)||f.slots[0];
    const workers=f.slots.filter(s=>s!==ctrl);
    return '<div class="orch">'+
      '<div class="ctrl-wrap">'+slotHTML(f,ctrl)+'</div>'+
      '<div class="pool"><span class="pl">open slots</span>'+workers.map(s=>slotHTML(f,s)).join('')+'</div>'+
    '</div>';
  }
  return '';
}
function verifyBandHTML(f){
  const v=f.verification;
  if(!v) return '<div class="verify-band empty"><span class="vico">'+VERIFY_SVG+'</span><span class="vlabel">+ verify</span></div>';
  return '<div class="verify-band'+stateClass(v)+'" data-gate="'+v.id+'"><span class="vico">'+VERIFY_SVG+'</span><span class="vlabel">verify</span><span class="vkinds">'+gateKindNames(v)+' · '+v.criterion+'</span>'+'</div>';
}

/* ===== RENDER: FORMATIONS ===== */
function renderFormations(){
  [...boardInner.querySelectorAll('.formation')].forEach(c=>c.remove());
  formations.forEach(f=>{
    const card=document.createElement('div');card.className='formation type-'+f.type;
    card.style.left=f.x+'px';card.style.top=f.y+'px';card.dataset.fid=f.id;card.dataset.node=f.id;
    card.setAttribute('data-screen-label',f.title);
    const hasManual = f.input.goal||f.input.beadId||f.input.files.length;
    // INPUT rows — one per input slot, each with its own connector
    let inRows='';
    f.inputs.forEach((slot,idx)=>{
      const conns=connections.filter(c=>c.to===f.id && (c.toPort||defIn(f.id))===slot.id && !isJudgeConn(c));
      let content='';
      conns.forEach(c=>{const src=nodeById(c.from);const ok=inputArrived(c);content+='<span class="io-text">← '+nodeTitle(src)+'</span><span class="io-status '+(ok?'done':'idle')+'">'+(ok?'ready':'waiting')+'</span>';});
      if(!conns.length){ content = (idx===0)?briefLabelHTML(f):'<span class="io-text placeholder">'+slot.label+' — wire an input…</span>'; }
      inRows+='<div class="fio in'+(idx===0?' brief':'')+'" data-islot="'+slot.id+'"><span class="port pin '+((conns.length||(idx===0&&hasManual))?'has':'')+'" data-port="'+slot.id+'" data-pk="in"></span><span class="glyph">in</span>'+content+'</div>';
    });
    // OUTPUT rows — one per output slot, each with its own connector
    let outRows='';
    f.outputs.forEach((slot,idx)=>{
      const body = (idx===0)?outLabelHTML(f):'<span class="io-status '+(f.output?'done':'idle')+'">'+(f.output?'ready':'—')+'</span><span class="io-text">'+slot.label.toLowerCase()+'</span>';
      outRows+='<div class="fio out" data-oslot="'+slot.id+'"><span class="glyph">out</span>'+body+'<span class="port pout '+(f.output?'ready':'empty')+'" data-port="'+slot.id+'" data-pk="out"></span></div>';
    });
    card.innerHTML=
      inRows+
      '<div class="fhead">'+
        '<div class="ft"><div class="tt">'+f.title+'</div><div class="tg">'+TYPES[f.type].tag+'</div></div>'+
        '<button class="frun" title="Run formation"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 4l14 8-14 8z"/></svg></button>'+
      '</div>'+
      '<div class="fstatus" data-status></div>'+
      '<div class="fbody">'+bodyHTML(f)+'</div>'+
      verifyBandHTML(f)+
      outRows;
    dragCard(card,f);
    // EVERYTHING is right-clickable: the card is a catch-all (body whitespace, gaps),
    // with element-specific menus layered on top (they stopPropagation).
    card.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuFormation(e,f);});
    card.querySelector('.fhead').addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuFormation(e,f);});
    card.querySelector('.frun').addEventListener('click',e=>{e.stopPropagation();runFormation(f,card);});
    const briefRow=card.querySelector('.fio.in.brief'); if(briefRow)briefRow.addEventListener('click',e=>{e.stopPropagation();openInputEditor(f);});
    card.querySelectorAll('.fio.in').forEach(row=>row.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuInputRow(e,f);}));
    card.querySelectorAll('.fio.out').forEach(row=>row.addEventListener('click',e=>{e.stopPropagation();f.output?openReport(f):runFormation(f,card);}));
    card.querySelectorAll('.fio.out').forEach(row=>row.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuOutputRow(e,f);}));
    card.querySelectorAll('.port.pin').forEach(pt=>pt.addEventListener('pointerdown',e=>startWire(e,f,pt.dataset.port,false)));
    card.querySelectorAll('.port.pout').forEach(pt=>pt.addEventListener('pointerdown',e=>startWire(e,f,pt.dataset.port,true)));
    const vb=card.querySelector('.verify-band');
    if(vb){
      vb.addEventListener('click',e=>{
        e.stopPropagation();
        if(!f.verification){pushUndo();f.verification=makeVerification(['code']);rerender();openGateConfig(f.verification,{type:'verify',f});}
        else openGateConfig(f.verification,{type:'verify',f});
      });
      vb.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuVerification(e,f);});
    }
    card.querySelectorAll('.slot').forEach(slotEl=>{
      const sl=findSlot(slotEl.dataset.fid,slotEl.dataset.sid);
      slotEl.addEventListener('pointerdown',e=>{ if(sl.agentId){ beginPointer(e,sl.agentId,{fid:f.id,sid:sl.id}); } });
      slotEl.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuSlot(e,f,sl);});
    });
    boardInner.appendChild(card);
  });
}
function briefLabelHTML(f){
  if(f.input.goal) return '<span class="io-text">'+f.input.goal+'</span>'+(f.input.beadId?'<span class="io-bead">'+f.input.beadId+'</span>':'');
  if(f.input.beadId) return '<span class="io-text">bead</span><span class="io-bead">'+f.input.beadId+'</span>';
  if(f.input.files.length) return '<span class="io-text">'+f.input.files.length+' file'+(f.input.files.length>1?'s':'')+' attached</span>';
  return '<span class="io-text placeholder">set a goal or input…</span>';
}
/* an input has "arrived" when its source formation has produced output (or its feeding gate passed) */
function inputArrived(c){const s=nodeById(c.from);if(!s)return false;return isGate(s)?(s._state==='pass'):!!s.output;}
function outLabelHTML(f){
  if(!f.output) return '<span class="io-status idle">no output yet</span>';
  const cls=f.output.status==='done'?'done':f.output.status==='blocked'?'blocked':'review';
  const label=f.output.status==='done'?'done':f.output.status==='blocked'?'blocked':'needs review';
  return '<span class="io-status '+cls+'">'+label+'</span><span class="io-text">'+f.output.summary+'</span>';
}

/* ===== RENDER: GATE NODES ===== */
const CHECK_SVG='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M4 12l5 5L20 6"/></svg>';
const XMARK_SVG='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M6 6l12 12M18 6L6 18"/></svg>';
function renderGates(){
  [...boardInner.querySelectorAll('.gatecard')].forEach(c=>c.remove());
  gateNodes.forEach(g=>{
    const card=document.createElement('div');card.className='gatecard'+stateClass(g)+(gateHasJudge(g)?' hasjudge':'');
    card.dataset.node=g.id;card.dataset.gate=g.id;card.style.left=g.x+'px';card.style.top=g.y+'px';
    card.setAttribute('data-screen-label','Gate · '+gateLabel(g));
    const incoming=connections.some(c=>c.to===g.id);
    card.innerHTML=
      '<span class="port pin'+(incoming?' has':'')+'" data-port="in" data-pk="in" title="Work to check"></span>'+
      '<button class="pjudge" data-port="judge" data-pk="judge" title="Judge with a formation — it runs and its result decides pass / fail"></button>'+
      '<span class="gico">'+GATE_SVG+'</span>'+
      '<span class="gmeta"><span class="gt">'+gateLabel(g)+'</span><span class="gs">'+g.criterion+'</span></span>'+
      '<span class="glabel pass">'+CHECK_SVG+'pass</span>'+
      '<span class="glabel fail">'+XMARK_SVG+'fail</span>'+
      '<span class="port pass" data-port="pass" data-pk="out" title="On PASS → drag to the next step"></span>'+
      '<span class="port fail" data-port="fail" data-pk="out" title="On FAIL → drag to a fallback or back a step"></span>';
    card.querySelector('.port.pin').addEventListener('pointerdown',e=>startWire(e,g,'in',false));
    card.querySelector('.port.pass').addEventListener('pointerdown',e=>startWire(e,g,'pass',true));
    card.querySelector('.port.fail').addEventListener('pointerdown',e=>startWire(e,g,'fail',true));
    card.querySelector('.pjudge').addEventListener('pointerdown',e=>startJudgeWire(e,g));
    card.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuGate(e,g);});
    dragGate(card,g);
    boardInner.appendChild(card);
  });
}

/* ===== RENDER: MISSIONS (entry points) ===== */
function renderMissions(){
  [...boardInner.querySelectorAll('.missioncard')].forEach(c=>c.remove());
  missions.forEach(m=>{
    const card=document.createElement('div');card.className='missioncard';card.dataset.node=m.id;
    card.style.left=m.x+'px';card.style.top=m.y+'px';card.setAttribute('data-screen-label','Mission · '+m.title);
    card.innerHTML=
      '<div class="mhd"><span class="meyebrow">◆ Mission</span><button class="mrun" title="Start mission"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 4l14 8-14 8z"/></svg></button></div>'+
      '<div class="mtitle">'+m.title+'</div>'+
      '<div class="mgoal'+(m.goal?'':' placeholder')+'">'+(m.goal||'set the mission objective…')+'</div>'+
      '<div class="mstatus" data-status></div>'+
      '<span class="port pout" data-port="out" data-pk="out" title="Starts the chain"></span>';
    card.querySelector('.mrun').addEventListener('click',e=>{e.stopPropagation();runMission(m);});
    card.querySelector('.port.pout').addEventListener('pointerdown',e=>startWire(e,m,'out',true));
    card.addEventListener('contextmenu',e=>{e.preventDefault();e.stopPropagation();menuMission(e,m);});
    dragMission(card,m);
    boardInner.appendChild(card);
  });
}
function dragMission(card,m){
  card.addEventListener('pointerdown',e=>{
    if(e.button!==0)return;
    if(e.target.closest('.port,.mrun'))return;
    const sx=e.clientX,sy=e.clientY,ox=m.x,oy=m.y;let moved=false,pushed=false;
    const move=ev=>{ if(!moved&&Math.abs(ev.clientX-sx)+Math.abs(ev.clientY-sy)<3)return; if(!pushed){pushUndo();pushed=true;draggingNodeId=m.id;} moved=true;
      m.x=ox+(ev.clientX-sx)/scale;m.y=oy+(ev.clientY-sy)/scale;card.style.left=m.x+'px';card.style.top=m.y+'px';drawWires(); };
    const up=()=>{draggingNodeId=null;window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up); if(!moved)openMissionPanel(m); };
    window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
  });
}
function menuMission(e,m){
  showMenu(e.clientX,e.clientY,[{head:'Mission'},
    {label:'Start mission',icon:IC.run,fn:()=>runMission(m)},
    {label:'Open panel…',icon:IC.assign,fn:()=>openMissionPanel(m)},
    {label:'Rename…',icon:IC.rename,fn:()=>{const n=prompt('Mission name',m.title);if(n&&n.trim()){pushUndo();m.title=n.trim();rerender();}}},
    {div:true},
    {label:'Delete mission',icon:IC.trash,danger:true,fn:()=>{pushUndo();deleteNode(m.id);rerender();}},
  ]);
}
/* the chain a mission wraps = every node reachable from it via connections (topological-ish order) */
function missionChain(m){
  const seen=new Set([m.id]); const order=[]; let frontier=[m.id]; let guard=0;
  while(frontier.length && guard++<400){
    const next=[];
    connections.forEach(c=>{ if(frontier.indexOf(c.from)>=0 && !seen.has(c.to)){ seen.add(c.to); const n=nodeById(c.to); if(n){order.push(n);next.push(c.to);} } });
    frontier=next;
  }
  return order;
}
function stepStatus(n){
  if(isGate(n)) return n._state||'idle';
  if(n.output) return n.output.status||'done';
  return 'idle';
}
function openMissionPanel(m){
  closePop();
  const chain=missionChain(m);
  const rows=chain.length?chain.map(n=>{
    const kind=isGate(n)?'gate':(n.type||'');
    return '<div class="mp-row"><span class="mp-dot '+stepStatus(n)+'"></span><span class="mp-t">'+nodeTitle(n)+'</span><span class="mp-k">'+kind+'</span></div>';
  }).join(''):'<div class="mp-empty">No steps yet — drag the mission’s output connector into a formation or gate to build the chain.</div>';
  const p=document.createElement('div');p.className='pop';p.style.width='370px';p.style.left='calc(50% - 185px)';p.style.top='96px';
  p.innerHTML='<div class="pop-head"><span class="pt">Mission</span><button class="x">×</button></div>'+
    '<div class="pop-body">'+
      '<label>Name</label><input class="f" id="m-title" value="'+m.title.replace(/"/g,'&quot;')+'"/>'+
      '<label>Objective</label><textarea id="m-goal" placeholder="What should this mission achieve?">'+(m.goal||'')+'</textarea>'+
      '<label>Steps in this mission ('+chain.length+')</label>'+
      '<div class="mp-list">'+rows+'</div>'+
      '<button class="save" id="m-run">▶ Start mission</button>'+
    '</div>';
  document.body.appendChild(p);currentPop=p;dragPop(p);
  p.querySelector('.x').addEventListener('click',closePop);
  const tt=p.querySelector('#m-title'); tt.addEventListener('focus',()=>{if(!tt._u){pushUndo();tt._u=true;}}); tt.addEventListener('input',()=>{m.title=tt.value;}); tt.addEventListener('blur',()=>rerender());
  const gg=p.querySelector('#m-goal'); gg.addEventListener('focus',()=>{if(!gg._u){pushUndo();gg._u=true;}}); gg.addEventListener('input',()=>{m.goal=gg.value;}); gg.addEventListener('blur',()=>rerender());
  p.querySelector('#m-run').addEventListener('click',()=>{closePop();runMission(m);});
}

/* ===== PORTS / WIRES ===== */
function within(ev,r,m){return ev.clientX>r.left-m&&ev.clientX<r.right+m&&ev.clientY>r.top-m&&ev.clientY<r.bottom+m;}
/* true visual centre of a port (transform-safe — handles translateX(-50%) on the judge socket,
   plus the world pan/zoom). Wires sit behind the cards, so a centre anchor reads cleanly. */
function portPoint(id,role){
  const card=nodeCardEl(id);
  if(!card)return null;
  const port=card.querySelector('[data-port="'+role+'"]');
  if(!port)return null;
  const r=port.getBoundingClientRect();
  return screenToWorld(r.left+r.width/2, r.top+r.height/2);
}
function wirePath(a,b){const dx=Math.max(40,Math.abs(b.x-a.x)*0.5);return 'M'+a.x+','+a.y+' C'+(a.x+dx)+','+a.y+' '+(b.x-dx)+','+b.y+' '+b.x+','+b.y;}

/* ---- obstacle-aware routing: curve AROUND cards rather than under them ---- */
let draggingNodeId=null; // the node currently being dragged is ignored by wire routing (no "gravity")
function obstacleRects(excludeIds){
  const ex=new Set(excludeIds); if(draggingNodeId)ex.add(draggingNodeId); const out=[];
  [...formations,...gateNodes].forEach(n=>{ if(ex.has(n.id))return; const el=nodeCardEl(n.id); if(!el)return; out.push({x:n.x,y:n.y,w:el.offsetWidth,h:el.offsetHeight}); });
  return out;
}
/* Liang–Barsky: does segment a→b cross rect r (inflated by m)? */
function segHitsRect(a,b,r,m){
  const x1=r.x-m,y1=r.y-m,x2=r.x+r.w+m,y2=r.y+r.h+m;
  let t0=0,t1=1; const dx=b.x-a.x, dy=b.y-a.y;
  const p=[-dx,dx,-dy,dy], q=[a.x-x1,x2-a.x,a.y-y1,y2-a.y];
  for(let i=0;i<4;i++){
    if(p[i]===0){ if(q[i]<0)return false; }
    else { const t=q[i]/p[i];
      if(p[i]<0){ if(t>t1)return false; if(t>t0)t0=t; }
      else { if(t<t0)return false; if(t<t1)t1=t; } }
  }
  return true;
}
/* orthogonal path with lightly-rounded 90° corners */
function roundedOrtho(rawPts,r){
  const pts=[];
  rawPts.forEach(p=>{const q=pts[pts.length-1]; if(!q||Math.abs(q.x-p.x)>0.5||Math.abs(q.y-p.y)>0.5)pts.push(p);});
  if(pts.length<2)return '';
  if(pts.length===2)return 'M'+pts[0].x+','+pts[0].y+' L'+pts[1].x+','+pts[1].y;
  let d='M'+pts[0].x+','+pts[0].y;
  for(let i=1;i<pts.length-1;i++){
    const p0=pts[i-1],p1=pts[i],p2=pts[i+1];
    const inx=Math.sign(p1.x-p0.x),iny=Math.sign(p1.y-p0.y);
    const oux=Math.sign(p2.x-p1.x),ouy=Math.sign(p2.y-p1.y);
    const d1=Math.min(r,Math.hypot(p1.x-p0.x,p1.y-p0.y)/2), d2=Math.min(r,Math.hypot(p2.x-p1.x,p2.y-p1.y)/2);
    const a={x:p1.x-inx*d1,y:p1.y-iny*d1}, b={x:p1.x+oux*d2,y:p1.y+ouy*d2};
    d+=' L'+a.x+','+a.y+' Q'+p1.x+','+p1.y+' '+b.x+','+b.y;
  }
  const last=pts[pts.length-1];
  d+=' L'+last.x+','+last.y;
  return d;
}
/* pick a horizontal lane (above or below) clear of cards spanning [xL,xR] */
function clearLaneY(xL,xR,fromId,toId,preferY){
  const obs=obstacleRects([fromId,toId]).filter(r=> r.x < xR+14 && r.x+r.w > xL-14);
  if(!obs.length)return preferY;
  const above=Math.min(...obs.map(r=>r.y))-32;
  const below=Math.max(...obs.map(r=>r.y+r.h))+32;
  return (Math.abs(above-preferY)<=Math.abs(below-preferY))?above:below;
}
/* orthogonal router: out the right of the source, in the left of the target, around any cards between */
function routeOrtho(S,T,fromId,toId,via){
  const stub=24;
  const S2={x:S.x+stub,y:S.y}, T2={x:T.x-stub,y:T.y};
  // manual override: user dragged the wire to a lane
  if(via){
    return roundedOrtho([S,S2,{x:S2.x,y:via.y},{x:T2.x,y:via.y},T2,T],10);
  }
  const frozen=!!draggingNodeId; // while dragging, don't reroute around obstacles (kills wire "gravity")
  const clear=arr=>{if(frozen)return true;const o=obstacleRects([fromId,toId]);for(let i=0;i<arr.length-1;i++)for(const r of o)if(segHitsRect(arr[i],arr[i+1],r,6))return false;return true;};
  const level=Math.abs(S.y-T.y)<6;
  let pts;
  const backward = T.x < S.x-24; // only a genuinely leftward target loops via a lane
  if(!backward){
    // (near-)level and unobstructed → a clean straight line, no spurious corner kinks
    if(level && clear([S,T])) return roundedOrtho([S,T],10);
    const midX=Math.round((S2.x+T2.x)/2);
    const z=[S,S2,{x:midX,y:S2.y},{x:midX,y:T2.y},T2,T];
    if(clear(z)) pts=z;
    else { const laneY=clearLaneY(S2.x,T2.x,fromId,toId,(S.y+T.y)/2);
      pts=[S,S2,{x:S2.x,y:laneY},{x:T2.x,y:laneY},{x:T2.x,y:T2.y},T2,T]; }
  } else {
    const laneY = frozen ? (Math.min(S.y,T.y)-46) : clearLaneY(T2.x,S2.x,fromId,toId,(S.y+T.y)/2);
    pts=[S,S2,{x:S2.x,y:laneY},{x:T2.x,y:laneY},{x:T2.x,y:T2.y},T2,T];
  }
  return roundedOrtho(pts,10);
}

function drawWires(){
  [...wiresSVG.querySelectorAll('path.wire, path.wirehit')].forEach(p=>p.remove());
  // work + branch connections
  connections.forEach(c=>{
    if(c._hidden)return; // being reconnected — drawn as the temp wire instead
    const a=portPoint(c.from,c.fromPort||defOut(c.from)), b=portPoint(c.to,c.toPort||defIn(c.to));
    if(!a||!b)return;
    const judgeWire=isJudgeConn(c);
    const d=judgeWire?routeJudge(c,a,b):routeOrtho(a,b,c.from,c.to,c.via);
    const gId=judgeWire?(c.fromPort==='judge'?c.from:c.to):null;
    const gEval=gId&&nodeById(gId)&&nodeById(gId)._state==='evaluating';
    const branchCls=judgeWire?' judge':(c.fromPort==='pass'?' pass':c.fromPort==='fail'?' fail':'');
    // invisible fat hit-area makes the thin wire easy to grab/drag
    const hit=document.createElementNS(svgns,'path');
    hit.setAttribute('class','wirehit');hit.setAttribute('d',d);
    hit.addEventListener('pointerdown',ev=>startWireDrag(ev,c));
    hit.addEventListener('contextmenu',ev=>{ev.preventDefault();ev.stopPropagation();menuWire(ev,c);});
    wiresSVG.appendChild(hit);
    const p=document.createElementNS(svgns,'path');
    p.setAttribute('class','wire'+branchCls+((c.flowing||gEval)?' flowing':''));
    p.setAttribute('d',d);p.dataset.ci=connections.indexOf(c);
    p.addEventListener('contextmenu',ev=>{ev.preventDefault();ev.stopPropagation();menuWire(ev,c);});
    p.addEventListener('pointerdown',ev=>startWireDrag(ev,c));
    wiresSVG.appendChild(p);
  });
}
/* route a judge connection: it touches a gate's TOP 'judge' socket (vertical).
   Bracket just above the SOCKET (not above the whole formation) and approach the
   formation from its side, so single loops and spread-out chains both read cleanly. */
function routeJudge(c,a,b){
  if(c.fromPort==='judge'){            // SEND: gate socket (a, top) → target input (b, left side)
    const tf=nodeById(c.to), tel=nodeCardEl(c.to);
    const riseY=a.y-26;                // just above the gate socket
    const Lx=(tf&&tel&&!isGate(tf))?tf.x-30:b.x-40;
    return roundedOrtho([a,{x:a.x,y:riseY},{x:Lx,y:riseY},{x:Lx,y:b.y},b],10);
  }
  // RETURN: source output (a, right side) → gate socket (b, top)
  const sf=nodeById(c.from), sel=nodeCardEl(c.from);
  const riseY=b.y-26;
  const Rx=(sf&&sel&&!isGate(sf))?sf.x+sel.offsetWidth+30:a.x+40;
  return roundedOrtho([a,{x:Rx,y:a.y},{x:Rx,y:riseY},{x:b.x,y:riseY},b],10);
}
/* grab a committed wire: near an ENDPOINT → reconnect that end; in the MIDDLE → set its lane */
function startWireDrag(e,c){
  if(e.button!==undefined && e.button!==0)return;
  e.preventDefault(); e.stopPropagation();
  const aPt=portPoint(c.from,c.fromPort||defOut(c.from)), bPt=portPoint(c.to,c.toPort||defIn(c.to));
  const w=screenToWorld(e.clientX,e.clientY);
  if(aPt&&bPt){
    const dFrom=Math.hypot(w.x-aPt.x,w.y-aPt.y), dTo=Math.hypot(w.x-bPt.x,w.y-bPt.y);
    const near=70; // world px
    if(dTo<=dFrom && dTo<near) return startReconnect(e,c,'to');
    if(dFrom<dTo && dFrom<near) return startReconnect(e,c,'from');
  }
  // middle → hand-route the lane
  let pushed=false;
  const move=ev=>{ if(!pushed){pushUndo();pushed=true;} c.via=screenToWorld(ev.clientX,ev.clientY); drawWires(); };
  const up=()=>{ window.removeEventListener('pointermove',move); window.removeEventListener('pointerup',up); };
  window.addEventListener('pointermove',move); window.addEventListener('pointerup',up);
}
/* reconnect one END of an existing connection by dragging it onto another port.
   end='to'  → repoint the target (drop on any input / a gate's judge socket)
   end='from'→ repoint the source (drop on any output)
   drop on empty / invalid → revert (no change). Right-click a wire to delete. */
function startReconnect(e,c,end){
  e.preventDefault();e.stopPropagation();
  const fixedId   = end==='to' ? c.from : c.to;
  const fixedPort = end==='to' ? (c.fromPort||defOut(c.from)) : (c.toPort||defIn(c.to));
  const branchCls = c.fromPort==='pass'?' pass':c.fromPort==='fail'?' fail':'';
  c._hidden=true; drawWires();
  const tmp=document.createElementNS(svgns,'path');tmp.setAttribute('class','wire temp'+branchCls);wiresSVG.appendChild(tmp);
  const wantSel = end==='to' ? '[data-pk="in"],[data-pk="judge"]' : '[data-pk="out"]';
  let target=null,targetPort=null,targetPk=null,committed=false;
  const move=ev=>{
    const fp=portPoint(fixedId,fixedPort); if(!fp)return;
    const w=screenToWorld(ev.clientX,ev.clientY);
    tmp.setAttribute('d', end==='to'?wirePath(fp,w):wirePath(w,fp));
    let hit=null;
    boardInner.querySelectorAll(wantSel).forEach(pt=>{const r=pt.getBoundingClientRect();if(within(ev,r,10))hit=pt;});
    boardInner.querySelectorAll('.snaptarget').forEach(p=>p.classList.remove('snaptarget'));
    if(hit){const nid=hit.closest('[data-node]').dataset.node; if(nid!==fixedId){hit.classList.add('snaptarget');target=nid;targetPort=hit.dataset.port;targetPk=hit.dataset.pk;} else target=null;}
    else target=null;
  };
  const up=()=>{
    window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);
    tmp.remove();boardInner.querySelectorAll('.snaptarget').forEach(p=>p.classList.remove('snaptarget'));
    c._hidden=false;
    if(target){
      // dropping a wire's target end onto a gate's "+" → its source becomes the gate's judge return
      if(end==='to' && targetPk==='judge'){
        const g=gateNodes.find(x=>x.id===target);
        if(g && !isGate(nodeById(c.from))){pushUndo();connections.splice(connections.indexOf(c),1);setJudgeReturn(g,c.from,c.fromPort);rerender();committed=true;}
      } else {
        const nf = end==='to' ? {from:c.from,fromPort:c.fromPort,to:target,toPort:targetPort}
                              : {from:target,fromPort:targetPort,to:c.to,toPort:c.toPort};
        const dup=connections.some(x=>x!==c&&x.from===nf.from&&x.to===nf.to&&(x.fromPort||'')===(nf.fromPort||'')&&(x.toPort||'')===(nf.toPort||''));
        if(nf.from!==nf.to && !dup){pushUndo();Object.assign(c,nf);committed=true;}
      }
    }
    if(!committed)rerender(); else rerender();
  };
  window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
}
function startWire(e,node,portId,isOutput){
  e.preventDefault();e.stopPropagation();
  // CONNECTIONS ALWAYS GO OUTPUT → INPUT. Pressing an INPUT port doesn't start a new wire;
  // instead it picks up an existing connection landing there and lets you reconnect it.
  if(!isOutput){
    const existing=connections.filter(c=>c.to===node.id && (c.toPort||defIn(node.id))===portId);
    if(existing.length){ return startReconnect(e,existing[existing.length-1],'to'); }
    return; // empty input → nothing (drag from an output instead)
  }
  const tmp=document.createElementNS(svgns,'path');
  tmp.setAttribute('class','wire temp'+(portId==='pass'?' pass':portId==='fail'?' fail':''));wiresSVG.appendChild(tmp);
  let target=null, targetPort=null, targetPk=null;
  const start=portPoint(node.id,portId);
  const move=ev=>{
    const w=screenToWorld(ev.clientX,ev.clientY);
    tmp.setAttribute('d', wirePath(start,w));
    const wantSel='[data-pk="in"],[data-pk="judge"]';
    let hit=null;
    boardInner.querySelectorAll(wantSel).forEach(pt=>{const r=pt.getBoundingClientRect();if(within(ev,r,10))hit=pt;});
    boardInner.querySelectorAll('.snaptarget').forEach(p=>p.classList.remove('snaptarget'));
    if(hit){const nid=hit.closest('[data-node]').dataset.node; if(nid!==node.id){hit.classList.add('snaptarget');target=nid;targetPort=hit.dataset.port;targetPk=hit.dataset.pk;} else target=null;}
    else target=null;
  };
  const up=()=>{
    window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);
    tmp.remove();boardInner.querySelectorAll('.snaptarget').forEach(p=>p.classList.remove('snaptarget'));
    if(target){
      // drop a formation's output onto a gate's "+" → it becomes the gate's judge RETURN
      // (if no entry exists yet, a send is auto-created so a single formation forms the loop)
      if(targetPk==='judge'){
        const g=gateNodes.find(x=>x.id===target);
        if(g && !isGate(node)){pushUndo();setJudgeReturn(g,node.id,portId);rerender();}
        return;
      }
      const from=node.id, fromPort=portId, to=target, toPort=targetPort;
      if(from!==to && !connections.some(c=>c.from===from&&c.to===to&&(c.fromPort||'')===(fromPort||'')&&(c.toPort||'')===(toPort||''))){pushUndo();connections.push({from,fromPort,to,toPort});rerender();}
    }
  };
  window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
}
/* drag from a gate's top "+" → drop on a formation (attach) or empty canvas (create a new judge) */
function startJudgeWire(e,gate){
  e.preventDefault();e.stopPropagation();
  const tmp=document.createElementNS(svgns,'path');tmp.setAttribute('class','wire temp judge');wiresSVG.appendChild(tmp);
  const start=portPoint(gate.id,'judge');
  let target=null, moved=false;
  const sx=e.clientX,sy=e.clientY;
  const move=ev=>{
    if(!moved && Math.abs(ev.clientX-sx)+Math.abs(ev.clientY-sy)<4)return; moved=true;
    const w=screenToWorld(ev.clientX,ev.clientY);
    tmp.setAttribute('d',wirePath(w,start));
    document.querySelectorAll('.formation.judgehover').forEach(c=>c.classList.remove('judgehover'));
    let hit=null;
    boardInner.querySelectorAll('.formation').forEach(card=>{if(within(ev,card.getBoundingClientRect(),0))hit=card;});
    if(hit){hit.classList.add('judgehover');target=hit.dataset.node;} else target=null;
  };
  const up=ev=>{
    window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);
    tmp.remove();document.querySelectorAll('.formation.judgehover').forEach(c=>c.classList.remove('judgehover'));
    if(!moved){ openJudgePicker(gate); return; }
    if(target && target!==gate.id){ pushUndo(); attachJudge(gate,target); rerender(); return; }
    // dropped on empty canvas → just-works: create a new judge formation right there
    const overCard = ev.target.closest && ev.target.closest('.formation,.gatecard,.ctxmenu,.pop,.term');
    const vr=viewport.getBoundingClientRect();
    const inView = ev.clientX>vr.left&&ev.clientX<vr.right&&ev.clientY>vr.top&&ev.clientY<vr.bottom;
    if(!overCard && inView){ const w=screenToWorld(ev.clientX,ev.clientY); pushUndo();
      const nf=makeFormation('solo','Judge',[{label:'Judge'}],Math.round(w.x-100),Math.round(w.y-58));
      formations.push(nf); attachJudge(gate,nf.id); rerender(); }
  };
  window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
}
function spawnJudge(gate,nf){
  formations.push(nf); attachJudge(gate,nf.id); rerender();
  // measure the freshly rendered cards and lift the judge clear above the gate, centered
  const jel=nodeCardEl(nf.id), gel=nodeCardEl(gate.id);
  if(jel&&gel){ nf.y=gate.y-jel.offsetHeight-72; nf.x=Math.round(gate.x+gel.offsetWidth/2-jel.offsetWidth/2); rerender(); }
}
function openJudgePicker(gate){
  const card=nodeCardEl(gate.id); const r=card.getBoundingClientRect();
  const mk=(type,title,defs)=>()=>{pushUndo();spawnJudge(gate,makeFormation(type,title,defs,gate.x,gate.y-200));};
  const items=[{head:'Judge with a NEW formation'},
    {label:'Solo',dot:TYPES.solo.color,sm:'1 agent',fn:mk('solo','Judge',[{label:'Judge'}])},
    {label:'Peer',dot:TYPES.peer.color,sm:'2 equals',fn:mk('peer','Judge panel',[{label:'Peer'},{label:'Peer'}])},
    {label:'Orchestrated',dot:TYPES.orchestrated.color,sm:'controller',fn:mk('orchestrated','Judge desk',[{label:'Orchestrator',ctrl:true},{label:'Agent'}])},
  ];
  if(formations.length){
    items.push({div:true},{head:'…or an existing formation'});
    const entry=judgeEntry(gate);
    formations.forEach(f=>{ if(!entry||f.id!==entry.id) items.push({icon:IC.shield,label:f.title,sm:TYPES[f.type].name,fn:()=>{pushUndo();attachJudge(gate,f.id);rerender();}}); });
  }
  if(gateHasJudge(gate)){items.push({div:true});items.push({label:'Detach judge',icon:IC.unassign,danger:true,fn:()=>{pushUndo();detachJudge(gate);rerender();}});}
  showMenu(r.left+r.width/2, r.top-4, items);
}

/* ===== INPUT EDITOR ===== */
function openInputEditor(f){
  closePop();
  const p=document.createElement('div');p.className='pop';p.style.left='calc(50% - 180px)';p.style.top='110px';
  p.innerHTML='<div class="pop-head"><span class="pt">Input · '+f.title+'</span><button class="x">×</button></div>'+
    '<div class="pop-body">'+
      '<label>Goal / idea</label>'+
      '<textarea id="ie-goal" placeholder="Describe what this formation should achieve…">'+(f.input.goal||'')+'</textarea>'+
      '<label>Bead</label>'+
      '<input class="f" id="ie-bead" placeholder="bd-000" value="'+(f.input.beadId||'')+'"/>'+
      '<label>File links &amp; context</label>'+
      '<div class="chiprow" id="ie-files">'+f.input.files.map((fl,i)=>fileChip(fl,i)).join('')+'</div>'+
      '<div class="addrow"><input class="f" id="ie-file" placeholder="path/to/file or https://…"/><button id="ie-add">Add</button></div>'+
      '<button class="save" id="ie-save">Save input</button>'+
    '</div>';
  document.body.appendChild(p);currentPop=p;dragPop(p);
  p.querySelector('.x').addEventListener('click',closePop);
  const filesEl=p.querySelector('#ie-files');
  const addFile=()=>{const v=p.querySelector('#ie-file').value.trim();if(!v)return;f.input.files.push(v);p.querySelector('#ie-file').value='';filesEl.innerHTML=f.input.files.map((fl,i)=>fileChip(fl,i)).join('');bindChips(filesEl,f);};
  p.querySelector('#ie-add').addEventListener('click',addFile);
  p.querySelector('#ie-file').addEventListener('keydown',e=>{if(e.key==='Enter')addFile();});
  bindChips(filesEl,f);
  p.querySelector('#ie-save').addEventListener('click',()=>{
    pushUndo();
    f.input.goal=p.querySelector('#ie-goal').value.trim();
    f.input.beadId=p.querySelector('#ie-bead').value.trim()||null;
    closePop();rerender();
  });
}
function fileChip(fl,i){const isLink=/^https?:/.test(fl);return '<span class="chip-i"><b>'+(isLink?'↗':'⎘')+' '+(fl.length>30?fl.slice(0,30)+'…':fl)+'</b><span class="rm" data-i="'+i+'">×</span></span>';}
function bindChips(el,f){el.querySelectorAll('.rm').forEach(r=>r.addEventListener('click',()=>{f.input.files.splice(+r.dataset.i,1);el.innerHTML=f.input.files.map((fl,i)=>fileChip(fl,i)).join('');bindChips(el,f);}));}

/* ===== REPORT ===== */
function openReport(f){
  closePop();const o=f.output;if(!o)return;
  const cls=o.status==='done'?'done':o.status==='blocked'?'blocked':'review';
  const stTxt=o.status==='done'?'✓ done':o.status==='blocked'?'✕ blocked':'◑ needs review';
  const p=document.createElement('div');p.className='pop';p.style.width='420px';p.style.left='calc(50% - 210px)';p.style.top='90px';
  p.innerHTML='<div class="pop-head"><span class="pt">Output · '+f.title+'</span><button class="x">×</button></div>'+
    '<div class="pop-body">'+
      '<span class="rpt-status io-status '+cls+'">'+stTxt+'</span>'+
      '<div class="rpt-section">Report</div>'+
      '<div class="rpt-body">'+o.report+'</div>'+
      (o.artifacts.length?'<div class="rpt-section">Artifacts</div>'+o.artifacts.map(a=>'<div class="rpt-art"><svg class="ai" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M6 2h8l4 4v16H6z"/><path d="M14 2v4h4"/></svg>'+a.name+'<span class="at">'+a.type+'</span></div>').join(''):'')+
      (o.diffs.length?'<div class="rpt-section">Diffs</div><div class="diff">'+o.diffs.map(d=>diffHTML(d)).join('<div class="dc"> </div>')+'</div>':'')+
    '</div>';
  document.body.appendChild(p);currentPop=p;dragPop(p);
  p.querySelector('.x').addEventListener('click',closePop);
}
function diffHTML(d){
  let h='<span class="dh">'+d.file+'</span>';
  d.lines.forEach(l=>{const c=l[0]==='+'?'da':l[0]==='-'?'dd':'dc';h+='<span class="'+c+'">'+l.replace(/</g,'&lt;')+'</span>';});
  return h;
}

/* one floating pop at a time + drag */
let currentPop=null;
function closePop(){if(currentPop){currentPop.remove();currentPop=null;}}
/* clicking anywhere outside an open popup closes it (but not while interacting with it,
   a context menu, or a terminal). The opening click can't close it — the pop doesn't
   exist yet at that pointerdown. */
document.addEventListener('pointerdown',e=>{
  if(currentPop && !currentPop.contains(e.target) && !e.target.closest('.ctxmenu,.term')) closePop();
});
function dragPop(p){
  const head=p.querySelector('.pop-head');
  head.addEventListener('pointerdown',e=>{
    if(e.target.classList.contains('x'))return;
    p.style.left=p.offsetLeft+'px';p.style.top=p.offsetTop+'px';
    const sx=e.clientX,sy=e.clientY,ox=p.offsetLeft,oy=p.offsetTop;
    const move=ev=>{p.style.left=(ox+ev.clientX-sx)+'px';p.style.top=(oy+ev.clientY-sy)+'px';};
    const up=()=>{window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);};
    window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
  });
}

/* ===== GATE / VERIFICATION CONFIG ===== */
function openGateConfig(g,ctx){
  closePop();
  const isVerify=ctx.type==='verify';
  const p=document.createElement('div');p.className='pop';p.style.width='380px';p.style.left='calc(50% - 190px)';p.style.top='100px';
  const kindList = isVerify ? ['code','human'] : ['code','human','formation'];
  const kindGrid=kindList.map(k=>{const v=GATE_KINDS[k];let label=v.name;if(k==='formation'){const jf=judgeEntry(g);if(jf)label='↻ '+nodeTitle(jf);}return '<div class="gk '+(g.kinds.includes(k)?'sel':'')+'" data-k="'+k+'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7">'+v.ic+'</svg>'+label+'</div>';}).join('');
  let midBlock;
  if(isVerify){
    midBlock='<label>If it fails</label>'+
      '<div class="radio-row">'+
        '<div class="radio-opt '+(g.onFail==='block'?'sel':'')+'" data-of="block"><div class="t">⊘ Block</div><div class="d">stop here — work goes no further</div></div>'+
        '<div class="radio-opt '+(g.onFail==='pushback'?'sel':'')+'" data-of="pushback"><div class="t">↩ Push back</div><div class="d">send back to the agents with feedback</div></div>'+
      '</div>';
  } else {
    midBlock='<label>Outputs</label>'+
      '<div class="route-note">Drag from the <b style="color:var(--sage)">green pass</b> port to the next step, and from the <b style="color:var(--terra-d)">red fail</b> port to a fallback or an earlier formation. An unwired fail = block.<br>Use the gold <b style="color:var(--gold)">+</b> on top of the gate to judge with a formation.</div>';
  }
  p.innerHTML='<div class="pop-head"><span class="pt">'+(isVerify?'Verification':'Gate')+'</span><button class="x">×</button></div>'+
    '<div class="pop-body">'+
      '<label>'+(isVerify?'How is this verified?':'What checks this gate?')+' <span style="color:var(--dimmer);text-transform:none;letter-spacing:0">(combine any)</span></label>'+
      '<div class="gate-kinds" id="gk" style="grid-template-columns:repeat('+kindList.length+',1fr)">'+kindGrid+'</div>'+
      '<label>Criterion to pass</label>'+
      '<textarea id="g-crit">'+g.criterion+'</textarea>'+
      midBlock+
      '<label>Verdict <span style="color:var(--dimmer);text-transform:none;letter-spacing:0">(mock — to feel both outcomes)</span></label>'+
      '<div class="verdict-row">'+
        '<div class="vbtn pass '+(g.verdict==='pass'?'sel':'')+'" data-v="pass">✓ passes<span class="note">MOCK</span></div>'+
        '<div class="vbtn fail '+(g.verdict==='fail'?'sel':'')+'" data-v="fail">✕ fails<span class="note">MOCK</span></div>'+
      '</div>'+
      '<button class="save" id="g-del" style="background:transparent;color:var(--terra-d);border:1px solid var(--line);margin-top:16px">Remove '+(isVerify?'verification':'gate')+'</button>'+
    '</div>';
  document.body.appendChild(p);currentPop=p;dragPop(p);
  p.querySelector('.x').addEventListener('click',closePop);
  p.querySelectorAll('#gk .gk').forEach(el=>el.addEventListener('click',()=>{
    const k=el.dataset.k, i=g.kinds.indexOf(k);
    pushUndo();
    if(k==='formation'){
      if(gateHasJudge(g)) detachJudge(g);
      else { closePop(); openJudgePicker(g); return; }
    } else {
      if(i>=0){ if(g.kinds.length>1) g.kinds.splice(i,1); } else g.kinds.push(k);
    }
    openGateConfig(g,ctx); rerender();
  }));
  const crit=p.querySelector('#g-crit');
  crit.addEventListener('focus',()=>{if(!crit._pushed){pushUndo();crit._pushed=true;}});
  crit.addEventListener('input',()=>{g.criterion=crit.value;});
  crit.addEventListener('blur',()=>{rerender();});
  const radios=p.querySelectorAll('.radio-opt');
  radios.forEach(el=>el.addEventListener('click',()=>{pushUndo();g.onFail=el.dataset.of;openGateConfig(g,ctx);rerender();}));
  const jsel=p.querySelector('#g-judge');
  if(jsel)jsel.addEventListener('change',()=>{pushUndo();jsel.value?attachJudge(g,jsel.value):detachJudge(g);openGateConfig(g,ctx);rerender();});
  p.querySelectorAll('.vbtn').forEach(el=>el.addEventListener('click',()=>{g.verdict=el.dataset.v;openGateConfig(g,ctx);}));
  p.querySelector('#g-del').addEventListener('click',()=>{
    pushUndo();
    if(isVerify){ctx.f.verification=null;} else {deleteNode(g.id);}
    closePop();rerender();
  });
}

/* ===== GATE TOKEN (topbar): drag onto canvas → drop a gate node ===== */
document.getElementById('gateToken').addEventListener('pointerdown',e=>{
  e.preventDefault();
  const sx=e.clientX,sy=e.clientY;let started=false,ghost=null;
  const onMove=ev=>{
    if(!started){if(Math.abs(ev.clientX-sx)+Math.abs(ev.clientY-sy)<4)return;started=true;
      ghost=document.createElement('div');ghost.className='gateghost';ghost.innerHTML=GATE_SVG;document.body.appendChild(ghost);}
    ghost.style.left=ev.clientX+'px';ghost.style.top=ev.clientY+'px';
  };
  const onUp=ev=>{
    window.removeEventListener('pointermove',onMove);window.removeEventListener('pointerup',onUp);
    if(ghost)ghost.remove();
    const vr=viewport.getBoundingClientRect();
    let w=null;
    if(started && ev.clientX>vr.left && ev.clientX<vr.right && ev.clientY>vr.top && ev.clientY<vr.bottom) w=screenToWorld(ev.clientX,ev.clientY);
    else if(!started) w=screenToWorld(vr.left+vr.width*0.5, vr.top+vr.height*0.4);
    if(!w)return; // dragged but dropped off-canvas
    pushUndo();gateNodes.push(makeGateNode(['code'],Math.round(w.x-75),Math.round(w.y-31)));rerender();
  };
  window.addEventListener('pointermove',onMove);window.addEventListener('pointerup',onUp);
});

/* ===== OUTPUT GENERATION =====  ▼▼ MOCK #4 — fabricated report/diffs/status ▼▼ */
function mockReport(f){
  const goal=f.input.goal||'the assigned task';
  const staffed=f.slots.filter(s=>s.agentId).map(s=>s.agentId);
  const lead=staffed[0]||'an agent';
  if(f.type==='solo')return{status:'done',summary:'task complete',
    report:'<b>'+lead+'</b> handled “'+goal+'”. Scoped the work, made the change, and left notes for review.',
    artifacts:[{name:'summary.md',type:'doc'}],diffs:[]};
  if(f.type==='peer')return{status:'review',summary:'synthesis ready',
    report:staffed.join(' & ')+' explored “'+goal+'” as peers — two independent reads, then a synthesis. They disagree on one point (caching) and flagged it for you.',
    artifacts:[{name:'options.md',type:'doc'},{name:'tradeoffs.md',type:'doc'}],diffs:[]};
  if(f.type==='flow')return{status:'done',summary:'shipped',
    report:'Pipeline ran end-to-end on “'+goal+'”. Plan → Execute → Push completed; tests green; opened a PR.',
    artifacts:[{name:'PR #482',type:'link'},{name:'run.log',type:'log'}],
    diffs:[{file:'src/SessionPanel.tsx',lines:['@@ search box @@','-  <input placeholder="Filter sessions..." />','+  <input placeholder="Fuzzy find · ⌘K" onKeyDown={onFuzzy} />','   // highlight matched ranges','+  const ranked = fuzzyRank(sessions, q)']}]};
  return{status:'done',summary:'work routed',
    report:'<b>'+lead+'</b> orchestrated “'+goal+'”, assigning '+(staffed.length-1||'no')+' worker(s) and merging their results.',
    artifacts:[{name:'dispatch.json',type:'data'}],diffs:[]};
}

/* ===== ASSIGNMENT (agents may appear in multiple slots) ===== */
function assignDirect(agentId,fId,sId){const sl=findSlot(fId,sId);if(sl)sl.agentId=agentId;}
function rerender(snapSlot){
  gateNodes.forEach(syncJudgeKind);
  renderFormations();renderGates();renderMissions();renderRoster();drawWires();
  if(snapSlot){const el=boardInner.querySelector('.slot[data-fid="'+snapSlot.fid+'"][data-sid="'+snapSlot.sid+'"]');if(el){el.classList.add('just');setTimeout(()=>el.classList.remove('just'),360);}}
}

/* ===== POINTER: click agent → terminal · drag → snap into a slot ===== */
function beginPointer(e,agentId,from){
  if(e.button!==undefined && e.button!==0)return;
  e.preventDefault(); e.stopPropagation();
  const sx=e.clientX, sy=e.clientY;
  let started=false, target=null, ghost=null;
  const hit=ev=>{
    let found=null;
    boardInner.querySelectorAll('.slot').forEach(sl=>{
      const r=sl.getBoundingClientRect();const cx=r.left+r.width/2,cy=r.top+r.height/2;
      const within=ev.clientX>r.left-14&&ev.clientX<r.right+14&&ev.clientY>r.top-14&&ev.clientY<r.bottom+14;
      if(within){const d=Math.hypot(ev.clientX-cx,ev.clientY-cy);if(!found||d<found.d)found={el:sl,d};}
    });
    if(target&&(!found||found.el!==target))target.classList.remove('snap');
    if(found){found.el.classList.add('snap');target=found.el;} else target=null;
  };
  const onMove=ev=>{
    if(!started){
      if(Math.abs(ev.clientX-sx)+Math.abs(ev.clientY-sy)<4)return;
      started=true;const a=agent(agentId);
      ghost=document.createElement('div');ghost.className='ghost';ghost.innerHTML='<img src="'+a.av+'"/>';document.body.appendChild(ghost);
    }
    ghost.style.left=ev.clientX+'px';ghost.style.top=ev.clientY+'px';hit(ev);
  };
  const onUp=ev=>{
    window.removeEventListener('pointermove',onMove);window.removeEventListener('pointerup',onUp);
    if(!started){ openTerm(agentId,ev.clientX,ev.clientY); return; }
    if(ghost)ghost.remove();
    if(target){ target.classList.remove('snap'); pushUndo(); assignDirect(agentId,target.dataset.fid,target.dataset.sid); rerender({fid:target.dataset.fid,sid:target.dataset.sid}); }
    else if(from){ pushUndo(); const sl=findSlot(from.fid,from.sid); if(sl)sl.agentId=null; rerender(); }
  };
  window.addEventListener('pointermove',onMove);window.addEventListener('pointerup',onUp);
}

/* ===== MOVE NODES (grab the header / empty body for formations, the pill for gates) ===== */
function dragCard(card,f){
  card.addEventListener('pointerdown',e=>{
    if(e.button!==0)return;
    if(e.target.closest('.frun,.slot,.port,.fio,.verify-band'))return;
    const sx=e.clientX,sy=e.clientY,ox=f.x,oy=f.y;let pushed=false;
    const move=ev=>{
      if(!pushed){if(Math.abs(ev.clientX-sx)+Math.abs(ev.clientY-sy)<3)return;pushUndo();pushed=true;draggingNodeId=f.id;}
      f.x=ox+(ev.clientX-sx)/scale;f.y=oy+(ev.clientY-sy)/scale;card.style.left=f.x+'px';card.style.top=f.y+'px';drawWires();
    };
    const up=()=>{draggingNodeId=null;window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);};
    window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
  });
}
function dragGate(card,g){
  card.addEventListener('pointerdown',e=>{
    if(e.button!==0)return;
    if(e.target.closest('.port,.pjudge'))return;
    const sx=e.clientX,sy=e.clientY,ox=g.x,oy=g.y;let moved=false,pushed=false;
    const move=ev=>{
      if(!moved && Math.abs(ev.clientX-sx)+Math.abs(ev.clientY-sy)<3)return;
      if(!pushed){pushUndo();pushed=true;draggingNodeId=g.id;} moved=true;
      g.x=ox+(ev.clientX-sx)/scale;g.y=oy+(ev.clientY-sy)/scale;card.style.left=g.x+'px';card.style.top=g.y+'px';drawWires();
    };
    const up=()=>{draggingNodeId=null;window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);
      if(!moved){openGateConfig(g,{type:'node',node:g});}};
    window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
  });
}

/* ===== CONTEXT MENUS ===== */
let menuEl=null;
function closeMenu(){if(menuEl){menuEl.remove();menuEl=null;}}
document.addEventListener('pointerdown',e=>{if(menuEl&&!menuEl.contains(e.target))closeMenu();});
document.addEventListener('scroll',closeMenu,true);
function showMenu(x,y,items){
  closeMenu();
  const m=document.createElement('div');m.className='ctxmenu';
  items.forEach(it=>{
    if(it.head){const h=document.createElement('div');h.className='mhead';h.textContent=it.head;m.appendChild(h);return;}
    if(it.div){const d=document.createElement('div');d.className='mdiv';m.appendChild(d);return;}
    const mi=document.createElement('div');mi.className='mi'+(it.danger?' danger':'');
    const icon=it.img?'<img class="mimg" src="'+it.img+'"/>':(it.dot?'<span class="dot" style="background:'+it.dot+'"></span>':(it.icon?'<svg class="mic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7">'+it.icon+'</svg>':''));
    mi.innerHTML=icon+'<span>'+it.label+'</span>'+(it.sm?'<span class="sm">'+it.sm+'</span>':'');
    mi.addEventListener('click',()=>{closeMenu();it.fn&&it.fn();});
    m.appendChild(mi);
  });
  document.body.appendChild(m);
  const r=m.getBoundingClientRect();
  m.style.left=Math.min(x,innerWidth-r.width-10)+'px';
  m.style.top=Math.min(y,innerHeight-r.height-10)+'px';
  menuEl=m;
}
const IC={term:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l3 3-3 3M13 15h4"/>',
  gate:'<path d="M4 20V10a8 8 0 0116 0v10"/><path d="M3 20h18M9 20V9M15 20V9"/>',
  shield:'<path d="M12 3l7 3v5c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6z"/><path d="M9 12l2 2 4-4"/>',
  plus:'<path d="M12 5v14M5 12h14"/>',trash:'<path d="M4 7h16M9 7V5h6v2M6 7l1 13h10l1-13"/>',
  copy:'<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M4 16V4h12"/>',
  rename:'<path d="M4 20h16M6 16l9-9 3 3-9 9H6z"/>',star:'<path d="M12 3l2.5 5.5L20 9l-4 4 1 6-5-3-5 3 1-6-4-4 5.5-.5z"/>',
  unassign:'<path d="M5 12h14"/>',run:'<path d="M6 4l14 8-14 8z"/>',
  assign:'<path d="M16 3l4 4-4 4M20 7H9M8 21l-4-4 4-4M4 17h11"/>'};

function menuAgent(e,agentId){
  showMenu(e.clientX,e.clientY,[
    {head:agentId},
    {label:'Open terminal',icon:IC.term,fn:()=>openTerm(agentId,e.clientX,e.clientY)},
  ]);
}
function menuFormation(e,f){
  const items=[{head:TYPES[f.type].name+' formation'},
    {label:'Run',icon:IC.run,fn:()=>runFormation(f,nodeCardEl(f.id))},
    {label:'Rename…',icon:IC.rename,fn:()=>renameFormation(f)},
  ];
  if(f.type!=='solo')items.push({label:f.type==='flow'?'Add step':'Add slot',icon:IC.plus,fn:()=>addSlot(f)});
  items.push({label:'Add input',icon:IC.plus,fn:()=>{pushUndo();f.inputs.push({id:newSid(),label:'Input '+(f.inputs.length+1)});rerender();}});
  items.push({label:'Add output',icon:IC.plus,fn:()=>{pushUndo();f.outputs.push({id:newSid(),label:'Output '+(f.outputs.length+1)});rerender();}});
  if(!f.verification)items.push({label:'Add verification',icon:IC.shield,fn:()=>{pushUndo();f.verification=makeVerification(['code']);rerender();openGateConfig(f.verification,{type:'verify',f});}});
  else{
    items.push({label:'Configure verification',icon:IC.shield,fn:()=>openGateConfig(f.verification,{type:'verify',f})});
    items.push({label:'Remove verification',icon:IC.unassign,fn:()=>{pushUndo();f.verification=null;rerender();}});
  }
  if(f.output)items.push({label:'Clear output',icon:IC.unassign,fn:()=>{pushUndo();f.output=null;rerender();}});
  items.push({label:'Set input…',icon:IC.assign,fn:()=>openInputEditor(f)});
  items.push({label:'Duplicate',icon:IC.copy,fn:()=>dupFormation(f)});
  items.push({div:true});
  items.push({label:'Delete formation',icon:IC.trash,danger:true,fn:()=>{pushUndo();deleteNode(f.id);rerender();}});
  showMenu(e.clientX,e.clientY,items);
}
/* dedicated right-click menus for the parts of a formation card */
function menuInputRow(e,f){
  const items=[{head:'Input'},
    {label:'Edit input…',icon:IC.assign,fn:()=>openInputEditor(f)},
    {label:'Add input',icon:IC.plus,fn:()=>{pushUndo();f.inputs.push({id:newSid(),label:'Input '+(f.inputs.length+1)});rerender();}},
  ];
  if(f.inputs.length>1)items.push({label:'Remove last input',icon:IC.unassign,danger:true,fn:()=>{pushUndo();const last=f.inputs[f.inputs.length-1];connections=connections.filter(c=>!(c.to===f.id&&(c.toPort||defIn(f.id))===last.id));f.inputs.pop();rerender();}});
  items.push({div:true},{label:'Run formation',icon:IC.run,fn:()=>runFormation(f,nodeCardEl(f.id))});
  showMenu(e.clientX,e.clientY,items);
}
function menuOutputRow(e,f){
  const items=[{head:'Output'}];
  if(f.output){items.push({label:'Open report',icon:IC.copy,fn:()=>openReport(f)});items.push({label:'Clear output',icon:IC.unassign,fn:()=>{pushUndo();f.output=null;rerender();}});}
  else items.push({label:'Run formation',icon:IC.run,fn:()=>runFormation(f,nodeCardEl(f.id))});
  items.push({label:'Add output',icon:IC.plus,fn:()=>{pushUndo();f.outputs.push({id:newSid(),label:'Output '+(f.outputs.length+1)});rerender();}});
  if(f.outputs.length>1)items.push({label:'Remove last output',icon:IC.unassign,danger:true,fn:()=>{pushUndo();const last=f.outputs[f.outputs.length-1];connections=connections.filter(c=>!(c.from===f.id&&(c.fromPort||defOut(f.id))===last.id));f.outputs.pop();rerender();}});
  showMenu(e.clientX,e.clientY,items);
}
function menuVerification(e,f){
  if(!f.verification){
    showMenu(e.clientX,e.clientY,[{head:'Verification'},{label:'Add verification',icon:IC.shield,fn:()=>{pushUndo();f.verification=makeVerification(['code']);rerender();openGateConfig(f.verification,{type:'verify',f});}}]);
    return;
  }
  showMenu(e.clientX,e.clientY,[{head:'Verification'},
    {label:'Configure…',icon:IC.shield,fn:()=>openGateConfig(f.verification,{type:'verify',f})},
    {div:true},
    {label:'Remove verification',icon:IC.trash,danger:true,fn:()=>{pushUndo();f.verification=null;rerender();}},
  ]);
}
function menuGate(e,g){
  showMenu(e.clientX,e.clientY,[
    {head:'Gate'},
    {label:'Configure…',icon:IC.gate,fn:()=>openGateConfig(g,{type:'node',node:g})},
    {label:'Duplicate',icon:IC.copy,fn:()=>{pushUndo();const c=makeGateNode(g.kinds,g.x+34,g.y+38);c.criterion=g.criterion;c.onFail=g.onFail;c.verdict=g.verdict;gateNodes.push(c);rerender();}},
    {div:true},
    {label:'Delete gate',icon:IC.trash,danger:true,fn:()=>{pushUndo();deleteNode(g.id);rerender();}},
  ]);
}
function menuWire(e,c){
  const items=[{head:'Connection'}];
  if(c.via)items.push({label:'Reset routing',icon:IC.unassign,fn:()=>{pushUndo();delete c.via;rerender();}});
  items.push({div:true});
  items.push({label:'Remove connection',icon:IC.trash,danger:true,fn:()=>{pushUndo();connections.splice(connections.indexOf(c),1);rerender();}});
  showMenu(e.clientX,e.clientY,items);
}
function menuSlot(e,f,sl){
  const ax=e.clientX, ay=e.clientY;
  const items=[{head:sl.label}];
  items.push({label:sl.agentId?'Change agent…':'Assign agent…',icon:IC.assign,fn:()=>showAssignMenu(ax,ay,f,sl)});
  if(sl.agentId){
    items.push({label:'Open terminal',icon:IC.term,fn:()=>openTerm(sl.agentId,e.clientX,e.clientY)});
    items.push({label:'Unassign agent',icon:IC.unassign,fn:()=>{pushUndo();sl.agentId=null;rerender();}});
  }
  if(f.type==='orchestrated' && !sl.ctrl){
    items.push({label:'Make controller',icon:IC.star,fn:()=>{pushUndo();f.slots.forEach(s=>s.ctrl=false);sl.ctrl=true;rerender();}});
  }
  if(f.type!=='solo'){
    items.push({div:true});
    items.push({label:f.type==='flow'?'Add step after':'Add slot',icon:IC.plus,fn:()=>addSlot(f,sl)});
    if(f.slots.filter(s=>!s.ctrl).length>1 || (sl.ctrl===false))
      items.push({label:'Remove slot',icon:IC.trash,danger:true,fn:()=>{if(sl.ctrl)return;pushUndo();f.slots=f.slots.filter(s=>s!==sl);rerender();}});
  }
  showMenu(e.clientX,e.clientY,items);
}
function showAssignMenu(x,y,f,sl){
  const items=[{head:(sl.agentId?'Change · ':'Assign to ')+sl.label}];
  AGENTS.forEach(a=>items.push({img:a.av,label:a.id,sm:a.role,fn:()=>{pushUndo();sl.agentId=a.id;rerender({fid:f.id,sid:sl.id});}}));
  if(sl.agentId){items.push({div:true});items.push({label:'Clear slot',icon:IC.unassign,danger:true,fn:()=>{pushUndo();sl.agentId=null;rerender();}});}
  showMenu(x,y,items);
}

/* board right-click → new formation / gate */
viewport.addEventListener('contextmenu',e=>{
  if(e.target.closest('.formation,.gatecard'))return;
  e.preventDefault();
  newFormationMenu(e.clientX,e.clientY,e);
});
document.getElementById('newBtn').addEventListener('click',e=>{
  const r=e.currentTarget.getBoundingClientRect();newFormationMenu(r.left,r.bottom+6);
});
function newFormationMenu(x,y,e){
  let bx=120,by=120;
  if(e){const w=screenToWorld(e.clientX,e.clientY);bx=w.x-90;by=w.y-40;}
  else {const r=viewport.getBoundingClientRect();const w=screenToWorld(r.left+r.width/2,r.top+r.height*0.4);bx=w.x-130;by=w.y-50;}
  const add=(type,title,defs)=>()=>{pushUndo();formations.push(makeFormation(type,title,defs,Math.max(10,Math.round(bx)),Math.max(10,Math.round(by))));rerender();};
  showMenu(x,y,[
    {head:'New'},
    {label:'Mission',dot:'#6b9fff',sm:'entry point',fn:()=>{pushUndo();missions.push(makeMission('New mission',Math.max(10,Math.round(bx)),Math.max(10,Math.round(by))));rerender();}},
    {div:true},
    {head:'Formation'},
    {label:'Solo',dot:TYPES.solo.color,sm:'1 agent',fn:add('solo','Solo task',[{label:'Agent'}])},
    {label:'Peer',dot:TYPES.peer.color,sm:'2+ equals',fn:add('peer','Peer huddle',[{label:'Peer'},{label:'Peer'}])},
    {label:'Flow',dot:TYPES.flow.color,sm:'A→B→C',fn:add('flow','New flow',[{label:'Step 1'},{label:'Step 2'},{label:'Step 3'}])},
    {label:'Orchestrated',dot:TYPES.orchestrated.color,sm:'controller',fn:add('orchestrated','Orchestration',[{label:'Orchestrator',ctrl:true},{label:'Agent'},{label:'Agent'}])},
    {div:true},
    {label:'Gate',icon:IC.gate,sm:'between formations',fn:()=>{pushUndo();gateNodes.push(makeGateNode(['code'],Math.max(10,Math.round(bx)),Math.max(10,Math.round(by))));rerender();}},
    {div:true},
    {head:'Plan templates'},
    {label:'Ship a change',icon:IC.run,sm:'flow',fn:add('flow','Ship a change',[{label:'Plan'},{label:'Execute'},{label:'Push'}])},
    {label:'Triage desk',icon:IC.star,sm:'orchestrated',fn:add('orchestrated','Triage desk',[{label:'Orchestrator',ctrl:true},{label:'Agent'},{label:'Agent'},{label:'Agent'}])},
  ]);
}

function addSlot(f,after){
  pushUndo();
  const label=f.type==='flow'?('Step '+(f.slots.length+1)):(f.type==='orchestrated'?'Agent':'Peer');
  const ns={id:newSid(),label,ctrl:false,agentId:null};
  if(after){const i=f.slots.indexOf(after);f.slots.splice(i+1,0,ns);}
  else f.slots.push(ns);
  rerender();
}
function renameFormation(f){
  const name=prompt('Rename formation',f.title);
  if(name!==null && name.trim()){pushUndo();f.title=name.trim();rerender();}
}
function dupFormation(f){
  pushUndo();
  const copy=makeFormation(f.type,f.title+' (copy)',f.slots.map(s=>({label:s.label,ctrl:s.ctrl})),f.x+34,f.y+34);
  if(f.verification){copy.verification=makeVerification(f.verification.kinds);copy.verification.criterion=f.verification.criterion;copy.verification.onFail=f.verification.onFail;copy.verification.verdict=f.verification.verdict;}
  formations.push(copy);rerender();
}

/* ===== RUN (the feel of coordination) =====
   ▼▼ MOCK #5 — setTimeout theatre + MOCK #6 hard-coded verdicts/statuses ▼▼ */
let running=new Set();
let arrived=new Set(); // connections whose work has been delivered in the current run
function inputsReady(f){return connections.filter(c=>c.to===f.id&&!isJudgeConn(c)).every(c=>arrived.has(c));}
/* a mission is the entry point — it starts the whole downstream chain */
function runMission(m){
  arrived=new Set();
  m.output={summary:(m.goal||m.title||'mission')};
  const card=nodeCardEl(m.id); const s=card&&card.querySelector('[data-status]');
  const outs=connections.filter(c=>c.from===m.id);
  if(!outs.length){ if(s){s.textContent='⚠ wire the mission to a step';setTimeout(()=>{if(s)s.textContent='';},1800);} return; }
  if(s)s.textContent='▶ mission running…';
  flowFrom(m,new Set([m.id]));
}
const time=()=>new Date().toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
function gateEl(id){return document.querySelector('[data-gate="'+id+'"]');}
function setNodeState(g,st){g._state=st;const el=gateEl(g.id);if(el){el.classList.remove('evaluating','pass','fail');if(st)el.classList.add(st);}}
function setStatus(card,txt){if(!card)return;const s=card.querySelector('[data-status]');if(s)s.textContent=txt;}
function setStatusByFid(fid,txt){setStatus(nodeCardEl(fid),txt);}

/* evaluate a gate/verification, then branch.
   A gate with a JUDGE actually RUNS its judge formation (the one it sends work to);
   the judge chain executes and, when the exit formation finishes, the verdict applies. */
let pendingJudge={}; // formationId → callback to run when that formation finishes (judge exit)
function evalGate(g,onPass,onFail){
  setNodeState(g,'evaluating');
  const decide=()=>{ const pass=g.verdict==='pass'; setNodeState(g,pass?'pass':'fail'); pass?onPass():onFail(g); };
  const entry = gateHasJudge(g) ? judgeEntry(g) : null;
  const canRun = entry && entry.slots.some(s=>s.agentId);
  if(canRun){
    const ret=judgeReturn(g); const exitId=ret?ret.from:entry.id;
    pendingJudge[exitId]=()=>setTimeout(decide,420);
    if(entry.input && !entry.input.goal) entry.input.goal='judge · '+g.criterion;
    setStatusByFid(entry.id,'⚖ judging…');
    runFormation(entry, nodeCardEl(entry.id), new Set([entry.id]));
  } else {
    setTimeout(decide,850); // code/human check (or an unstaffed judge) — mock verdict
  }
}

/* walk wires backward from a gate to the first feeding formation (for pushback) */
function upstreamFormation(node){
  let cur=node,guard=0;
  while(cur && isGate(cur) && guard++<24){
    const c=connections.find(x=>x.to===cur.id&&!isJudgeConn(x)); cur=c?nodeById(c.from):null;
  }
  return (cur && !isGate(cur))?cur:null;
}
/* flow output from a formation along its out-wires; gates evaluate and branch */
function flowFrom(node,visited){
  visited=visited||new Set([node.id]);
  connections.filter(c=>c.from===node.id&&!isJudgeConn(c)).forEach((c,idx)=>{
    const to=nodeById(c.to); if(!to)return;
    setTimeout(()=>{
      c.flowing=true; drawWires();
      setTimeout(()=>{
        c.flowing=false; drawWires();
        if(isGate(to)){
          if(visited.has(to.id))return; visited.add(to.id);
          setNodeState(to,null);
          evalGate(to, ()=>followBranch(to,'pass',visited), ()=>followBranch(to,'fail',visited));
        } else {
          if(node.output && !to.input.goal){ to.input.goal=node.output.summary+' — from '+nodeTitle(node); }
          arrived.add(c);
          if(visited.has(to.id))return;
          if(!inputsReady(to)){ const reqs=connections.filter(x=>x.to===to.id&&!isJudgeConn(x)); setStatusByFid(to.id,'⏸ waiting · '+reqs.filter(x=>arrived.has(x)).length+'/'+reqs.length+' inputs'); return; }
          visited.add(to.id);
          const tcard=nodeCardEl(to.id);
          if(tcard && !running.has(to.id)) runFormation(to,tcard,visited);
        }
      }, 620);
    }, 450+idx*240);
  });
}
/* a gate decided — send work down the wires on the matching output */
function followBranch(gate,branch,visited){
  const outs=connections.filter(c=>c.from===gate.id && (c.fromPort||'pass')===branch);
  const up=upstreamFormation(gate);
  if(!outs.length){
    if(branch==='fail' && up){ if(up.output)up.output.status='blocked'; setStatusByFid(up.id,'⊘ blocked · '+gateLabel(gate)+' gate'); rerender(); }
    return;
  }
  outs.forEach((c,idx)=>{
    const to=nodeById(c.to); if(!to)return;
    setTimeout(()=>{
      c.flowing=true; drawWires();
      setTimeout(()=>{
        c.flowing=false; drawWires();
        if(isGate(to)){
          if(visited.has(to.id))return; visited.add(to.id);
          setNodeState(to,null);
          evalGate(to, ()=>followBranch(to,'pass',visited), ()=>followBranch(to,'fail',visited));
        } else {
          if(up && up.output && !to.input.goal){
            to.input.goal=(branch==='fail'?'↩ revise — '+gateLabel(gate)+': '+gate.criterion+' · ':'')+up.output.summary+' — from '+nodeTitle(up);
          }
          if(branch==='fail' && to.output) to.output.status='needs-review';
          arrived.add(c);
          if(visited.has(to.id))return;
          if(!inputsReady(to)){ const reqs=connections.filter(x=>x.to===to.id&&!isJudgeConn(x)); setStatusByFid(to.id,'⏸ waiting · '+reqs.filter(x=>arrived.has(x)).length+'/'+reqs.length+' inputs'); return; }
          visited.add(to.id);
          const tcard=nodeCardEl(to.id);
          if(tcard && !running.has(to.id)) runFormation(to,tcard,visited);
        }
      }, 620);
    }, 300+idx*240);
  });
}

function runFormation(f,card,visited){
  if(!card)card=nodeCardEl(f.id);
  if(running.has(f.id))return;
  const topLevel=!visited;
  visited=visited||new Set([f.id]);
  if(topLevel) arrived=new Set();
  const filled=f.slots.filter(s=>s.agentId);
  if(filled.length===0){setStatus(card,'⚠ assign agents first');setTimeout(()=>setStatus(card,''),1600);return;}
  running.add(f.id);card.classList.add('running');f.output=null;
  if(f.verification)setNodeState(f.verification,null);
  const slotEl=sl=>card.querySelector('.slot[data-sid="'+sl.id+'"]');
  const finish=(status,note)=>{
    running.delete(f.id);
    f.output=mockReport(f); if(status)f.output.status=status;
    setStatus(card, status==='blocked'?('⊘ '+note):status?('↩ '+note):('✓ done · '+time()));
    const judgeCb=pendingJudge[f.id]; if(judgeCb)delete pendingJudge[f.id];
    setTimeout(()=>{ rerender(); if(judgeCb){judgeCb();} else if(!status) flowFrom(f,visited); }, 540);
  };
  const verifyThenFinish=()=>{
    if(!f.verification){finish();return;}
    evalGate(f.verification, ()=>finish(), (g)=>{
      if(g.onFail==='pushback') finish('needs-review','verify pushed back');
      else finish('blocked','blocked at verify');
    });
  };

  if(f.type==='flow'){
    const steps=f.slots; const arrows=[...card.querySelectorAll('.flow-arrow')];
    let i=0;
    const proceed=()=>{
      if(i>0){const pe=slotEl(steps[i-1]);pe&&pe.classList.add('done');if(arrows[i-1])arrows[i-1].classList.add('lit');}
      if(i>=steps.length){verifyThenFinish();return;}
      const e=slotEl(steps[i]);e&&e.classList.add('active');
      setStatus(card,'→ '+steps[i].label.toLowerCase()+(steps[i].agentId?' · '+steps[i].agentId:' · (empty)'));
      i++;setTimeout(proceed,900);
    };
    proceed();
  } else if(f.type==='orchestrated'){
    const ctrl=f.slots.find(s=>s.ctrl)||f.slots[0];
    const workers=f.slots.filter(s=>s!==ctrl);
    const ce=slotEl(ctrl);ce&&ce.classList.add('active');
    setStatus(card,'orchestrating · '+(ctrl.agentId||'controller')+' deciding…');
    let i=0;
    const assign=()=>{
      if(i>=workers.length){workers.forEach(w=>{const e=slotEl(w);e&&e.classList.add('done');});ce&&ce.classList.add('done');verifyThenFinish();return;}
      const w=workers[i];const e=slotEl(w);
      if(w.agentId){e&&e.classList.add('active');setStatus(card,'↳ assigned to '+w.agentId);}
      i++;setTimeout(assign,820);
    };
    setTimeout(assign,900);
  } else if(f.type==='peer'){
    const h=card.querySelector('.huddle');h&&h.classList.add('running');
    f.slots.forEach(s=>{const e=slotEl(s);if(s.agentId)e&&e.classList.add('active');});
    setStatus(card,'synthesizing · '+f.slots.filter(s=>s.agentId).map(s=>s.agentId).join(' ⇄ '));
    setTimeout(()=>{f.slots.forEach(s=>{const e=slotEl(s);if(s.agentId)e&&e.classList.add('done');});verifyThenFinish();},2200);
  } else { // solo
    const s=f.slots[0];const e=slotEl(s);e&&e.classList.add('active');
    setStatus(card,'working · '+(s.agentId||''));
    setTimeout(()=>{e&&e.classList.add('done');verifyThenFinish();},1700);
  }
}

/* ===== TERMINAL POPUP =====  ▼▼ MOCK #2 — canned terminal lines ▼▼ */
let termZ=150;
const openTerms={};
function feed(a){
  const p='<span class="pmt">'+a.id+' ›</span> ';
  return ['<span class="l">'+p+'status</span>',
    '<span class="l"><span class="mut">'+a.role+' · attached on tmux socket</span></span>',
    '<span class="l"><span class="ok">✓ ready</span></span>',
    '<span class="l">'+p+'<span class="pcur"></span></span>'].join('');
}
function openTerm(agentId,x,y){
  if(openTerms[agentId]){const t=openTerms[agentId];t.style.zIndex=++termZ;t.animate([{boxShadow:'0 0 0 3px var(--terra)'},{boxShadow:'0 30px 64px -26px rgba(0,0,0,.9)'}],{duration:500});return;}
  const a=agent(agentId);
  const t=document.createElement('div');t.className='term';
  t.style.width='330px';t.style.height='234px';
  const vr=viewport.getBoundingClientRect();
  const inView = x>vr.left+10 && x<vr.right && y>vr.top && y<vr.bottom;
  const sx = inView ? x : vr.left+vr.width*0.46;
  const sy = inView ? y : vr.top+vr.height*0.4;
  const wp=screenToWorld(sx,sy);
  const off=(Object.keys(openTerms).length%5)*26;
  t.style.left=(wp.x-40+off)+'px';t.style.top=(wp.y-20+off)+'px';t.style.zIndex=++termZ;
  t.innerHTML='<div class="term-head"><img src="'+a.av+'"/><div class="ti"><div class="n">'+a.id+'</div><div class="r">'+a.role+'</div></div><button class="x">×</button></div>'+
    '<div class="term-body">'+feed(a)+'</div>'+
    '<div class="resize-handle"></div>';
  openTerms[agentId]=t;
  t.querySelector('.x').addEventListener('click',()=>{t.remove();delete openTerms[agentId];});
  t.addEventListener('pointerdown',()=>{t.style.zIndex=++termZ;});
  const head=t.querySelector('.term-head');
  head.addEventListener('pointerdown',e=>{
    if(e.target.classList.contains('x'))return;
    e.stopPropagation();
    const sx=e.clientX,sy=e.clientY,ox=parseFloat(t.style.left),oy=parseFloat(t.style.top);
    const move=ev=>{t.style.left=(ox+(ev.clientX-sx)/scale)+'px';t.style.top=(oy+(ev.clientY-sy)/scale)+'px';};
    const up=()=>{window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);};
    window.addEventListener('pointermove',move);window.addEventListener('pointerup',up);
  });
  const rh=t.querySelector('.resize-handle');
  rh.addEventListener('pointerdown',e=>{
    e.stopPropagation();
    const sx=e.clientX,sy=e.clientY,ow=t.offsetWidth,oh=t.offsetHeight;rh.setPointerCapture(e.pointerId);
    const move=ev=>{t.style.width=Math.max(250,ow+(ev.clientX-sx)/scale)+'px';t.style.height=Math.max(160,oh+(ev.clientY-sy)/scale)+'px';};
    const up=()=>{rh.removeEventListener('pointermove',move);rh.removeEventListener('pointerup',up);};
    rh.addEventListener('pointermove',move);rh.addEventListener('pointerup',up);
  });
  world.appendChild(t);
}

/* ===== INIT ===== */
seed();renderFormations();renderGates();renderMissions();renderRoster();drawWires();
requestAnimationFrame(()=>{fitView();drawWires();});
// fonts change card metrics — re-fit AND redraw wires once they're ready (and as a timed fallback)
if(document.fonts&&document.fonts.ready)document.fonts.ready.then(()=>{fitView();drawWires();});
setTimeout(()=>{fitView();drawWires();},500);
