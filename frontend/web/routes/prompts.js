import { api, fmt, SESSION_ID_PREFIX } from '/web/app.js';

const COPY_ICON = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>`;
const USER_ICON = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/></svg>`;

function prettyPrompt(text) {
  if (!text) return '';
  if (!/<[a-z_][a-z_0-9]*[^>]*>/.test(text)) return text;
  return text
    .replace(/(<[a-z_][a-z_0-9]*(?:\s[^>]*)?>)/g, '\n$1\n')
    .replace(/(<\/[a-z_][a-z_0-9]*>)/g, '\n$1\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

const SORTS = [
  { key: 'tokens', label: 'Most tokens' },
  { key: 'recent', label: 'Most recent' },
];

function readSort() {
  const q = (location.hash.split('?')[1] || '');
  const m = /(?:^|&)sort=([^&]+)/.exec(q);
  const k = m && decodeURIComponent(m[1]);
  return SORTS.find(s => s.key === k) || SORTS[0];
}

function writeSort(key) {
  const base = (location.hash.replace(/^#/, '').split('?')[0]) || '/prompts';
  location.hash = '#' + base + '?sort=' + encodeURIComponent(key);
}

export default async function (root) {
  const sort = readSort();
  const rows = await api('/api/prompts?limit=100&sort=' + encodeURIComponent(sort.key));

  const sortTabs = `
    <div class="range-tabs" role="tablist">
      ${SORTS.map(s => `<button data-sort="${s.key}" class="${s.key === sort.key ? 'active' : ''}">${s.label}</button>`).join('')}
    </div>`;

  const subtitle = sort.key === 'recent'
    ? 'Your latest prompts and the assistant turn each one triggered. Click a row to see the full prompt.'
    : 'The prompts that cost the most tokens. Click a row to see the full prompt.';

  root.innerHTML = `
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Prompts</h2>
      <div class="spacer"></div>
      ${sortTabs}
    </div>

    <div class="card">
      <p class="muted" style="margin:0 0 14px">${subtitle}</p>
      <table id="prompts">
        <thead><tr>
          <th>${sort.key === 'recent' ? 'when' : 'cache cost'}</th>
          <th></th>
          <th>prompt</th>
          <th>model</th>
          <th class="num">tokens</th>
          <th class="num">cache rd</th>
          <th>session</th>
        </tr></thead>
        <tbody>
          ${rows.map((r,i) => `
            <tr data-i="${i}" style="cursor:pointer">
              <td class="${sort.key === 'recent' ? 'mono' : 'num mono'}">${sort.key === 'recent' ? fmt.ts(r.timestamp) : fmt.money4(r.estimated_cost_usd)}</td>
              <td style="color:var(--muted)">${USER_ICON}</td>
              <td>${fmt.htmlSafe(fmt.short(r.prompt_text, 110))}</td>
              <td><span class="badge ${fmt.modelClass(r.model)}">${fmt.htmlSafe(fmt.modelShort(r.model))}</span></td>
              <td class="num">${fmt.int(r.billable_tokens)}</td>
              <td class="num">${fmt.int(r.cache_read_tokens)}</td>
              <td><a href="#/sessions/${encodeURIComponent(r.session_id)}" class="mono" onclick="event.stopPropagation()">${fmt.htmlSafe(r.session_id.slice(0, SESSION_ID_PREFIX))}…</a></td>
            </tr>`).join('') || '<tr><td colspan="7" class="muted">no prompts yet</td></tr>'}
        </tbody>
      </table>
    </div>
  `;

  root.querySelectorAll('.range-tabs button').forEach(btn => {
    btn.addEventListener('click', () => writeSort(btn.dataset.sort));
  });

  root.querySelectorAll('#prompts tbody tr').forEach(tr => {
    tr.addEventListener('click', () => {
      const r = rows[Number(tr.dataset.i)];
      const overlay = document.createElement('div');
      overlay.className = 'modal-overlay';
      overlay.innerHTML = `
        <div class="modal" style="max-width:760px;width:90vw;max-height:80vh;display:flex;flex-direction:column">
          <div style="display:flex;align-items:center;margin-bottom:12px;flex-shrink:0">
            <strong style="font-size:14px">Prompt detail</strong>
            <span class="spacer"></span>
            <span class="badge ${fmt.modelClass(r.model)}">${fmt.htmlSafe(fmt.modelShort(r.model))}</span>
          </div>
          <div style="position:relative;flex:1;overflow:hidden;display:flex;flex-direction:column">
            <pre id="prompt-pre" style="font-family:var(--mono);white-space:pre-wrap;word-break:break-word;background:var(--bg);padding:12px;padding-bottom:36px;border-radius:6px;border:1px solid var(--border);font-size:12px;line-height:1.5;overflow-y:auto;flex:1;margin:0">${fmt.htmlSafe(prettyPrompt(r.prompt_text || ''))}</pre>
            <button id="copy-btn" title="Copy to clipboard" style="position:absolute;bottom:8px;right:8px;background:transparent;border:1px solid var(--border);border-radius:4px;padding:5px 7px;cursor:pointer;color:var(--muted);display:flex;align-items:center;justify-content:center;line-height:1;transition:color 120ms,border-color 120ms">${COPY_ICON}</button>
          </div>
          <div class="flex" style="margin-top:12px;flex-wrap:wrap;gap:14px;flex-shrink:0">
            <span class="muted">${fmt.ts(r.timestamp)}</span>
            <span class="muted">${fmt.int(r.billable_tokens)} billable · ${fmt.int(r.cache_read_tokens)} cache rd · ~${fmt.money4(r.estimated_cost_usd)} cache cost</span>
            <span class="spacer"></span>
            <a href="#/sessions/${encodeURIComponent(r.session_id)}" onclick="this.closest('.modal-overlay').remove()">Open session →</a>
          </div>
        </div>`;
      overlay.addEventListener('click', e => { if (e.target === overlay) overlay.remove(); });
      document.body.appendChild(overlay);

      overlay.querySelector('#copy-btn').addEventListener('click', async () => {
        await navigator.clipboard.writeText(prettyPrompt(r.prompt_text || ''));
        const btn = overlay.querySelector('#copy-btn');
        btn.style.color = 'var(--good)';
        btn.style.borderColor = 'var(--good)';
        setTimeout(() => { btn.style.color = ''; btn.style.borderColor = ''; }, 1200);
      });
    });
  });
}
