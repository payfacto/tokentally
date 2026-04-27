# Scan Now + Data Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Scan Now" button and a configurable data-retention purge (auto + manual) to the Settings page.

**Architecture:** Three new DB helpers in `internal/db/db.go`, three thin wrapper methods on `App` in `app/app.go` (plus a one-line `scanLoop` change), and a new "Data Management" card in `frontend/web/routes/settings.js`. Pruned message rows stay deleted because the `files` table rows are left intact — the scanner treats those file paths as already-processed and skips re-import.

**Tech Stack:** Go 1.21+, SQLite (modernc.org/sqlite), Wails v2, vanilla JS (no build step).

---

## File Map

| File | Change |
|---|---|
| `internal/db/db.go` | Add `GetRetentionDays`, `SetRetentionDays`, `PurgeMessages` |
| `internal/db/db_test.go` | Add tests for the three new DB helpers |
| `app/app.go` | Add `GetRetentionDays`, `SetRetentionDays`, `PurgeOlderThan`; update `scanLoop` |
| `frontend/web/routes/settings.js` | Add Data Management card + bind functions |

---

## Task 1: DB helpers — retention get/set

**Files:**
- Modify: `internal/db/db.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing tests**

Add to the bottom of `internal/db/db_test.go`:

```go
func TestGetSetRetentionDays(t *testing.T) {
	conn := openMem(t)

	// Default when key is absent should be 0 (off).
	days, err := db.GetRetentionDays(conn)
	if err != nil {
		t.Fatalf("GetRetentionDays (default) failed: %v", err)
	}
	if days != 0 {
		t.Errorf("default retention = %d, want 0", days)
	}

	// Set to 90.
	if err := db.SetRetentionDays(conn, 90); err != nil {
		t.Fatalf("SetRetentionDays(90) failed: %v", err)
	}

	days, err = db.GetRetentionDays(conn)
	if err != nil {
		t.Fatalf("GetRetentionDays after set failed: %v", err)
	}
	if days != 90 {
		t.Errorf("retention after set = %d, want 90", days)
	}

	// Overwrite with 30 (idempotent upsert).
	if err := db.SetRetentionDays(conn, 30); err != nil {
		t.Fatalf("SetRetentionDays(30) failed: %v", err)
	}
	days, _ = db.GetRetentionDays(conn)
	if days != 30 {
		t.Errorf("retention after overwrite = %d, want 30", days)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```
go test ./internal/db/... -run TestGetSetRetentionDays -v
```

Expected: `FAIL — db.GetRetentionDays undefined`

- [ ] **Step 3: Implement GetRetentionDays and SetRetentionDays**

Add to `internal/db/db.go` after the `SetPlan`/`GetPlan` block (search for `func SetPlan`):

```go
// GetRetentionDays reads the retention policy from the plan table.
// Returns 0 if not set (= keep forever / auto-purge disabled).
func GetRetentionDays(conn *sql.DB) (int, error) {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k='retention_days'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var days int
	fmt.Sscanf(v, "%d", &days)
	return days, nil
}

// SetRetentionDays persists the retention policy.
// days=0 effectively disables auto-purge.
func SetRetentionDays(conn *sql.DB, days int) error {
	_, err := conn.Exec(
		`INSERT INTO plan (k,v) VALUES ('retention_days',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`,
		fmt.Sprintf("%d", days),
	)
	return err
}
```

- [ ] **Step 4: Run the tests to confirm they pass**

```
go test ./internal/db/... -run TestGetSetRetentionDays -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go
git commit -m "feat(db): add GetRetentionDays and SetRetentionDays"
```

---

## Task 2: DB helper — PurgeMessages

**Files:**
- Modify: `internal/db/db.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing test**

Add to the bottom of `internal/db/db_test.go`:

```go
func TestPurgeMessages(t *testing.T) {
	conn := openMem(t)

	// Old message + tool_call (timestamp safely in the past).
	insertMessage(t, conn, map[string]any{
		"uuid": "old1", "session_id": "s-old", "project_slug": "proj",
		"type": "assistant", "timestamp": "2020-01-01T00:00:00Z",
		"input_tokens": 100, "output_tokens": 50,
	})
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "old1", "session_id": "s-old", "project_slug": "proj",
		"tool_name": "Bash", "timestamp": "2020-01-01T00:00:00Z",
	})

	// Recent message (far future — will never be purged with days=1).
	insertMessage(t, conn, map[string]any{
		"uuid": "new1", "session_id": "s-new", "project_slug": "proj",
		"type": "assistant", "timestamp": "2099-01-01T00:00:00Z",
		"input_tokens": 200, "output_tokens": 80,
	})

	deleted, err := db.PurgeMessages(conn, 1) // purge anything older than 1 day
	if err != nil {
		t.Fatalf("PurgeMessages failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only the old message)", deleted)
	}

	// Recent message must still be present.
	var count int
	conn.QueryRow(`SELECT COUNT(*) FROM messages WHERE uuid='new1'`).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("recent message was incorrectly deleted")
	}

	// Old message must be gone.
	conn.QueryRow(`SELECT COUNT(*) FROM messages WHERE uuid='old1'`).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("old message was not deleted")
	}

	// Tool call for old message must be gone.
	conn.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE message_uuid='old1'`).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("tool_call for old message was not deleted")
	}
}

func TestPurgeMessages_ZeroDaysIsNoop(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "m1", "session_id": "s1", "project_slug": "proj",
		"type": "assistant", "timestamp": "2020-01-01T00:00:00Z",
	})

	deleted, err := db.PurgeMessages(conn, 0)
	if err != nil {
		t.Fatalf("PurgeMessages(0) failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("PurgeMessages(0) deleted %d rows, want 0 (no-op)", deleted)
	}

	var count int
	conn.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("message was incorrectly deleted when days=0")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```
