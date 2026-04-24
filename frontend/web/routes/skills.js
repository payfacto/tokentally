import { api, fmt, RANGES, readRange, writeRange, sinceIso } from '/web/app.js';
import { barChart } from '/web/charts.js';

const SKILL_NAME_MAX = 25;
const TOP_SKILLS_LIMIT = 12;

export default async function (root) {
  const range = readRange();
  const since = sinceIso(range);
  const url = '/api/skills' + (since ? '?since=' + encodeURIComponent(since) : '');
  const skills = await api(url);

  const totalInvocations = skills.reduce((s, r) => s + r.invocations, 0);

  const rangeTabs = `
    <div class="range-tabs" role="tablist">
      ${RANGES.map(r => `<button data-range="${r.key}" class="${r.key === range.key ? 'active' : ''}">${r.label}</button>`).join('')}
    </div>`;

  root.innerHTML = `
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Skills</h2>
      <span class="muted" style="font-size:12px">${range.days ? `last ${range.days} days` : 'all time'}</span>
      <div class="spacer"></div>
      ${rangeTabs}
    </div>

    <div class="row cols-2">
      <div class="card kpi"><div class="label">Unique skills used</div><div class="value">${fmt.int(skills.length)}</div></div>
      <div class="card kpi"><div class="label">Total invocations</div><div class="value">${fmt.int(totalInvocations)}</div></div>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>Top skills (by invocations)</h3>
      <div id="ch-skills" style="height:320px"></div>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>All skills</h3>
      <p class="muted" style="margin:-4px 0 14px;font-size:12px">"Tokens per call" is the size of the skill's <code>SKILL.md</code> file — what Claude Code loads into context each time the skill is invoked.</p>
      <table>
        <thead><tr>
          <th>skill</th>
          <th class="num">invocations</th>
          <th class="num">tokens per call</th>
          <th class="num">sessions</th>
          <th>last used</th>
        </tr></thead>
        <tbody>
          ${skills.map(s => `
            <tr>
              <td><span class="badge">${fmt.htmlSafe(s.skill)}</span></td>
              <td class="num">${fmt.int(s.invocations)}</td>
              <td class="num">${s.tokens_per_call == null ? '<span class="muted">—</span>' : fmt.int(s.tokens_per_call)}</td>
              <td class="num">${fmt.int(s.sessions)}</td>
              <td class="mono">${fmt.ts(s.last_used)}</td>
            </tr>`).join('') || '<tr><td colspan="5" class="muted">no skills invoked in this range</td></tr>'}
        </tbody>
      </table>
    </div>
  `;

  root.querySelectorAll('.range-tabs button').forEach(btn => {
    btn.addEventListener('click', () => writeRange(btn.dataset.range, '/skills'));
  });

  const top = skills.slice(0, TOP_SKILLS_LIMIT);
  barChart(document.getElementById('ch-skills'), {
    categories: top.map(t => t.skill.length > SKILL_NAME_MAX ? t.skill.slice(0, SKILL_NAME_MAX) + '…' : t.skill),
    values: top.map(t => t.invocations),
    color: '#3FB68B',
  });
}
