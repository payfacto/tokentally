// app.js — router, state, fetch helpers

export const $  = (sel, root=document) => root.querySelector(sel);
export const $$ = (sel, root=document) => Array.from(root.querySelectorAll(sel));

const COMPACT = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 });
export const fmt = {
  int:   n => (n ?? 0).toLocaleString(),
  compact: n => COMPACT.format(n ?? 0),
  usd:   n => n == null ? '—' : '$' + Number(n).toFixed(2),
  usd4:  n => n == null ? '—' : '$' + Number(n).toFixed(4),
  pct:   n => n == null ? '—' : (n * 100).toFixed(0) + '%',
  short: (s, n=80) => s == null ? '' : (s.length > n ? s.slice(0, n - 1) + '…' : s),
  htmlSafe: s => (s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
  modelClass: m => {
    const s = (m || '').toLowerCase();
    if (s.includes('opus'))   return 'opus';
    if (s.includes('sonnet')) return 'sonnet';
    if (s.includes('haiku'))  return 'haiku';
    return '';
  },
  modelShort: m => (m || '').replace('claude-', ''),
  ts: t => (t || '').slice(0, 16).replace('T', ' '),
};

const _apiMap = {
  '/api/overview': (qs) => window.go.App.GetOverview(qs.since||'', qs.until||''),
  '/api/prompts':  (qs) => window.go.App.GetPrompts(parseInt(qs.limit||50), qs.sort||'tokens'),
  '/api/projects': (qs) => window.go.App.GetProjects(qs.since||'', qs.until||''),
  '/api/sessions': (qs) => window.go.App.GetSessions(parseInt(qs.limit||20), qs.since||'', qs.until||''),
  '/api/tools':    (qs) => window.go.App.GetTools(qs.since||'', qs.until||''),
  '/api/daily':    (qs) => window.go.App.GetDaily(qs.since||'', qs.until||''),
  '/api/by-model': (qs) => window.go.App.GetByModel(qs.since||'', qs.until||''),
  '/api/skills':   (qs) => window.go.App.GetSkills(qs.since||'', qs.until||''),
  '/api/tips':     (_)  => window.go.App.GetTips(),
  '/api/plan':     (_)  => window.go.App.GetPlan(),
  '/api/scan':     (_)  => window.go.App.ScanNow(),
};

export async function api(path, opts) {
  const [base, search] = path.split('?');
  const qs = Object.fromEntries(new URLSearchParams(search||''));

  if (base.startsWith('/api/sessions/')) {
    const sid = base.split('/').pop();
    return window.go.App.GetSessionTurns(sid);
  }

  if (opts && opts.method === 'POST') {
    const body = JSON.parse(opts.body || '{}');
    if (base === '/api/tips/dismiss') return window.go.App.DismissTip(body.key||'');
    if (base === '/api/plan') return window.go.App.SetPlan(body.plan||'');
  }

  const handler = _apiMap[base];
  if (!handler) throw new Error(`No binding for ${base}`);
  return handler(qs);
}

export const state = { plan: 'api', pricing: null };

const ROUTES = {
  '/overview': () => import('/web/routes/overview.js'),
  '/prompts':  () => import('/web/routes/prompts.js'),
  '/sessions': () => import('/web/routes/sessions.js'),
  '/projects': () => import('/web/routes/projects.js'),
  '/skills':   () => import('/web/routes/skills.js'),
  '/tips':     () => import('/web/routes/tips.js'),
  '/settings': () => import('/web/routes/settings.js'),
};

function buildTopbar() {
  const wrap = document.createElement('header');
  wrap.className = 'topbar';
  wrap.innerHTML = `
    <div class="brand">Token<span style="color:var(--accent)">Tally</span></div>
    <nav>
      ${Object.keys(ROUTES).map(p => `<a href="#${p}" data-route="${p}">${p.slice(1)}</a>`).join('')}
    </nav>
    <div class="spacer"></div>
    <span class="pill" id="plan-pill">api</span>
    <span class="pill muted" title="Cmd/Ctrl+B blurs sensitive text">⌘B blur</span>
  `;
  document.body.prepend(wrap);
}

function setActiveTab(routeKey) {
  $$('header.topbar nav a').forEach(a => a.classList.toggle('active', a.dataset.route === routeKey));
}

async function render() {
  const hash = location.hash.replace(/^#/, '') || '/overview';
  const path = hash.split('?')[0];
  let key = path;
  if (path.startsWith('/sessions/')) key = '/sessions';
  setActiveTab(key);
  const loader = ROUTES[key] || ROUTES['/overview'];
  const mod = await loader();
  $('#app').innerHTML = '';
  try {
    await mod.default($('#app'));
  } catch (e) {
    $('#app').innerHTML = `<div class="card"><h2>Error</h2><pre>${fmt.htmlSafe(String(e.stack || e))}</pre></div>`;
  }
}

async function firstRun() {
  if (localStorage.getItem('td.plan-set')) return;
  const plans = Object.entries(state.pricing.plans);
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="modal">
      <h2>Welcome — pick your plan</h2>
      <p>This sets how costs are displayed. Change it later in Settings.</p>
      <select id="firstplan" style="width:100%">
        ${plans.map(([k,v]) => `<option value="${k}">${v.label}${v.monthly ? ` — $${v.monthly}/mo` : ''}</option>`).join('')}
      </select>
      <div class="actions">
        <div class="spacer"></div>
        <button class="primary" id="firstsave">Continue</button>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  await new Promise(res => $('#firstsave', overlay).addEventListener('click', async () => {
    const plan = $('#firstplan', overlay).value;
    await window.go.App.SetPlan(plan);
    localStorage.setItem('td.plan-set', '1');
    overlay.remove();
    res();
  }));
  state.plan = (await api('/api/plan')).plan;
}

async function boot() {
  buildTopbar();
  const planResp = await api('/api/plan');
  state.plan = planResp.plan;
  state.pricing = planResp.pricing;
  $('#plan-pill').textContent = state.plan;

  await firstRun();

  window.addEventListener('hashchange', render);
  await render();

  // Privacy blur (Cmd+B / Ctrl+B)
  window.addEventListener('keydown', e => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'b') {
      e.preventDefault();
      document.body.classList.toggle('privacy-on');
    }
  });

  // Live refresh via Wails events
  try {
    window.runtime.EventsOn('scan', () => render());
  } catch {}
}

boot();