go test ./internal/db/... -run TestPurgeMessages -v
```

Expected: `FAIL — db.PurgeMessages undefined`

- [ ] **Step 3: Implement PurgeMessages**

Add to `internal/db/db.go` immediately after `SetRetentionDays`:

```go
// PurgeMessages deletes tool_calls and messages whose timestamp is older than
// the given number of days. Returns the number of message rows deleted.
// The files table is left intact so the scanner skips already-processed paths
// and does not re-import the pruned data.
// days=0 is a no-op.
func PurgeMessages(conn *sql.DB, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := fmt.Sprintf("-%d days", days)
	if _, err := conn.Exec(
		`DELETE FROM tool_calls WHERE timestamp < datetime('now', ?)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("purge tool_calls: %w", err)
	}
	result, err := conn.Exec(
		`DELETE FROM messages WHERE timestamp < datetime('now', ?)`, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("purge messages: %w", err)
	}
	return result.RowsAffected()
}
```

- [ ] **Step 4: Run all DB tests**

```
go test ./internal/db/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go
git commit -m "feat(db): add PurgeMessages with tool_calls cascade"
```

---

## Task 3: App methods + scanLoop

**Files:**
- Modify: `app/app.go`

No unit tests needed here — these are thin wrappers over the already-tested DB helpers. The `go build` step is the verification.

- [ ] **Step 1: Add the three exported methods**

In `app/app.go`, add after the `ScanNow` method (around line 425):

```go
func (a *App) GetRetentionDays() (int, error) {
	return db.GetRetentionDays(a.conn)
}

func (a *App) SetRetentionDays(days int) error {
	return db.SetRetentionDays(a.conn, days)
}

// PurgeOlderThan deletes messages older than the given number of days.
// Returns the number of message rows deleted.
func (a *App) PurgeOlderThan(days int) (int64, error) {
	return db.PurgeMessages(a.conn, days)
}
```

- [ ] **Step 2: Update scanLoop to auto-purge**

Replace the existing `scanLoop` method in `app/app.go`:

```go
func (a *App) scanLoop() {
	interval := 30 * time.Second
	for {
		result, err := scanner.ScanDir(a.conn, a.projectsDir)
		if err == nil && (result.Messages > 0 || result.Files > 0) {
			runtime.EventsEmit(a.ctx, "scan", result)
		}
		if days, _ := db.GetRetentionDays(a.conn); days > 0 {
			db.PurgeMessages(a.conn, days) //nolint:errcheck
		}
		time.Sleep(interval)
	}
}
```

- [ ] **Step 3: Verify the build compiles**

```
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add app/app.go
git commit -m "feat(app): add retention methods and auto-purge in scanLoop"
```

---

## Task 4: Frontend — Data Management card

**Files:**
- Modify: `frontend/web/routes/settings.js`

No automated tests for vanilla JS. Manual verification steps are provided below.

- [ ] **Step 1: Add GetRetentionDays to renderAll's Promise.all**

In `frontend/web/routes/settings.js`, find the `renderAll` function. The current `Promise.all` is:

```js
const [planResp, models, plans, rates, apiKey] = await Promise.all([
    App.GetPlan(),
    App.GetPricingModels(),
    App.GetPricingPlans(),
    App.GetExchangeRates(),
    App.GetExchangeApiKey(),
  ]);
```

Replace it with:

```js
const [planResp, models, plans, rates, apiKey, retentionDays] = await Promise.all([
    App.GetPlan(),
    App.GetPricingModels(),
    App.GetPricingPlans(),
    App.GetExchangeRates(),
    App.GetExchangeApiKey(),
    App.GetRetentionDays(),
  ]);
```

- [ ] **Step 2: Add the Data Management card HTML**

In `settings.js`, find this comment in the `root.innerHTML` template:

```js
    <div class="card" id="service-card" style="margin-top:16px">
```

Insert the new card immediately before it:

```js
    <div class="card" id="card-data" style="margin-top:16px">
      <h2>Data Management</h2>

      <h3 style="margin-top:16px">Scanner</h3>
      <p class="muted" style="margin:0 0 12px;font-size:13px">The scanner runs automatically every 30 seconds. Use this to pick up new sessions immediately.</p>
      <div class="flex" style="gap:10px;align-items:center">
        <button id="btn-scan-now">Scan Now</button>
        <span id="scan-msg" class="muted" style="font-size:12px"></span>
      </div>

      <hr class="divider" style="margin:20px 0">

      <h3>Retention</h3>
      <p class="muted" style="margin:0 0 12px;font-size:13px">Automatically delete old data from TokenTally's database. Leave blank to keep data forever.</p>
      <div class="flex" style="gap:10px;align-items:center">
        <label class="form-label" style="margin:0;white-space:nowrap">Delete data older than</label>
        <input id="retention-days" type="number" min="1" step="1" class="form-input" style="width:90px"
          value="${retentionDays > 0 ? retentionDays : ''}" placeholder="e.g. 90">
        <span style="color:var(--muted);font-size:13px">days</span>
        <button class="primary" id="btn-save-retention">Save</button>
        <span id="retention-msg" class="muted" style="font-size:12px"></span>
      </div>
      <div style="margin-top:10px">
        <button id="btn-purge-now" style="background:var(--bad);color:#fff;border:none;padding:6px 14px;border-radius:6px;cursor:pointer" ${retentionDays <= 0 ? 'disabled' : ''}>Purge Now</button>
        <span id="purge-msg" class="muted" style="font-size:12px;margin-left:10px"></span>
      </div>
      <p class="muted" style="font-size:11px;margin-top:8px">Removes messages from TokenTally's database only. Your <code style="font-size:11px">~/.claude/projects/</code> files are not affected and won't be re-imported.</p>
    </div>

```

- [ ] **Step 3: Pass retentionDays to bindDataManagement**

In the bottom of `renderAll`, find:

```js
  bindGeneral(root, plans, currentPlan, rates, currency);
  bindModels(root, models);
  bindPlans(root, plans);
  bindService(root);
```

Add `bindDataManagement` at the end:

```js
  bindGeneral(root, plans, currentPlan, rates, currency);
  bindModels(root, models);
  bindPlans(root, plans);
  bindService(root);
  bindDataManagement(root, retentionDays);
```

- [ ] **Step 4: Add the bindDataManagement function**

Add this function at the end of `settings.js`, after `bindService`:

```js
function bindDataManagement(root, initialRetentionDays) {
  let currentDays = initialRetentionDays;

  root.querySelector('#btn-scan-now').addEventListener('click', async () => {
    const msg = root.querySelector('#scan-msg');
    msg.textContent = 'Scanning…';
    msg.style.color = 'var(--muted)';
    try {
      const result = await App.ScanNow();
      if (result.Messages > 0 || result.Files > 0) {
        msg.textContent = `Scanned ${result.Messages} messages in ${result.Files} files`;
      } else {
        msg.textContent = 'Nothing new';
      }
      msg.style.color = 'var(--good)';
    } catch (e) {
      msg.textContent = 'Error: ' + (e.message || String(e));
      msg.style.color = 'var(--bad)';
    }
    setTimeout(() => { msg.textContent = ''; }, 2500);
  });

  const daysInput  = root.querySelector('#retention-days');
  const purgeBtn   = root.querySelector('#btn-purge-now');

  daysInput.addEventListener('input', () => {
    const val = parseInt(daysInput.value, 10);
    purgeBtn.disabled = !(val > 0);
  });

  root.querySelector('#btn-save-retention').addEventListener('click', async () => {
    const msg  = root.querySelector('#retention-msg');
    const val  = parseInt(daysInput.value, 10) || 0;
    await App.SetRetentionDays(val);
    currentDays = val;
    purgeBtn.disabled = val <= 0;
    flash(msg, val > 0 ? `Saved — auto-purge every scan (>${val} days)` : 'Saved — retention off');
  });

  purgeBtn.addEventListener('click', async () => {
    const val = parseInt(daysInput.value, 10) || 0;
    if (val <= 0) return;
    if (!confirm(`Delete all TokenTally data older than ${val} days? This cannot be undone.`)) return;
    const msg = root.querySelector('#purge-msg');
    msg.textContent = 'Purging…';
    msg.style.color = 'var(--muted)';
    try {
      const deleted = await App.PurgeOlderThan(val);
      msg.textContent = deleted > 0
        ? `Deleted ${deleted.toLocaleString()} messages`
        : 'Nothing to purge';
      msg.style.color = deleted > 0 ? 'var(--good)' : 'var(--muted)';
    } catch (e) {
      msg.textContent = 'Error: ' + (e.message || String(e));
      msg.style.color = 'var(--bad)';
    }
    setTimeout(() => { msg.textContent = ''; }, 2500);
  });
}
```

- [ ] **Step 5: Manual verification**

Build and run the app:

```
wails build -platform windows/amd64 -skipbindings
.\build\bin\tokentally.exe
```

Navigate to Settings. Verify:

1. **Data Management card** appears between Exchange Rate API and Windows Service.
2. **Scan Now** — click it; feedback shows message/file counts or "Nothing new" then clears after 2.5 s.
3. **Save retention** — enter `90`, click Save; confirm message appears. Reload Settings; input shows `90`.
4. **Purge Now** — disabled when input is blank/0. Enabled after entering a value. Clicking shows confirm dialog. After confirm, shows deleted count or "Nothing to purge".
5. **Purge Now disabled by default** — if retention was never set, button starts disabled.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/routes/settings.js
git commit -m "feat(settings): add Data Management card with Scan Now and retention purge"
```

---

## Self-Review

**Spec coverage:**
- ✅ Scan Now button in Settings UI → Task 4
- ✅ `App.ScanNow()` already existed; wired in Task 4
- ✅ `GetRetentionDays` / `SetRetentionDays` DB helpers → Task 1
- ✅ `PurgeMessages` (deletes tool_calls + messages, keeps files) → Task 2
- ✅ Auto-purge in `scanLoop` → Task 3
- ✅ `App.GetRetentionDays`, `App.SetRetentionDays`, `App.PurgeOlderThan` → Task 3
- ✅ Frontend card with Scan Now + Retention input + Save + Purge Now → Task 4
- ✅ Default off (blank/0 = disabled) → Task 4 initial value + disabled state
- ✅ "Stay pruned" (files table untouched) → Task 2 PurgeMessages impl + comment
- ✅ Purge Now disabled when days=0 → Task 4 HTML `disabled` attr + input listener
- ✅ Confirm dialog before purge → Task 4 `bindDataManagement`
- ✅ Feedback text clears after 2.5 s → Task 4 `setTimeout`

**Placeholder scan:** None found.

**Type consistency:**
- `PurgeMessages` returns `(int64, error)` in DB layer → `PurgeOlderThan` returns `(int64, error)` in App → JS receives a number. Consistent.
- `GetRetentionDays` returns `(int, error)` throughout. `SetRetentionDays` takes `int`. Consistent.
- JS `App.ScanNow()` returns `scanner.ScanResult` with `.Messages` and `.Files` fields — matches the existing `ScanNow` in `app.go` which returns `scanner.ScanResult`. Consistent.
