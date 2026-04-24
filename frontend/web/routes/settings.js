import { api, fmt, state, $ } from '/web/app.js';

const App = window.go.app.App;

const SERVICE_STATUS_DELAY = 1500;

const CURRENCIES = [
  { code: 'CAD', label: 'CAD — Canadian Dollar' },
  { code: 'USD', label: 'USD — US Dollar' },
  { code: 'EUR', label: 'EUR — Euro' },
  { code: 'GBP', label: 'GBP — British Pound' },
  { code: 'AUD', label: 'AUD — Australian Dollar' },
  { code: 'NZD', label: 'NZD — New Zealand Dollar' },
  { code: 'CHF', label: 'CHF — Swiss Franc' },
  { code: 'JPY', label: 'JPY — Japanese Yen' },
  { code: 'MXN', label: 'MXN — Mexican Peso' },
  { code: 'BRL', label: 'BRL — Brazilian Real' },
];

const PENCIL = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>`;
const TRASH  = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>`;
const PLUS   = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>`;
const RESET  = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-5.1L1 10"/></svg>`;
const REFRESH = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>`;

export default async function (root) {
  await renderAll(root);
}

async function renderAll(root) {
  const [planResp, models, plans, rates, apiKey] = await Promise.all([
    App.GetPlan(),
    App.GetPricingModels(),
    App.GetPricingPlans(),
    App.GetExchangeRates(),
    App.GetExchangeApiKey(),
  ]);
  plans.sort((a, b) => a.label.localeCompare(b.label));
  const currentPlan = planResp.plan || 'api';
  const currency = planResp.currency || 'CAD';
  const currentRate = rates[currency] || 1.0;

  root.innerHTML = `
    <div class="card" id="card-general">
      <h2>Settings</h2>

      <h3 style="margin-top:16px">Plan</h3>
      <p class="muted" style="margin:0 0 12px">Sets how cost is displayed. API mode shows pay-per-token rates. Subscription modes show what you actually pay each month.</p>
      <div class="flex" style="gap:10px;align-items:center">
        <select id="plan-sel">
          ${plans.map(p => `<option value="${fmt.htmlSafe(p.plan_key)}" ${p.plan_key === currentPlan ? 'selected' : ''}>${fmt.htmlSafe(p.label)}${p.monthly ? ` — ${fmt.money(p.monthly)}/mo` : ''}</option>`).join('')}
        </select>
        <button class="primary" id="save-plan">Save</button>
        <span id="plan-msg" class="muted" style="font-size:12px"></span>
      </div>

      <hr class="divider" style="margin:20px 0">

      <h3>Currency &amp; Exchange Rate</h3>
      <p class="muted" style="margin:0 0 12px">All pricing is stored in USD. Enter the exchange rate so costs display in your currency.</p>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;max-width:480px">
        <div>
          <label class="form-label">Currency</label>
          <select id="currency-sel" style="width:100%">
            ${CURRENCIES.map(c => `<option value="${c.code}" ${c.code === currency ? 'selected' : ''}>${c.label}</option>`).join('')}
          </select>
        </div>
        <div>
          <label class="form-label">Exchange rate (1 USD = ?)</label>
          <input id="currency-rate" type="number" step="0.0001" min="0" class="form-input" value="${currentRate.toFixed(4)}">
        </div>
      </div>
      <div class="flex" style="gap:10px;margin-top:10px;align-items:center">
        <button class="primary" id="save-currency">Save</button>
        <span id="currency-msg" class="muted" style="font-size:12px"></span>
      </div>

      <hr class="divider" style="margin:20px 0">

      <h3>Exchange Rate API</h3>
      <p class="muted" style="margin:0 0 12px">Connect to <strong>exchangerate-api.com</strong> to fetch live rates with one click.
        <a href="https://www.exchangerate-api.com/" target="_blank" style="margin-left:4px">Sign up for a free account →</a>
      </p>
      <div style="display:grid;grid-template-columns:1fr auto auto;gap:10px;max-width:600px;align-items:flex-end">
        <div>
          <label class="form-label">API key</label>
          <input id="exchange-api-key" type="password" class="form-input" value="${fmt.htmlSafe(apiKey || '')}" placeholder="Your exchangerate-api.com key">
        </div>
        <button id="save-api-key" style="white-space:nowrap">Save key</button>
        <button id="btn-refresh-rates" style="display:flex;align-items:center;gap:5px;white-space:nowrap">${REFRESH} Refresh rates</button>
      </div>
      <span id="rates-msg" class="muted" style="font-size:12px;display:block;margin-top:6px"></span>
    </div>

    <div class="card" id="card-models" style="margin-top:16px">
      <div class="flex" style="align-items:center;margin-bottom:4px">
        <h2 style="margin:0">Model Rates</h2>
        <span class="spacer"></span>
        <button id="btn-reset" title="Reset to defaults" style="display:flex;align-items:center;gap:5px;background:transparent;border:1px solid var(--border);color:var(--muted);padding:5px 10px;border-radius:6px;cursor:pointer;font-size:12px">${RESET} Reset to defaults</button>
        <button id="btn-add-model" style="display:flex;align-items:center;gap:5px;margin-left:8px">${PLUS} Add model</button>
      </div>
      <p class="muted" style="margin:0 0 12px;font-size:12px">Rates per 1M tokens, USD. All pricing data is sourced from Anthropic's published rates.</p>
      ${renderModelsTable(models)}
    </div>

    <div class="card" id="card-plans" style="margin-top:16px">
      <div class="flex" style="align-items:center;margin-bottom:12px">
        <h2 style="margin:0">Plans</h2>
        <span class="spacer"></span>
        <button id="btn-add-plan" style="display:flex;align-items:center;gap:5px">${PLUS} Add plan</button>
      </div>
      ${renderPlansTable(plans)}
    </div>

    <div class="card" id="service-card" style="margin-top:16px">
      <h2>Windows Service</h2>
      <p class="muted" style="font-size:13px">The background scanner runs as a Windows service, keeping data up to date even when the dashboard is closed.</p>
      <div id="svc-status" style="margin:12px 0;font-size:13px">Checking…</div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button id="btn-install" class="primary">Install Service</button>
        <button id="btn-uninstall" style="background:var(--bad);color:#fff;border:none;padding:6px 14px;border-radius:6px;cursor:pointer">Uninstall Service</button>
      </div>
      <p class="muted" style="font-size:11px;margin-top:8px">Requires administrator rights (UAC prompt will appear).</p>
    </div>`;

  bindGeneral(root, plans, currentPlan, rates, currency);
  bindModels(root, models);
  bindPlans(root, plans);
  bindService(root);
}

function renderModelsTable(models) {
  if (!models.length) return '<p class="muted">No model rates configured.</p>';
  return `<div style="overflow-x:auto">
    <table>
      <thead>
        <tr>
          <th>model</th><th>tier</th>
          <th class="num">input</th><th class="num">output</th>
          <th class="num">cache read</th><th class="num">cache 5m</th><th class="num">cache 1h</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        ${models.map(m => `
          <tr>
            <td><span class="badge ${fmt.htmlSafe(m.tier)}">${fmt.htmlSafe(m.model_name)}</span></td>
            <td class="muted" style="font-size:12px">${fmt.htmlSafe(m.tier)}</td>
            <td class="num">${Number(m.input).toFixed(2)}</td>
            <td class="num">${Number(m.output).toFixed(2)}</td>
            <td class="num">${Number(m.cache_read).toFixed(2)}</td>
            <td class="num">${Number(m.cache_create_5m).toFixed(2)}</td>
            <td class="num">${Number(m.cache_create_1h).toFixed(2)}</td>
            <td style="white-space:nowrap;text-align:right">
              <button class="icon-btn edit-model" data-name="${fmt.htmlSafe(m.model_name)}" title="Edit">${PENCIL}</button>
              <button class="icon-btn del-model" data-name="${fmt.htmlSafe(m.model_name)}" title="Delete" style="color:var(--bad)">${TRASH}</button>
            </td>
          </tr>`).join('')}
      </tbody>
    </table>
  </div>`;
}

function renderPlansTable(plans) {
  if (!plans.length) return '<p class="muted">No plans configured.</p>';
  return `<table>
    <thead>
      <tr><th>key</th><th>label</th><th class="num">monthly (USD)</th><th></th></tr>
    </thead>
    <tbody>
      ${plans.map(p => `
        <tr>
          <td class="mono" style="font-size:12px">${fmt.htmlSafe(p.plan_key)}</td>
          <td>${fmt.htmlSafe(p.label)}</td>
          <td class="num">${p.monthly > 0 ? fmt.money(p.monthly) : p.label.toLowerCase().includes('pay-per-token') ? '<span class="muted">pay-per-token</span>' : fmt.money(0)}</td>
          <td style="white-space:nowrap;text-align:right">
            <button class="icon-btn edit-plan" data-key="${fmt.htmlSafe(p.plan_key)}" title="Edit">${PENCIL}</button>
            <button class="icon-btn del-plan" data-key="${fmt.htmlSafe(p.plan_key)}" title="Delete" style="color:var(--bad)">${TRASH}</button>
          </td>
        </tr>`).join('')}
    </tbody>
  </table>`;
}

function flash(el, text, color = 'var(--good)') {
  if (!el) return;
  el.textContent = text;
  el.style.color = color;
  setTimeout(() => { el.textContent = ''; }, 2500);
}

function bindGeneral(root, plans, currentPlan, rates, currentCurrency) {
  root.querySelector('#save-plan').addEventListener('click', async () => {
    const plan = root.querySelector('#plan-sel').value;
    await App.SetPlan(plan);
    state.plan = plan;
    document.getElementById('plan-pill').textContent = plan;
    flash(root.querySelector('#plan-msg'), 'Saved.');
  });

  const currencySel = root.querySelector('#currency-sel');
  const rateInput   = root.querySelector('#currency-rate');

  currencySel.addEventListener('change', () => {
    const cur = currencySel.value;
    rateInput.value = (rates[cur] || 1.0).toFixed(4);
  });

  root.querySelector('#save-currency').addEventListener('click', async () => {
    const cur  = currencySel.value;
    const rate = parseFloat(rateInput.value) || 1.0;
    await App.SetCurrency(cur);
    await App.SetExchangeRate(cur, rate);
    state.currency     = cur;
    state.exchangeRate = rate;
    flash(root.querySelector('#currency-msg'), 'Saved.');
  });

  root.querySelector('#save-api-key').addEventListener('click', async () => {
    const key = root.querySelector('#exchange-api-key').value.trim();
    await App.SetExchangeApiKey(key);
    flash(root.querySelector('#rates-msg'), 'API key saved.');
  });

  root.querySelector('#btn-refresh-rates').addEventListener('click', async () => {
    const msg = root.querySelector('#rates-msg');
    msg.textContent = 'Fetching live rates…';
    msg.style.color = 'var(--muted)';
    try {
      const keyVal = root.querySelector('#exchange-api-key').value.trim();
      if (keyVal) await App.SetExchangeApiKey(keyVal);
      const updated = await App.RefreshExchangeRates();
      Object.assign(rates, updated);
      const cur = currencySel.value;
      if (updated[cur] != null) {
        rateInput.value = updated[cur].toFixed(4);
      }
      flash(msg, 'Rates updated from exchangerate-api.com.');
    } catch (e) {
      msg.textContent = 'Error: ' + (e.message || String(e));
      msg.style.color = 'var(--bad)';
    }
  });
}

function bindModels(root, models) {
  root.querySelector('#btn-add-model').addEventListener('click', () => {
    modelModal('Add model', {}, async (data) => {
      await App.UpsertPricingModel(data.model_name, data.tier, data.input, data.output, data.cache_read, data.cache_create_5m, data.cache_create_1h);
      await renderAll(root.parentElement || root);
    });
  });

  root.querySelector('#btn-reset').addEventListener('click', async () => {
    if (!confirm('Reset all model rates and plans to the built-in defaults? This cannot be undone.')) return;
    await App.ResetPricingToDefaults();
    await renderAll(root.parentElement || root);
  });

  root.querySelectorAll('.edit-model').forEach(btn => {
    const name = btn.dataset.name;
    const m = models.find(x => x.model_name === name) || {};
    btn.addEventListener('click', () => {
      modelModal('Edit model', m, async (data) => {
        await App.UpsertPricingModel(data.model_name, data.tier, data.input, data.output, data.cache_read, data.cache_create_5m, data.cache_create_1h);
        await renderAll(root.parentElement || root);
      });
    });
  });

  root.querySelectorAll('.del-model').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (!confirm(`Delete model "${btn.dataset.name}"?`)) return;
      await App.DeletePricingModel(btn.dataset.name);
      await renderAll(root.parentElement || root);
    });
  });
}

function bindPlans(root, plans) {
  root.querySelector('#btn-add-plan').addEventListener('click', () => {
    planModal('Add plan', {}, async (data) => {
      await App.UpsertPricingPlan(data.plan_key, data.label, data.monthly);
      await renderAll(root.parentElement || root);
    });
  });

  root.querySelectorAll('.edit-plan').forEach(btn => {
    const key = btn.dataset.key;
    const p = plans.find(x => x.plan_key === key) || {};
    btn.addEventListener('click', () => {
      planModal('Edit plan', p, async (data) => {
        await App.UpsertPricingPlan(data.plan_key, data.label, data.monthly);
        await renderAll(root.parentElement || root);
      });
    });
  });

  root.querySelectorAll('.del-plan').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (!confirm(`Delete plan "${btn.dataset.key}"?`)) return;
      await App.DeletePricingPlan(btn.dataset.key);
      await renderAll(root.parentElement || root);
    });
  });
}

function bindService(root) {
  async function refreshServiceStatus() {
    const el = root.querySelector('#svc-status');
    if (!el) return;
    try {
      const status = await App.GetServiceStatus();
      if (!status.installed) {
        el.innerHTML = '<span style="color:var(--bad)">● Not installed</span>';
      } else {
        const color = status.state === 'running' ? 'var(--good)' : 'var(--muted)';
        el.innerHTML = `<span style="color:${color}">● ${status.state}</span>`;
      }
    } catch {
      el.innerHTML = '<span style="color:var(--muted)">● Status unavailable</span>';
    }
  }
  refreshServiceStatus();

  root.querySelector('#btn-install')?.addEventListener('click', async () => {
    await App.InstallService().catch(() => {});
    setTimeout(refreshServiceStatus, SERVICE_STATUS_DELAY);
  });
  root.querySelector('#btn-uninstall')?.addEventListener('click', async () => {
    await App.UninstallService().catch(() => {});
    setTimeout(refreshServiceStatus, SERVICE_STATUS_DELAY);
  });
}

function modelModal(title, initial, onSave) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  const nameReadonly = initial.model_name ? 'readonly style="opacity:0.6"' : '';
  overlay.innerHTML = `
    <div class="modal" style="max-width:560px;width:90vw">
      <h3 style="margin:0 0 16px;font-size:15px">${title}</h3>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">
        <div style="grid-column:1/-1">
          <label class="form-label">Model name</label>
          <input id="m-name" class="form-input" value="${fmt.htmlSafe(initial.model_name || '')}" ${nameReadonly} placeholder="claude-sonnet-4-6">
        </div>
        <div style="grid-column:1/-1">
          <label class="form-label">Tier (opus / sonnet / haiku)</label>
          <input id="m-tier" class="form-input" value="${fmt.htmlSafe(initial.tier || '')}" placeholder="sonnet">
        </div>
        <div>
          <label class="form-label">Input (per 1M, USD)</label>
          <input id="m-input" type="number" step="0.01" min="0" class="form-input" value="${Number(initial.input || 0).toFixed(2)}">
        </div>
        <div>
          <label class="form-label">Output (per 1M, USD)</label>
          <input id="m-output" type="number" step="0.01" min="0" class="form-input" value="${Number(initial.output || 0).toFixed(2)}">
        </div>
        <div>
          <label class="form-label">Cache read (per 1M, USD)</label>
          <input id="m-cacheread" type="number" step="0.01" min="0" class="form-input" value="${Number(initial.cache_read || 0).toFixed(2)}">
        </div>
        <div>
          <label class="form-label">Cache 5m (per 1M, USD)</label>
          <input id="m-cache5m" type="number" step="0.01" min="0" class="form-input" value="${Number(initial.cache_create_5m || 0).toFixed(2)}">
        </div>
        <div>
          <label class="form-label">Cache 1h (per 1M, USD)</label>
          <input id="m-cache1h" type="number" step="0.01" min="0" class="form-input" value="${Number(initial.cache_create_1h || 0).toFixed(2)}">
        </div>
      </div>
      <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
        <button id="m-cancel">Cancel</button>
        <button id="m-save" class="primary">Save</button>
      </div>
    </div>`;

  overlay.querySelector('#m-cancel').addEventListener('click', () => overlay.remove());
  overlay.addEventListener('click', e => { if (e.target === overlay) overlay.remove(); });

  overlay.querySelector('#m-save').addEventListener('click', async () => {
    const name = overlay.querySelector('#m-name').value.trim();
    if (!name) { overlay.querySelector('#m-name').focus(); return; }
    await onSave({
      model_name:      name,
      tier:            overlay.querySelector('#m-tier').value.trim(),
      input:           parseFloat(overlay.querySelector('#m-input').value) || 0,
      output:          parseFloat(overlay.querySelector('#m-output').value) || 0,
      cache_read:      parseFloat(overlay.querySelector('#m-cacheread').value) || 0,
      cache_create_5m: parseFloat(overlay.querySelector('#m-cache5m').value) || 0,
      cache_create_1h: parseFloat(overlay.querySelector('#m-cache1h').value) || 0,
    });
    overlay.remove();
  });

  document.body.appendChild(overlay);
  overlay.querySelector('#m-name').focus();
}

function planModal(title, initial, onSave) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  const keyReadonly = initial.plan_key ? 'readonly style="opacity:0.6"' : '';
  overlay.innerHTML = `
    <div class="modal" style="max-width:400px;width:90vw">
      <h3 style="margin:0 0 16px;font-size:15px">${title}</h3>
      <div style="display:flex;flex-direction:column;gap:12px">
        <div>
          <label class="form-label">Plan key (unique identifier)</label>
          <input id="p-key" class="form-input" value="${fmt.htmlSafe(initial.plan_key || '')}" ${keyReadonly} placeholder="max">
        </div>
        <div>
          <label class="form-label">Label</label>
          <input id="p-label" class="form-input" value="${fmt.htmlSafe(initial.label || '')}" placeholder="Max">
        </div>
        <div>
          <label class="form-label">Monthly cost in USD (0 = free or pay-per-token)</label>
          <input id="p-monthly" type="number" step="0.01" min="0" class="form-input" value="${Number(initial.monthly || 0).toFixed(2)}">
        </div>
      </div>
      <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
        <button id="p-cancel">Cancel</button>
        <button id="p-save" class="primary">Save</button>
      </div>
    </div>`;

  overlay.querySelector('#p-cancel').addEventListener('click', () => overlay.remove());
  overlay.addEventListener('click', e => { if (e.target === overlay) overlay.remove(); });

  overlay.querySelector('#p-save').addEventListener('click', async () => {
    const key   = overlay.querySelector('#p-key').value.trim();
    const label = overlay.querySelector('#p-label').value.trim();
    if (!key)   { overlay.querySelector('#p-key').focus(); return; }
    if (!label) { overlay.querySelector('#p-label').focus(); return; }
    await onSave({
      plan_key: key,
      label,
      monthly: parseFloat(overlay.querySelector('#p-monthly').value) || 0,
    });
    overlay.remove();
  });

  document.body.appendChild(overlay);
  overlay.querySelector('#p-key').focus();
}
