import { api, fmt, SESSION_ID_PREFIX } from '/web/app.js';

const TYPE_ICONS = {
  user:       `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/></svg>`,
  assistant:  `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2l2.4 7.4H22l-6.2 4.5 2.4 7.4L12 17l-6.2 4.3 2.4-7.4L2 9.4h7.6z"/></svg>`,
  attachment: `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>`,
  tool:       `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>`,
  summary:    `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>`,
};

const COPY_ICON = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>`;

function typeCell(type) {
  const icon = TYPE_ICONS[type] || `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/></svg>`;
  return `<span style="display:inline-flex;align-items:center;gap:5px;color:var(--text)">${icon}${type}</span>`;
}

function prettyPrompt(text) {
  if (!text) return '';
  if (!/<[a-z_][a-z_0-9]*[^>]*>/.test(text)) return text;
  return text
    .replace(/(<[a-z_][a-z_0-9]*(?:\s[^>]*)?>)/g, '\n$1\n')
    .replace(/(<\/[a-z_][a-z_0-9]*>)/g, '\n$1\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

function turnContent(t) {
  if (t.prompt_text) return { text: prettyPrompt(t.prompt_text), label: t.type === 'attachment' ? 'Hook Output' : 'Prompt' };
  if (t.tool_calls_json) {
    try {
      const tools = JSON.parse(t.tool_calls_json);
      const lines = tools.map(x => {
        const input = x.input ? '\n' + JSON.stringify(x.input, null, 2) : '';
        return `${x.name}${input}`;
      });
      return { text: lines.join('\n\n'), label: 'Tool calls' };
    } catch { /* fall through */ }
  }
  return null;
}

function showTurnDetail(t) {
  const content = turnContent(t);
  if (!content) return;

  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="modal" style="max-width:760px;width:90vw;max-height:80vh;display:flex;flex-direction:column">
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:12px;flex-shrink:0">
        <strong style="font-size:14px">${content.label}</strong>
        <span class="muted" style="font-size:12px">${typeCell(t.type)}</span>
        ${t.model ? `<span class="badge ${fmt.modelClass(t.model)}">${fmt.htmlSafe(fmt.modelShort(t.model))}</span>` : ''}
      </div>
      <div style="position:relative;flex:1;overflow:hidden;display:flex;flex-direction:column">
        <pre id="turn-pre" style="font-family:var(--mono);white-space:pre-wrap;word-break:break-word;background:var(--bg);padding:12px;padding-bottom:36px;border-radius:6px;border:1px solid var(--border);font-size:12px;line-height:1.5;overflow-y:auto;flex:1;margin:0">${fmt.htmlSafe(content.text)}</pre>
        <button id="copy-btn" title="Copy to clipboard" style="position:absolute;bottom:8px;right:8px;background:transparent;border:1px solid var(--border);border-radius:4px;padding:5px 7px;cursor:pointer;color:var(--muted);display:flex;align-items:center;justify-content:center;line-height:1;transition:color 120ms,border-color 120ms">${COPY_ICON}</button>
      </div>
      <div class="flex" style="margin-top:12px;flex-wrap:wrap;gap:14px;flex-shrink:0">
        <span class="muted">${fmt.ts(t.timestamp)}</span>
        <span class="muted">${fmt.int(t.input_tokens)} in · ${fmt.int(t.output_tokens)} out · ${fmt.int(t.cache_read_tokens)} cache rd</span>
      </div>
    </div>`;

  overlay.addEventListener('click', e => { if (e.target === overlay) overlay.remove(); });
  document.body.appendChild(overlay);

  overlay.querySelector('#copy-btn').addEventListener('click', async () => {
    await navigator.clipboard.writeText(content.text);
    const btn = overlay.querySelector('#copy-btn');
    btn.style.color = 'var(--good)';
    btn.style.borderColor = 'var(--good)';
    setTimeout(() => { btn.style.color = ''; btn.style.borderColor = ''; }, 1200);
  });
}

export default async function (root) {
  const id = decodeURIComponent(location.hash.split('/')[2] || '');
  if (!id) return renderList(root);
  return renderSession(root, id);
}

