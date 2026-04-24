import { api, fmt } from '/web/app.js';

const TIP_ICON = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;color:var(--accent)"><path d="M9 18h6"/><path d="M10 22h4"/><path d="M12 2a7 7 0 0 1 7 7c0 2.6-1.4 4.9-3.5 6.2-.5.3-.5.8-.5 1.3V17H9v-.5c0-.5 0-1-.5-1.3A7 7 0 0 1 12 2z"/></svg>`;

export default async function (root) {
  const tips = (await api('/api/tips')) || [];
  root.innerHTML = `
    <div class="card">
      <h2>Suggestions</h2>
      ${tips.length === 0
        ? '<p class="muted">No suggestions right now. Token Dashboard surfaces patterns weekly — check back after more activity.</p>'
        : `<p class="muted" style="margin:-8px 0 14px">Rule-based pattern detection over the last 7 days. Dismissed tips re-appear after 14 days.</p>`}
      ${tips.map(t => `
        <div class="tip">
          <div class="tip-head">
            ${TIP_ICON}
            <strong>${fmt.htmlSafe(t.title)}</strong>
            <span class="spacer"></span>
            <button class="ghost" data-key="${fmt.htmlSafe(t.key)}">dismiss</button>
          </div>
          <p class="tip-body">${fmt.htmlSafe(t.body)}</p>
        </div>`).join('')}
    </div>`;
  root.querySelectorAll('button[data-key]').forEach(b => {
    b.addEventListener('click', async () => {
      await api('/api/tips/dismiss', { method: 'POST', body: JSON.stringify({ key: b.dataset.key }) });
      location.reload();
    });
  });
}
