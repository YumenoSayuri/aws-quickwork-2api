package wui

const indexHTML = `<!doctype html>
<html lang="zh-CN" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>QuickWork 控制台</title>
<style>
:root{
  --bg:#ffffff; --bg-soft:#fafafa; --plant:#f4f4f5; --line:#e8e6e3;
  --text:#2d2a26; --dim:#8b857d; --dim2:#a8a29a;
  --accent:#2d2a26; --green:#12b76a; --red:#f04438; --amber:#f79009; --blue:#3b82f6;
  --shadow:0 1px 2px rgba(16,24,40,.04),0 1px 3px rgba(16,24,40,.06);
  --radius:14px;
}
html[data-theme=dark]{
  --bg:#111110; --bg-soft:#181816; --plant:#1e1e1b; --line:#2c2c28;
  --text:#f2efe9; --dim:#9b968d; --dim2:#6f6a62;
  --accent:#f2efe9; --shadow:0 1px 3px rgba(0,0,0,.4);
}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);
  font:14px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;
  -webkit-font-smoothing:antialiased}
a{color:inherit;text-decoration:none}
button{font:inherit;cursor:pointer;border:none;border-radius:9px;padding:8px 15px;
  background:var(--plant);color:var(--text);transition:.14s;white-space:nowrap}
button:hover{filter:brightness(.96)}
button.primary{background:var(--accent);color:var(--bg)}
button.primary:hover{opacity:.88;filter:none}
button.ghost{background:transparent;border:1px solid var(--line);color:var(--dim)}
button.ghost:hover{color:var(--text);border-color:var(--dim2)}
button.danger{background:transparent;border:1px solid var(--line);color:var(--red)}
button.danger:hover{background:rgba(240,68,56,.08);border-color:var(--red)}
button:disabled{opacity:.4;cursor:not-allowed}
input,select{font:inherit;background:var(--bg);color:var(--text);
  border:1px solid var(--line);border-radius:9px;padding:9px 12px;width:100%}
input:focus,select:focus{outline:none;border-color:var(--dim2)}
input[readonly]{background:var(--bg-soft)}

.layout{display:flex;min-height:100vh}
aside{width:224px;flex:0 0 224px;background:var(--bg-soft);border-right:1px solid var(--line);
  padding:20px 12px;position:sticky;top:0;height:100vh;overflow:auto}
.brand{display:flex;align-items:center;gap:9px;padding:0 10px 22px;font-weight:650;font-size:15px;letter-spacing:-.01em}
.brand i{width:22px;height:22px;border-radius:7px;display:block;
  background:linear-gradient(135deg,#ff9900,#ffb84d);flex:0 0 auto}
.nav-group{margin-bottom:18px}
.nav-group-label{font-size:11px;color:var(--dim2);padding:0 10px 7px;letter-spacing:.06em}
.nav-item{display:flex;align-items:center;gap:9px;padding:8px 10px;border-radius:9px;
  color:var(--dim);font-size:13.5px;cursor:pointer;transition:.12s}
.nav-item:hover{background:var(--plant);color:var(--text)}
.nav-item.active{background:var(--plant);color:var(--text);font-weight:550}
.nav-item .ico{width:16px;text-align:center;opacity:.75;font-size:13px}
.nav-badge{margin-left:auto;font-size:11px;background:var(--plant);color:var(--dim);
  padding:1px 7px;border-radius:99px;border:1px solid var(--line)}
.nav-item.active .nav-badge{background:var(--bg);color:var(--text)}

main{flex:1;min-width:0;padding:34px 40px 60px;max-width:1120px}
.page{display:none}
.page.on{display:block}
.h-display{font-size:40px;font-weight:750;letter-spacing:-.03em;line-height:1.1;margin:0 0 10px}
.h-sub{color:var(--dim);font-size:13px;margin:0 0 26px}
.h-sub b{color:var(--text);font-weight:550}
.section-title{font-size:12.5px;color:var(--dim);margin:34px 0 4px;
  padding-left:9px;border-left:2px solid var(--green);line-height:1.3}
.section-big{font-size:21px;font-weight:650;letter-spacing:-.02em;margin:0 0 16px}

.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(158px,1fr));gap:14px}
.stat{background:var(--bg);border:1px solid var(--line);border-radius:var(--radius);
  padding:17px 19px;box-shadow:var(--shadow);position:relative;overflow:hidden}
.stat .k{font-size:12px;color:var(--dim);margin-bottom:9px}
.stat .v{font-size:30px;font-weight:700;letter-spacing:-.03em;line-height:1}
.stat .bar{position:absolute;left:0;bottom:0;height:2px;width:100%;background:var(--green);opacity:.9}
.stat.amber .bar{background:var(--amber)}
.stat.red .bar{background:var(--red)}
.stat.dim .bar{background:var(--dim2)}

.card{background:var(--bg);border:1px solid var(--line);border-radius:var(--radius);
  padding:20px 22px;box-shadow:var(--shadow);margin-bottom:16px}
.card-h{display:flex;align-items:center;gap:12px;margin-bottom:16px;flex-wrap:wrap}
.card-h h3{margin:0;font-size:15px;font-weight:600}
.card-h .hint{color:var(--dim);font-size:12.5px;margin:0}
.grow{flex:1}

table{width:100%;border-collapse:collapse;font-size:13.5px}
th{text-align:left;font-weight:500;color:var(--dim);font-size:12px;
  padding:0 12px 11px;border-bottom:1px solid var(--line);white-space:nowrap}
td{padding:14px 12px;border-bottom:1px solid var(--line);vertical-align:middle}
tbody tr:last-child td{border-bottom:none}
tbody tr:hover{background:var(--bg-soft)}
td.act{text-align:right;white-space:nowrap}
td.act button{padding:5px 10px;font-size:12.5px;margin-left:6px}
.who{font-weight:550}
.who-sub{color:var(--dim);font-size:12px;margin-top:2px}
.mono{font-family:ui-monospace,"Cascadia Mono",Consolas,monospace;font-size:12px}
.dot{display:inline-flex;align-items:center;gap:6px;font-size:12.5px;white-space:nowrap}
.dot::before{content:"";width:6px;height:6px;border-radius:50%;background:var(--dim2)}
.dot.ok::before{background:var(--green)}
.dot.warn::before{background:var(--amber)}
.dot.err::before{background:var(--red)}
.dot.off::before{background:var(--dim2)}
.empty{text-align:center;color:var(--dim);padding:44px 0;font-size:13.5px}

.steps{display:flex;gap:10px;flex-wrap:wrap;margin:0 0 24px}
.step{display:flex;align-items:center;gap:8px;font-size:12.5px;color:var(--dim);
  padding:7px 14px;border-radius:99px;border:1px solid var(--line);background:var(--bg)}
.step b{width:17px;height:17px;border-radius:50%;background:var(--plant);color:var(--dim);
  display:grid;place-items:center;font-size:10.5px;font-weight:600}
.step.on{color:var(--text);border-color:var(--text)}
.step.on b{background:var(--text);color:var(--bg)}

.field{margin-bottom:16px}
.field label{display:block;font-size:12.5px;color:var(--dim);margin-bottom:7px}
.field .note{font-size:12px;color:var(--dim);margin-top:6px}
.row{display:flex;gap:9px;align-items:center}
.row input{flex:1;min-width:0}
.row button{flex:0 0 auto}
pre.log{background:var(--bg-soft);border:1px solid var(--line);border-radius:10px;padding:13px;
  max-height:240px;overflow:auto;font-family:ui-monospace,Consolas,monospace;font-size:11.5px;
  color:var(--dim);white-space:pre-wrap;word-break:break-all;margin:0;line-height:1.65}
.kvgrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px}
.kv{background:var(--bg-soft);border:1px solid var(--line);border-radius:11px;padding:12px 14px;min-width:0}
.kv b{display:block;font-size:11.5px;color:var(--dim);font-weight:500;margin-bottom:4px}
.kv span{font-size:13px;word-break:break-all;display:block}
.msg{font-size:13px;color:var(--dim);min-height:20px}
.msg.warn{color:var(--amber)}
.msg.err{color:var(--red)}
.topbar{display:flex;align-items:center;gap:10px;justify-content:flex-end;margin-bottom:22px}
.topbar .who2{font-size:12.5px;color:var(--dim)}
.iconbtn{width:34px;height:34px;padding:0;display:grid;place-items:center;border-radius:9px;
  background:var(--bg);border:1px solid var(--line);color:var(--dim)}
.iconbtn:hover{color:var(--text)}
#toast{position:fixed;right:22px;bottom:22px;display:flex;flex-direction:column;gap:9px;z-index:99}
.toast{background:var(--bg);border:1px solid var(--line);border-left:3px solid var(--text);
  padding:11px 17px;border-radius:10px;font-size:13px;box-shadow:0 10px 30px rgba(16,24,40,.12);
  max-width:390px;word-break:break-all;animation:in .18s ease}
.toast.ok{border-left-color:var(--green)}
.toast.err{border-left-color:var(--red)}
@keyframes in{from{transform:translateY(8px);opacity:0}to{transform:none;opacity:1}}
.spin{display:inline-block;width:12px;height:12px;border:2px solid rgba(128,128,128,.3);
  border-top-color:currentColor;border-radius:50%;animation:sp .7s linear infinite;
  vertical-align:-2px;margin-right:6px}
@keyframes sp{to{transform:rotate(360deg)}}
.login-wrap{max-width:390px;margin:15vh auto;text-align:center}
.login-wrap .h-display{font-size:30px;margin-bottom:26px}
@media(max-width:860px){
  aside{position:fixed;left:0;top:0;z-index:20;transform:translateX(-100%);transition:.2s}
  aside.open{transform:none}
  main{padding:22px 18px 50px}
  .h-display{font-size:30px}
  td.hide,th.hide{display:none}
}
</style>
</head>
<body>

<div class="layout">

  <aside id="side">
    <div class="brand"><i></i> QuickWork</div>
    <div class="nav-group">
      <div class="nav-group-label">概览</div>
      <div class="nav-item" data-page="dashboard"><span class="ico">▤</span> 仪表盘</div>
    </div>
    <div class="nav-group">
      <div class="nav-group-label">网关</div>
      <div class="nav-item" data-page="accounts"><span class="ico">◍</span> 账号池 <span class="nav-badge" id="navCount">0</span></div>
      <div class="nav-item" data-page="login"><span class="ico">⊕</span> OAuth 登录</div>
      <div class="nav-item" data-page="settings"><span class="ico">⚙</span> 配置面板</div>
    </div>
  </aside>

  <main>

    <!-- 登录 -->
    <div class="page" id="page-login-gate">
      <div class="login-wrap">
        <div class="h-display">QuickWork</div>
        <div class="card" style="text-align:left">
          <div class="field" style="margin-bottom:14px">
            <label>管理密码</label>
            <input id="pw" type="password" placeholder="输入密码进入控制台" autocomplete="current-password">
          </div>
          <button class="primary" id="loginBtn" style="width:100%">进入</button>
        </div>
        <div class="h-sub" style="margin-top:18px">QuickWork → OpenAI 兼容网关</div>
      </div>
    </div>

    <!-- 仪表盘 -->
    <div class="page" id="page-dashboard">
      <div class="topbar">
        <span class="who2" id="connState">已连接</span>
        <button class="iconbtn" id="themeBtn" title="切换主题">◐</button>
        <button class="iconbtn" id="outBtn" title="退出">⏻</button>
      </div>
      <h1 class="h-display" id="dashTitle">静候请求。</h1>
      <p class="h-sub"><b id="dashAddr">—</b> · <span id="dashAccounts">0 个账号</span></p>
      <div class="row" style="gap:10px">
        <button class="primary" data-goto="login">添加账号</button>
        <button class="ghost" data-goto="accounts">查看账号池 →</button>
      </div>

      <div class="section-title">号池</div>
      <h2 class="section-big">账号总览</h2>
      <div class="cards" style="margin-bottom:6px">
        <div class="stat"><div class="k">可用账号</div><div class="v" id="stReady">0</div><div class="bar"></div></div>
        <div class="stat dim"><div class="k">已停用</div><div class="v" id="stOff">0</div><div class="bar"></div></div>
        <div class="stat amber"><div class="k">24 小时内到期</div><div class="v" id="stSoon">0</div><div class="bar"></div></div>
        <div class="stat red"><div class="k">异常</div><div class="v" id="stErr">0</div><div class="bar"></div></div>
      </div>

      <div class="section-title">运行时</div>
      <h2 class="section-big">服务一览</h2>
      <div class="card">
        <div class="kvgrid" id="dashKv"></div>
      </div>
    </div>

    <!-- 账号池 -->
    <div class="page" id="page-accounts">
      <div class="topbar">
        <button class="ghost" id="refreshAll">刷新全部 Token</button>
        <button class="primary" data-goto="login">+ 添加账号</button>
      </div>
      <h1 class="h-display">账号池</h1>
      <p class="h-sub">请求按 ID 轮询；已停用的账号不参与分发，Token 到期前 10 分钟自动刷新。</p>
      <div class="card" style="padding:8px 10px">
        <table>
          <thead><tr>
            <th style="width:44px">ID</th><th>账号</th><th>状态</th><th class="hide">区域</th>
            <th>到期</th><th class="hide">用量</th><th class="hide">最近使用</th><th></th>
          </tr></thead>
          <tbody id="accBody"><tr><td colspan="8" class="empty">加载中…</td></tr></tbody>
        </table>
      </div>
    </div>

    <!-- OAuth 登录 -->
    <div class="page" id="page-login">
      <div class="topbar">
        <button class="ghost" id="cancelLogin">取消本次登录</button>
        <button class="primary" id="newLogin">开始登录</button>
      </div>
      <h1 class="h-display">OAuth 登录</h1>
      <p class="h-sub">新邮箱走 AWS Builder ID 注册流程，已有账号直接登录。浏览器打不开 127.0.0.1 是正常的，把地址栏那串贴回来即可。</p>

      <div class="steps">
        <div class="step" data-step="opening"><b>1</b> 启动登录</div>
        <div class="step" data-step="waiting"><b>2</b> 浏览器完成登录</div>
        <div class="step" data-step="done"><b>3</b> 账号入池</div>
      </div>

      <div class="card">
        <div class="field">
          <label id="urlLabel">① 注册 / 登录地址</label>
          <div class="row">
            <input id="loginUrl" class="mono" readonly placeholder="点右上角「开始登录」生成">
            <button class="primary" id="openUrl">打开</button>
            <button class="ghost" id="copyUrl">复制</button>
          </div>
          <div class="note">在这一步注册或登录，完成后浏览器会跳到 127.0.0.1（打不开属正常），把地址栏整串复制下来。</div>
        </div>

        <div class="field">
          <label>② 回调 URL（粘贴到这里）</label>
          <div class="row">
            <input id="cbUrl" class="mono" placeholder=".../callback?nonce=...  或  http://127.0.0.1:18080/?code=...&state=...">
            <button class="primary" id="submitCb">提交</button>
          </div>
          <div class="note">两步用的是同一个输入框，面板会按 URL 参数自动识别阶段。</div>
        </div>

        <div class="msg" id="loginMsg"></div>
      </div>

      <div class="card">
        <div class="card-h"><h3>进程日志</h3><span class="grow"></span><span class="hint">实时回显 CLI 输出</span></div>
        <pre class="log" id="loginLog"></pre>
      </div>
    </div>

    <!-- 配置 -->
    <div class="page" id="page-settings">
      <h1 class="h-display">配置面板</h1>
      <p class="h-sub">修改会写回 config.json，重启服务后生效。</p>

      <div class="card">
        <div class="card-h"><h3>运行参数</h3></div>
        <div class="kvgrid" id="cfgKv"></div>
      </div>

      <div class="card">
        <div class="card-h"><h3>可编辑项</h3><span class="grow"></span><button class="primary" id="saveCfg">保存</button></div>
        <div class="kvgrid" style="grid-template-columns:repeat(auto-fit,minmax(200px,1fr))">
          <div class="field" style="margin:0">
            <label>默认模型</label>
            <select id="cfgModel"></select>
          </div>
          <div class="field" style="margin:0">
            <label>时区</label>
            <input id="cfgTz">
          </div>
          <div class="field" style="margin:0">
            <label>语言</label>
            <input id="cfgLocale">
          </div>
          <div class="field" style="margin:0">
            <label>思考强度</label>
            <select id="cfgEffort">
              <option value="">默认</option><option value="off">off</option>
              <option value="low">low</option><option value="medium">medium</option>
              <option value="high">high</option>
            </select>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-h"><h3>接入信息</h3></div>
        <div class="kvgrid" id="apiKv"></div>
      </div>
    </div>

  </main>
</div>

<div id="toast"></div>

<script>
const $ = s => document.querySelector(s);
const $$ = s => Array.from(document.querySelectorAll(s));
let state = null, pollTimer = null, loginPoll = null, currentPage = 'dashboard';

function toast(msg, kind){
  const el = document.createElement('div');
  el.className = 'toast ' + (kind || '');
  el.textContent = msg;
  $('#toast').appendChild(el);
  setTimeout(() => el.remove(), 4200);
}

function esc(s){
  return String(s == null ? '' : s).replace(/[&<>"']/g, m =>
    ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
}

async function api(path, opts){
  opts = opts || {};
  if (opts.body) opts.headers = Object.assign({'Content-Type':'application/json'}, opts.headers || {});
  const res = await fetch(path, opts);
  let data = {};
  try { data = await res.json(); } catch (e) {}
  if (res.status === 401 && path !== '/api/session') { gate(); throw new Error('登录已失效'); }
  if (!res.ok || data.ok === false) throw new Error(data.error || ('HTTP ' + res.status));
  return data;
}

/* ---------- 路由 ---------- */
function goto(page){
  currentPage = page;
  $$('.page').forEach(p => p.classList.remove('on'));
  const el = $('#page-' + page);
  if (el) el.classList.add('on');
  $$('.nav-item').forEach(n => n.classList.toggle('active', n.dataset.page === page));
  $('#side').classList.remove('open');
  if (location.hash !== '#/' + page) history.replaceState(null, '', '#/' + page);
}
window.addEventListener('hashchange', () => {
  const p = (location.hash || '').replace('#/', '');
  if (p && $('#page-' + p)) goto(p);
});

function gate(){
  clearInterval(pollTimer); clearInterval(loginPoll);
  pollTimer = null; loginPoll = null;
  $$('.page').forEach(p => p.classList.remove('on'));
  $('#page-login-gate').classList.add('on');
}

/* ---------- 数据 ---------- */
function fmtTime(ts){
  if (!ts) return '—';
  const d = new Date(ts * 1000), diff = ts - Date.now()/1000;
  const p = n => String(n).padStart(2, '0');
  const base = p(d.getMonth()+1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
  if (Math.abs(diff) < 172800) {
    const m = Math.round(diff / 60);
    return base + ' <span style="color:var(--dim2);font-size:12px">' + (diff >= 0 ? '剩 ' + m + ' 分' : '过期 ' + Math.abs(m) + ' 分') + '</span>';
  }
  return base;
}

function statusDot(a){
  if (a.disabled) return '<span class="dot off">已停用</span>';
  if (a.last_error) return '<span class="dot err" title="' + esc(a.last_error) + '">异常</span>';
  if (a.expires_at && a.expires_at <= Date.now()/1000) return '<span class="dot err">已过期</span>';
  if (a.expires_at && a.expires_at - Date.now()/1000 < 86400) return '<span class="dot warn">即将到期</span>';
  return '<span class="dot ok">可用</span>';
}

async function load(){
  const r = await api('/api/state');
  state = r;
  renderAll();
}

function renderAll(){
  if (!state) return;
  const list = state.accounts || [];
  $('#navCount').textContent = list.length;
  $('#connState').textContent = 'v' + (state.config.app_version || '') + ' · 已连接';

  const now = Date.now() / 1000;
  const ready = list.filter(a => !a.disabled && !a.last_error).length;
  const off = list.filter(a => a.disabled).length;
  const soon = list.filter(a => !a.disabled && a.expires_at && a.expires_at > now && a.expires_at - now < 86400).length;
  const err = list.filter(a => !a.disabled && a.last_error).length;
  $('#stReady').textContent = ready;
  $('#stOff').textContent = off;
  $('#stSoon').textContent = soon;
  $('#stErr').textContent = err;

  $('#dashTitle').textContent = list.length ? ('号池就绪 ' + ready + ' / ' + list.length + '。') : '静候首个账号。';
  $('#dashAddr').textContent = state.config.addr;
  $('#dashAccounts').textContent = list.length + ' 个账号 · 可用 ' + state.enabled;

  $('#dashKv').innerHTML = [
    ['监听地址', state.config.addr],
    ['默认模型', state.config.default_model],
    ['上游 Provider', state.config.provider + ' / ' + state.config.build_stage],
    ['模型数量', (state.config.models || []).length],
    ['API Key 数', state.config.api_key_count],
    ['请求超时', state.config.request_timeout],
    ['自动刷新阈值', state.config.refresh_before],
    ['数据库', state.config.database],
  ].map(kvHtml).join('');

  $('#cfgKv').innerHTML = [
    ['监听地址', state.config.addr],
    ['数据库', state.config.database],
    ['上游 Provider', state.config.provider + ' / ' + state.config.build_stage],
    ['OAuth 回调端口', state.config.callback_port],
    ['OAuth 超时', state.config.oauth_timeout],
    ['请求超时', state.config.request_timeout],
    ['配置文件', state.config.config_path],
  ].map(kvHtml).join('');

  const ak = state.config.api_keys || [];
  $('#apiKv').innerHTML = [
    ['API Base', 'http://' + (state.config.addr || '').replace('0.0.0.0', location.hostname)],
    ['鉴权方式', 'Authorization: Bearer <key>'],
    ['API Key 1', ak[0] || '(未设置)'],
    ['模型名', (state.config.models || []).join(', ')],
  ].map(kvHtml).join('');

  const sel = $('#cfgModel');
  if (sel.options.length !== (state.config.models || []).length) {
    sel.innerHTML = (state.config.models || []).map(m => '<option value="' + esc(m) + '">' + esc(m) + '</option>').join('');
  }
  sel.value = state.config.default_model;
  if (document.activeElement !== $('#cfgTz')) $('#cfgTz').value = state.config.timezone || '';
  if (document.activeElement !== $('#cfgLocale')) $('#cfgLocale').value = state.config.locale || '';
  if (document.activeElement !== $('#cfgEffort')) $('#cfgEffort').value = state.config.thinking_effort || '';

  renderAccounts(list);
  renderLogin(state.login);
}

function kvHtml(pair){
  const k = pair[0], v = pair[1];
  return '<div class="kv"><b>' + esc(k) + '</b><span class="mono">' + esc(String(v == null ? '' : v)) + '</span></div>';
}

function renderAccounts(list){
  const body = $('#accBody');
  if (!list.length) {
    body.innerHTML = '<tr><td colspan="8" class="empty">还没有账号，去「OAuth 登录」添加一个</td></tr>';
    return;
  }
  body.innerHTML = list.map(a => {
    const name = a.display_name || a.email || a.account_name || ('#' + a.id);
    const sub = [a.email && a.display_name ? a.email : '', a.aws_account_id].filter(Boolean).join(' · ');
    const err = a.last_error ? '<div style="color:var(--red);font-size:11.5px;margin-top:3px">' + esc(a.last_error) + '</div>' : '';
    return '<tr>' +
      '<td class="mono" style="color:var(--dim)">' + a.id + '</td>' +
      '<td><div class="who">' + esc(name) + '</div><div class="who-sub mono">' + esc(sub) + '</div>' + err + '</td>' +
      '<td>' + statusDot(a) + '</td>' +
      '<td class="hide mono" style="color:var(--dim)">' + esc(a.region || '—') + '</td>' +
      '<td class="mono">' + fmtTime(a.expires_at) + '</td>' +
      '<td class="hide mono" style="color:var(--dim)">' + a.use_count + '</td>' +
      '<td class="hide mono" style="color:var(--dim)">' + (a.last_used_at ? fmtTime(a.last_used_at) : '—') + '</td>' +
      '<td class="act">' +
        '<button data-act="refresh" data-id="' + a.id + '">刷新</button>' +
        '<button data-act="' + (a.disabled ? 'enable' : 'disable') + '" data-id="' + a.id + '">' + (a.disabled ? '启用' : '停用') + '</button>' +
        '<button class="danger" data-act="delete" data-id="' + a.id + '">删除</button>' +
      '</td></tr>';
  }).join('');
}

/* ---------- 登录流程 ---------- */
function renderLogin(l){
  $$('.step').forEach(s => s.classList.toggle('on', !!l && l.step === s.dataset.step));
  if (!l) {
    $('#loginUrl').value = '';
    $('#loginMsg').textContent = '';
    $('#loginMsg').className = 'msg';
    $('#urlLabel').textContent = '① 注册 / 登录地址';
    $('#loginLog').textContent = '';
    $('#submitCb').disabled = true;
    $('#cancelLogin').disabled = true;
    return;
  }
  const url = l.login_url || '';
  const isAuthorize = url.indexOf('/authorize?') > 0;
  $('#loginUrl').value = url;
  $('#urlLabel').textContent = isAuthorize
    ? '② 授权地址（打开它完成授权）'
    : '① 注册 / 登录地址';
  $('#urlLabel').style.color = isAuthorize ? 'var(--amber)' : '';

  const box = $('#loginMsg');
  box.textContent = l.message || '';
  box.className = 'msg';
  if (l.step === 'failed') box.className = 'msg err';
  else if (/(失败|failed|mismatch|无法识别|error)/i.test(l.message || '')) box.className = 'msg warn';

  const log = $('#loginLog');
  log.textContent = (l.logs || []).slice(-60).join('\n');
  log.scrollTop = log.scrollHeight;

  const busy = l.step === 'opening' || l.step === 'waiting';
  $('#submitCb').disabled = !busy;
  $('#cancelLogin').disabled = !busy;
  $('#newLogin').disabled = busy;

  if (l.step === 'done') {
    clearInterval(loginPoll); loginPoll = null;
    setTimeout(() => { load().catch(() => {}); }, 500);
  }
}

function startLoginPoll(){
  clearInterval(loginPoll);
  loginPoll = setInterval(async () => {
    try {
      const r = await api('/api/login/current');
      renderLogin(r.login);
      if (!r.login || r.login.step === 'done' || r.login.step === 'failed') {
        clearInterval(loginPoll); loginPoll = null;
        if (r.login && r.login.step === 'failed') toast(r.login.message || '登录失败', 'err');
        if (r.login && r.login.step === 'done') toast('账号已加入号池', 'ok');
        await load();
      }
    } catch (e) { clearInterval(loginPoll); loginPoll = null; }
  }, 1200);
}

function startPoll(){
  clearInterval(pollTimer);
  pollTimer = setInterval(() => { load().catch(() => {}); }, 15000);
}

/* ---------- 事件 ---------- */
$('#loginBtn').onclick = async () => {
  const pw = $('#pw').value;
  if (!pw) return toast('请输入密码', 'err');
  try {
    await api('/api/session', {method:'POST', body: JSON.stringify({password: pw})});
    $('#pw').value = '';
    await load();
    goto('dashboard');
    startPoll();
  } catch (e) { toast(e.message, 'err'); }
};
$('#pw').addEventListener('keydown', e => { if (e.key === 'Enter') $('#loginBtn').click(); });

$('#outBtn').onclick = async () => {
  try { await api('/api/logout', {method:'POST'}); } catch (e) {}
  gate();
};
$('#themeBtn').onclick = () => {
  const cur = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = cur;
  try { localStorage.setItem('qw-theme', cur); } catch (e) {}
};

$$('.nav-item').forEach(n => n.onclick = () => goto(n.dataset.page));
$$('[data-goto]').forEach(b => b.onclick = () => goto(b.dataset.goto));

$('#newLogin').onclick = async () => {
  try {
    const r = await api('/api/login', {method:'POST', body: JSON.stringify({})});
    renderLogin(r.login);
    $('#cbUrl').value = '';
    startLoginPoll();
  } catch (e) { toast(e.message, 'err'); }
};

$('#submitCb').onclick = async () => {
  const url = $('#cbUrl').value.trim();
  if (!url) return toast('请粘贴回调 URL', 'err');
  try {
    const r = await api('/api/login/callback', {method:'POST', body: JSON.stringify({url: url})});
    renderLogin(r.login);
    $('#cbUrl').value = '';
    startLoginPoll();
    if (r.login && (r.login.login_url || '').indexOf('/authorize?') > 0) {
      toast('第一步完成，请打开上面新出现的授权地址继续', 'ok');
    }
  } catch (e) { toast(e.message, 'err'); }
};

$('#cancelLogin').onclick = async () => {
  try { await api('/api/login/cancel', {method:'POST'}); } catch (e) {}
  clearInterval(loginPoll); loginPoll = null;
  await load();
};

$('#copyUrl').onclick = async () => {
  const v = $('#loginUrl').value;
  if (!v) return;
  try { await navigator.clipboard.writeText(v); toast('已复制', 'ok'); }
  catch (e) { $('#loginUrl').select(); document.execCommand('copy'); toast('已复制', 'ok'); }
};
$('#openUrl').onclick = () => {
  const v = $('#loginUrl').value;
  if (v) window.open(v, '_blank', 'noopener');
};

$('#refreshAll').onclick = async (e) => {
  const btn = e.currentTarget;
  btn.disabled = true;
  btn.innerHTML = '<span class="spin"></span>刷新中';
  try {
    const r = await api('/api/refresh', {method:'POST'});
    toast('刷新完成：' + (r.total - r.failed) + ' 成功 / ' + r.failed + ' 失败', r.failed ? 'err' : 'ok');
    await load();
  } catch (e2) { toast(e2.message, 'err'); }
  btn.disabled = false;
  btn.textContent = '刷新全部 Token';
};

$('#saveCfg').onclick = async () => {
  try {
    await api('/api/config', {method:'POST', body: JSON.stringify({
      default_model: $('#cfgModel').value,
      timezone: $('#cfgTz').value,
      locale: $('#cfgLocale').value,
      thinking_effort: $('#cfgEffort').value
    })});
    toast('已保存，重启服务后生效', 'ok');
    await load();
  } catch (e) { toast(e.message, 'err'); }
};

$('#accBody').addEventListener('click', async (e) => {
  const btn = e.target.closest('button[data-act]');
  if (!btn) return;
  const act = btn.dataset.act, id = btn.dataset.id;
  if (act === 'delete' && !confirm('确定删除账号 #' + id + '？删除后需要重新登录添加。')) return;
  btn.disabled = true;
  try {
    if (act === 'delete') await api('/api/accounts/' + id, {method:'DELETE'});
    else await api('/api/accounts/' + id + '/' + act, {method:'POST'});
    toast('操作成功', 'ok');
    await load();
  } catch (err) { toast(err.message, 'err'); btn.disabled = false; }
});

/* ---------- 启动 ---------- */
try {
  const t = localStorage.getItem('qw-theme');
  if (t) document.documentElement.dataset.theme = t;
} catch (e) {}

(async () => {
  try {
    const r = await api('/api/session');
    if (r.authed) {
      await load();
      const p = (location.hash || '').replace('#/', '');
      goto(p && $('#page-' + p) ? p : 'dashboard');
      startPoll();
      return;
    }
  } catch (e) {}
  gate();
})();
</script>
</body>
</html>
`