async function renderList(root) {
  const list = await api('/api/sessions?limit=100');
  root.innerHTML = `
    <div class="card">
      <h2>Sessions</h2>
      <table>
        <thead><tr><th>started</th><th>project</th><th class="num">turns</th><th class="num">tokens</th><th>session</th></tr></thead>
        <tbody>
          ${list.map(s => `
            <tr>
              <td class="mono">${fmt.ts(s.started)}</td>
              <td title="${fmt.htmlSafe(s.project_slug)}">${fmt.htmlSafe(s.project_name || s.project_slug)}</td>
              <td class="num">${fmt.int(s.turns)}</td>
              <td class="num">${fmt.int(s.tokens)}</td>
              <td><a href="#/sessions/${encodeURIComponent(s.session_id)}" class="mono">${fmt.htmlSafe(s.session_id.slice(0, SESSION_ID_PREFIX))}…</a></td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`;
}

async function renderSession(root, id) {
  const turns = await api('/api/sessions/' + encodeURIComponent(id));
  let totalIn = 0, totalOut = 0, totalCacheRd = 0;
  for (const t of turns) {
    if (t.type !== 'assistant') continue;
    totalIn += t.input_tokens || 0;
    totalOut += t.output_tokens || 0;
    totalCacheRd += t.cache_read_tokens || 0;
  }
  const slug = turns[0]?.project_slug || '';
  const cwd = (turns.find(t => t.cwd) || {}).cwd || '';
  const base = cwd ? cwd.replace(/\\/g, '/').replace(/\/+$/, '').split('/').pop() : '';
  const project = base || slug;
  const started = turns[0]?.timestamp || '';
  const ended = turns.at(-1)?.timestamp || '';

  root.innerHTML = `
    <div class="card">
      <h2 style="display:flex;align-items:center">
        <span>Session <span class="mono" style="font-size:14px;font-weight:400">${fmt.htmlSafe(id)}</span></span>
        <span class="spacer"></span>
        <a href="#/sessions" class="muted">← all sessions</a>
      </h2>
      <div class="flex muted" style="font-family:var(--mono);font-size:12px;flex-wrap:wrap;gap:14px">
        <span>${fmt.htmlSafe(project)}</span>
        <span>${fmt.ts(started)} → ${fmt.ts(ended)}</span>
        <span>${turns.length} records</span>
        <span>${fmt.int(totalIn)} in · ${fmt.int(totalOut)} out · ${fmt.int(totalCacheRd)} cache rd</span>
      </div>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>Turn-by-turn</h3>
      <div style="overflow-x:auto">
      <table id="turn-table">
        <thead><tr><th>time</th><th>type</th><th>model</th><th>prompt / tools</th><th class="num">in</th><th class="num">out</th><th class="num">cache rd</th></tr></thead>
        <tbody>
          ${turns.map((t, i) => {
            const tools = t.tool_calls_json ? JSON.parse(t.tool_calls_json) : [];
            const summary = t.prompt_text
              ? fmt.short(t.type === 'attachment' ? t.prompt_text.split('\n')[0] : t.prompt_text, 110)
              : tools.length ? tools.map(x => x.name).join(' · ')
              : '';
            const clickable = !!(t.prompt_text || t.tool_calls_json);
            return `<tr data-i="${i}"${clickable ? ' style="cursor:pointer"' : ''}>
              <td class="mono">${(t.timestamp || '').slice(11,19)}</td>
              <td>${typeCell(t.type)}${t.is_sidechain ? ' <span class="badge">side</span>' : ''}</td>
              <td>${t.model ? `<span class="badge ${fmt.modelClass(t.model)}">${fmt.htmlSafe(fmt.modelShort(t.model))}</span>` : ''}</td>
              <td>${fmt.htmlSafe(summary)}</td>
              <td class="num">${fmt.int(t.input_tokens)}</td>
              <td class="num">${fmt.int(t.output_tokens)}</td>
              <td class="num">${fmt.int(t.cache_read_tokens)}</td>
            </tr>`;
          }).join('')}
        </tbody>
      </table>
      </div>
    </div>`;

  root.querySelectorAll('#turn-table tbody tr[data-i]').forEach(tr => {
    const t = turns[Number(tr.dataset.i)];
    if (!turnContent(t)) return;
    tr.addEventListener('click', () => showTurnDetail(t));
  });
}
