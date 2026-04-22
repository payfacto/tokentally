import { api, state, $ } from '/web/app.js';

export default async function (root) {
  const cur = await api('/api/plan');
  const plans = Object.entries(cur.pricing?.plans || {});
  const models = Object.entries(cur.pricing?.models || {});

  root.innerHTML = `
    <div class="card">
      <h2>Settings</h2>
      <h3 style="margin-top:16px">Plan</h3>
      <p class="muted" style="margin:0 0 12px">Sets how cost is displayed. API mode shows pay-per-token rates. Subscription modes show what you actually pay each month.</p>
      <div class="flex">
        <select id="plan">
          ${plans.map(([k,v]) => `<option value="${k}" ${k===cur.plan?'selected':''}>${v.label}${v.monthly ? ` — $${v.monthly}/mo` : ''}</option>`).join('')}
        </select>
        <button class="primary" id="save">Save</button>
        <span id="msg" class="muted"></span>
      </div>

      <hr class="divider">

      <h3>Pricing table</h3>
      <p class="muted" style="margin:0 0 12px">Edit <code>pricing.json</code> in the install directory to change rates. Reload the page after editing.</p>
      <table>
        <thead><tr><th>model</th><th class="num">input</th><th class="num">output</th><th class="num">cache read</th><th class="num">cache 5m</th><th class="num">cache 1h</th></tr></thead>
        <tbody>
          ${models.map(([k,v]) => `
            <tr><td><span class="badge ${v.tier || ''}">${k}</span></td>
              <td class="num">$${(v.input||0).toFixed(2)}</td>
              <td class="num">$${(v.output||0).toFixed(2)}</td>
              <td class="num">$${(v.cache_read||0).toFixed(2)}</td>
              <td class="num">$${(v.cache_create_5m||0).toFixed(2)}</td>
              <td class="num">$${(v.cache_create_1h||0).toFixed(2)}</td>
            </tr>`).join('')}
        </tbody>
      </table>
      <p class="muted" style="margin-top:8px;font-size:11px">Rates per 1M tokens, USD.</p>

    </div>

    <div class="card" id="service-card" style="margin-top:16px">
      <h2>Windows Service</h2>
      <p style="color:var(--muted);font-size:13px">
        The background scanner runs as a Windows service, keeping data up to date even when the dashboard is closed.
      </p>
      <div id="svc-status" style="margin:12px 0;font-size:13px">Checking...</div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button id="btn-install" class="primary">Install Service</button>
        <button id="btn-uninstall" style="background:var(--bad);color:#fff;border:none;padding:6px 14px;border-radius:6px;cursor:pointer">Uninstall Service</button>
      </div>
      <p style="color:var(--muted);font-size:11px;margin-top:8px">Requires administrator rights (UAC prompt will appear).</p>
    </div>`;

  $('#save').addEventListener('click', async () => {
    const plan = $('#plan').value;
    await window.go.app.App.SetPlan(plan);
    state.plan = plan;
    document.getElementById('plan-pill').textContent = plan;
    $('#msg').textContent = 'Saved.';
    $('#msg').style.color = 'var(--good)';
  });

  async function refreshServiceStatus() {
    const el = document.getElementById('svc-status');
    if (!el) return;
    try {
      const status = await window.go.app.App.GetServiceStatus();
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

  document.getElementById('btn-install')?.addEventListener('click', async () => {
    await window.go.app.App.InstallService().catch(() => {});
    setTimeout(refreshServiceStatus, 1500);
  });
  document.getElementById('btn-uninstall')?.addEventListener('click', async () => {
    await window.go.app.App.UninstallService().catch(() => {});
    setTimeout(refreshServiceStatus, 1500);
  });
}
