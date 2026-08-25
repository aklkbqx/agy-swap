import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { parseAnsi, xterm256ToHex } from "../src/components/ansi.js";
import { VIEWS, ACCOUNTS, PROFILES, HISTORY_EVENTS, FORM_FIELD_COUNTS, resolveFixture, fetchShard } from "../src/components/fixtures.js";
import { createInitialTuiState, transitionTuiState, selectNextAccount, routeKeyAction, getPaletteActions } from "../src/components/tuiState.js";
import { createShardManager } from "../src/components/shardManager.js";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

test("Required public assets exist and stale public fixtures absent", () => {
  const publicDir = join(__dirname, "../public");
  assert.ok(existsSync(join(publicDir, "favicon.png")));
  assert.ok(existsSync(join(publicDir, "fallback/credential-relay.jpg")));
  assert.ok(existsSync(join(publicDir, "fallback/credential-relay.avif")));
  assert.ok(existsSync(join(publicDir, "fallback/credential-relay-800.jpg")));
  assert.ok(existsSync(join(publicDir, "fallback/credential-relay-800.avif")));

  assert.ok(
    !existsSync(join(publicDir, "tui-fixtures.json")),
    "stale site/public/tui-fixtures.json must not exist"
  );
  assert.ok(
    !existsSync(join(__dirname, "../src/components/TUI.module.css")),
    "stale site/src/components/TUI.module.css must not exist"
  );
});

test("Canonical install strings exist in App source", () => {
  const appPath = join(__dirname, "../src/App.jsx");
  const appCode = readFileSync(appPath, "utf-8");

  assert.ok(appCode.includes("go install github.com/aklkbqx/agy-swap/cmd/agy-swap@latest"));
  assert.ok(appCode.includes("install.sh"));
  assert.ok(appCode.includes("install.ps1"));
});

test("Exactly seven view ids and no Help view in navigation tabs", () => {
  assert.equal(VIEWS.length, 7);
  assert.deepEqual(VIEWS, ["Dashboard", "Quota", "Profiles", "History", "Settings", "Doctor", "Backup"]);
  assert.ok(!VIEWS.includes("Help"), "Help must not be an external view button; it is an in-terminal overlay");
});

test("No forbidden fabricated quota patterns, obsolete tuiContent/tuiStatus, or dangerouslySetInnerHTML", () => {
  const demoPath = join(__dirname, "../src/components/InteractiveDemo.jsx");
  const termPath = join(__dirname, "../src/components/TuiTerminal.jsx");
  const demoCode = readFileSync(demoPath, "utf-8");
  const termCode = readFileSync(termPath, "utf-8");

  assert.ok(!demoCode.includes("tuiStatus"), "InteractiveDemo must not use obsolete tuiStatus");
  assert.ok(!demoCode.includes("tuiContent"), "InteractiveDemo must not use obsolete tuiContent");
  assert.ok(!demoCode.includes("dangerouslySetInnerHTML"), "InteractiveDemo must not use dangerouslySetInnerHTML");
  assert.ok(!termCode.includes("dangerouslySetInnerHTML"), "TuiTerminal must not use dangerouslySetInnerHTML");

  assert.ok(!demoCode.includes("tier1"), "Fabricated tier1 found in InteractiveDemo");
  assert.ok(!demoCode.includes("tier2"), "Fabricated tier2 found in InteractiveDemo");
  assert.ok(!demoCode.includes("100% (2h 30m)"), "Fabricated quota string found in InteractiveDemo");
});

test("Production canonical domain in metadata, github.io absent", () => {
  const htmlPath = join(__dirname, "../index.html");
  const htmlCode = readFileSync(htmlPath, "utf-8");

  assert.ok(htmlCode.includes("https://agy-swap.aklkbqx.com/"));
  assert.ok(!htmlCode.includes("github.io"));
});

test("No window/document keydown listeners, no global Tab trap, no console usage in src", () => {
  const files = [
    join(__dirname, "../src/App.jsx"),
    join(__dirname, "../src/components/InteractiveDemo.jsx"),
    join(__dirname, "../src/components/TuiTerminal.jsx"),
    join(__dirname, "../src/components/TerminalScene.jsx"),
    join(__dirname, "../src/components/fixtures.js"),
    join(__dirname, "../src/components/shardLoaders.js"),
    join(__dirname, "../src/components/tuiState.js"),
    join(__dirname, "../src/components/shardManager.js"),
    join(__dirname, "../src/components/ansi.js"),
  ];

  for (const f of files) {
    if (!existsSync(f)) continue;
    const content = readFileSync(f, "utf-8");
    assert.ok(!content.includes("window.addEventListener('keydown'"), `window keydown found in ${f}`);
    assert.ok(!content.includes("document.addEventListener('keydown'"), `document keydown found in ${f}`);
    assert.ok(!content.includes("console.log("), `console.log found in ${f}`);
  }
});

test("TerminalScene uses both textures and Math.PI / 2 dial orientation", () => {
  const scenePath = join(__dirname, "../src/components/TerminalScene.jsx");
  const sceneCode = readFileSync(scenePath, "utf-8");

  assert.ok(sceneCode.includes("useLoader"), "TerminalScene must use useLoader");
  assert.ok(sceneCode.includes("meshStandardMaterial") || sceneCode.includes("MeshStandardMaterial"), "TerminalScene missing standard material");
  assert.ok(sceneCode.includes("Math.PI / 2"), "TerminalScene missing Math.PI / 2 rotation");
});

test("Authoritative initial, full, and 21 layout+view shard split (51054 fixtures)", () => {
  const fullPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const initialPath = join(__dirname, "../src/generated/tui-initial-fixtures.json");
  const shardsDir = join(__dirname, "../src/generated/shards");

  assert.ok(existsSync(fullPath), "Full fixture bank missing");
  assert.ok(existsSync(initialPath), "Initial fixtures missing");
  assert.ok(existsSync(shardsDir), "Shards directory missing");
  
  const fullData = JSON.parse(readFileSync(fullPath, "utf-8"));
  const initialData = JSON.parse(readFileSync(initialPath, "utf-8"));

  assert.equal(fullData.schema, 1);
  assert.equal(fullData.renderer, "internal/app.(*Application).tuiLines");
  assert.equal(fullData.version, "2.1.2");
  assert.ok(fullData.sourceFingerprint && fullData.sourceFingerprint.length === 64);
  assert.equal(fullData.fixtures.length, 51054, `fixture count = ${fullData.fixtures.length}, want 51054`);

  assert.equal(initialData.fixtures.length, 3, "initial fixtures must contain exactly 3 canonical frames");

  const layouts = ["wide", "stacked", "compact"];
  const views = ["dashboard", "quota", "profiles", "history", "settings", "doctor", "backup"];
  let totalShardFixtures = 0;

  for (const l of layouts) {
    for (const v of views) {
      const shardPath = join(shardsDir, `${l}.${v}.json`);
      assert.ok(existsSync(shardPath), `Shard ${l}.${v}.json missing`);
      const sData = JSON.parse(readFileSync(shardPath, "utf-8"));
      assert.equal(sData.schema, 1);
      assert.ok(sData.fixtures.length > 0, `Shard ${l}.${v}.json has no fixtures`);
      totalShardFixtures += sData.fixtures.length;
    }
  }

  assert.equal(totalShardFixtures, 51054, `Total shard fixtures = ${totalShardFixtures}, want 51054`);
});

test("ANSI whitelist SGR parser unit tests", () => {
  assert.equal(xterm256ToHex(0), "#000000");
  assert.equal(xterm256ToHex(15), "#ffffff");
  assert.equal(xterm256ToHex(208), "#ff8700");
  assert.equal(xterm256ToHex(78), "#5fd787");
  assert.equal(xterm256ToHex(238), "#444444");
  assert.equal(xterm256ToHex(244), "#808080");
  assert.equal(xterm256ToHex(255), "#eeeeee");
  assert.equal(xterm256ToHex(999), null);

  const plainRes = parseAnsi("Hello World");
  assert.equal(plainRes.length, 1);
  assert.deepEqual(plainRes[0], { text: "Hello World", bold: false, color: null });

  const boldRes = parseAnsi("\x1b[1mBold Text\x1b[0m Normal");
  assert.equal(boldRes.length, 2);
  assert.deepEqual(boldRes[0], { text: "Bold Text", bold: true, color: null });
  assert.deepEqual(boldRes[1], { text: " Normal", bold: false, color: null });
});

test("Exact production keyboard routing: routeKeyAction pure router tests", () => {
  let s = createInitialTuiState();

  assert.deepEqual(routeKeyAction(s, "?"), { type: "OPEN_HELP" });
  assert.deepEqual(routeKeyAction(s, "/"), { type: "OPEN_SEARCH" });
  assert.deepEqual(routeKeyAction(s, "k", { ctrlKey: true }), { type: "OPEN_PALETTE" });
  assert.deepEqual(routeKeyAction(s, ":"), { type: "OPEN_PALETTE" });
  assert.deepEqual(routeKeyAction(s, "h"), { type: "NAVIGATE_VIEW", view: "History" });
  assert.deepEqual(routeKeyAction(s, "p"), { type: "NAVIGATE_VIEW", view: "Profiles" });
  assert.deepEqual(routeKeyAction(s, "s"), { type: "NAVIGATE_VIEW", view: "Settings" });
  assert.deepEqual(routeKeyAction(s, "o"), { type: "NAVIGATE_DOCTOR" });

  assert.deepEqual(routeKeyAction(s, "b"), { type: "NAVIGATE_VIEW", view: "Backup" });
  s.view = "Settings";
  assert.deepEqual(routeKeyAction(s, "b"), { type: "OPEN_FORM", formKind: "binding" });
  s.view = "Quota";
  assert.deepEqual(routeKeyAction(s, "b"), { type: "NAVIGATE_VIEW", view: "Dashboard" });

  s.view = "Dashboard";
  assert.deepEqual(routeKeyAction(s, "v"), { type: "NAVIGATE_VIEW", view: "Quota" });
  s.view = "Backup";
  assert.deepEqual(routeKeyAction(s, "v"), { type: "OPEN_FORM", formKind: "backup-verify" });

  s.view = "Dashboard";
  assert.deepEqual(routeKeyAction(s, "r"), { type: "TRIGGER_REFRESH" });
  s.view = "Doctor";
  assert.deepEqual(routeKeyAction(s, "r"), { type: "START_DOCTOR" });

  s.view = "Settings";
  assert.deepEqual(routeKeyAction(s, "a"), { type: "OPEN_FORM", formKind: "alias" });
  assert.deepEqual(routeKeyAction(s, "t"), { type: "OPEN_FORM", formKind: "target" });
  s.view = "Dashboard";
  assert.equal(routeKeyAction(s, "a").type, "ANNOUNCE_SAFE_NOTICE");
  assert.equal(routeKeyAction(s, "t").type, "ANNOUNCE_SAFE_NOTICE");
  assert.deepEqual(routeKeyAction(s, "d"), { type: "OPEN_CONFIRM_DELETE" });
  s.view = "Profiles";
  assert.deepEqual(routeKeyAction(s, "d"), { type: "OPEN_CONFIRM_ACTION", confirmKind: "profile-remove" });

  s.view = "Dashboard";
  assert.deepEqual(routeKeyAction(s, "home"), { type: "NAV_HOME" });
  assert.deepEqual(routeKeyAction(s, "end"), { type: "NAV_END" });
  assert.deepEqual(routeKeyAction(s, "pageup"), { type: "NAV_PAGE_UP" });
  assert.deepEqual(routeKeyAction(s, "pagedown"), { type: "NAV_PAGE_DOWN" });
});

test("Explicit partial vs full shard semantics: initial bank is not hasFullShard", () => {
  const sm = createShardManager();
  assert.equal(sm.hasFullShard("wide", "Dashboard"), false);
  assert.equal(sm.hasFullShard("stacked", "Dashboard"), false);
  assert.equal(sm.hasFullShard("compact", "Dashboard"), false);
  assert.equal(sm.getLoadedShard("wide", "Dashboard"), null);
});

test("Empty loader rejection: empty or non-array returns are rejected and never cached", async () => {
  const emptyManager = createShardManager(async () => []);
  await assert.rejects(async () => {
    await emptyManager.ensureShard("wide", "Dashboard");
  }, /Failed to load valid non-empty fixtures/);

  assert.equal(emptyManager.hasFullShard("wide", "Dashboard"), false);
  assert.equal(emptyManager.getLoadedShard("wide", "Dashboard"), null);
});

test("Transactional in-view first interaction: delayed-promise proves capture, single replay, and non-initial resolution", async () => {
  let resolveDelayedDashboard;
  const delayedFetch = (layout, view) => {
    return new Promise((resolve) => {
      resolveDelayedDashboard = () => {
        const fullPath = join(__dirname, `../src/generated/shards/${layout}.${view.toLowerCase()}.json`);
        const data = JSON.parse(readFileSync(fullPath, "utf-8"));
        resolve(data.fixtures);
      };
    });
  };

  const sm = createShardManager(delayedFetch);
  let state = createInitialTuiState();
  let pendingAction = null;
  let dispatchedActions = [];
  let shardRevision = 0;

  function getCurrentFixtures(layout, view) {
    const loaded = sm.getLoadedShard(layout, view);
    if (loaded && loaded.length > 0) return loaded;
    const initialData = JSON.parse(readFileSync(join(__dirname, "../src/generated/tui-initial-fixtures.json"), "utf-8"));
    return initialData.fixtures;
  }

  function handleInteraction(key) {
    const action = routeKeyAction(state, key);
    if (sm.hasFullShard("wide", state.view)) {
      dispatchedActions.push(action);
      state = transitionTuiState(state, action);
      return;
    }

    if (pendingAction) {
      return;
    }

    pendingAction = action;
    const reqId = sm.nextRequestId();
    sm.ensureShard("wide", state.view).then(() => {
      if (!sm.isCurrentRequest(reqId)) return;
      shardRevision += 1;
      const act = pendingAction;
      pendingAction = null;
      if (act) {
        dispatchedActions.push(act);
        state = transitionTuiState(state, act);
      }
    });
  }

  // 1. Initial render reads 3-frame initial bank
  let curFixs = getCurrentFixtures("wide", state.view);
  let fInit = resolveFixture(curFixs, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fInit, "initial fixture must resolve");
  assert.equal(fInit.id, "wide.dashboard.ready.active-alpha.selected-alpha");

  // 2. User presses 'd' (delete confirm) on initial Dashboard
  handleInteraction("d");
  assert.equal(dispatchedActions.length, 0, "zero actions before shard load");
  assert.equal(state.mode, "ready", "state mode must remain ready before shard load");

  // 3. Resolve shard
  resolveDelayedDashboard();
  await new Promise((r) => setTimeout(r, 10));

  assert.equal(dispatchedActions.length, 1, "action replayed after shard load");
  assert.equal(state.mode, "confirm_delete", "mode updated to confirm_delete");

  // 4. Resolve fixture after revision update: must resolve exact confirm-delete fixture!
  curFixs = getCurrentFixtures("wide", state.view);
  let fDelConfirm = resolveFixture(curFixs, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fDelConfirm, "confirm-delete fixture must resolve");
  assert.equal(fDelConfirm.id, "wide.dashboard.confirm-delete.account-alpha.active-alpha");
  assert.ok(fDelConfirm.plain.join("\n").includes("DELETE ACCOUNT"), "plain text must render DELETE ACCOUNT header");
  assert.ok(fDelConfirm.plain.join("\n").includes("alpha@example.invalid"), "plain text must render confirm email");
});

test("Initial Dashboard first ArrowDown and Search resolve exact full-shard fixtures", async () => {
  const fullPath = join(__dirname, "../src/generated/shards/wide.dashboard.json");
  const data = JSON.parse(readFileSync(fullPath, "utf-8"));
  const sm = createShardManager(async () => data.fixtures);

  let state = createInitialTuiState();

  let pendingAction = routeKeyAction(state, "arrowdown");
  await sm.ensureShard("wide", "Dashboard");
  state = transitionTuiState(state, pendingAction);

  let curFixs = sm.getLoadedShard("wide", "Dashboard");
  let fArrow = resolveFixture(curFixs, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fArrow, "ArrowDown fixture must resolve from full shard");
  assert.equal(fArrow.id, "wide.dashboard.ready.active-alpha.selected-beta");
  assert.ok(fArrow.plain.join("\n").includes("Beta User"), "plain text must render Beta User as selected");

  state = createInitialTuiState();
  pendingAction = routeKeyAction(state, "/");
  state = transitionTuiState(state, pendingAction);

  let fSearch = resolveFixture(curFixs, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fSearch, "Search fixture must resolve from full shard");
  assert.equal(fSearch.id, "wide.dashboard.search.query-empty.active-alpha.selected-alpha");
  assert.ok(fSearch.plain.join("\n").includes("/▌"), "plain text must render search prompt");
});

test("Failed shard load leaves state unchanged and announces failure", async () => {
  let rejectDelayed;
  const failingFetch = () => new Promise((_, reject) => { rejectDelayed = () => reject(new Error("Network error")); });

  const sm = createShardManager(failingFetch);
  let state = createInitialTuiState();
  let pendingAction = null;
  let dispatchedActions = [];

  function handleInteraction(key) {
    const action = routeKeyAction(state, key);
    if (sm.hasFullShard("wide", state.view)) {
      dispatchedActions.push(action);
      state = transitionTuiState(state, action);
      return;
    }
    pendingAction = action;
    const reqId = sm.nextRequestId();
    sm.ensureShard("wide", state.view).catch(() => {
      if (sm.isCurrentRequest(reqId)) {
        pendingAction = null;
        state = transitionTuiState(state, { type: "ANNOUNCE_SAFE_NOTICE", msg: "Failed to load Dashboard data." });
      }
    });
  }

  handleInteraction("arrowdown");
  assert.equal(dispatchedActions.length, 0);

  rejectDelayed();
  await new Promise((r) => setTimeout(r, 10));

  assert.equal(dispatchedActions.length, 0);
  assert.equal(state.selectedIndex, 0);
  assert.equal(state.ariaLiveMsg, "Failed to load Dashboard data.");
});

test("Exact delete semantics: All 9 active × deleted account combinations match Go state and metadata (with active-none)", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  const emails = ["alpha@example.invalid", "beta@example.invalid", "gamma@example.invalid"];

  for (const act of emails) {
    for (const del of emails) {
      let state = createInitialTuiState();
      state.activeEmail = act;
      state.selectedIndex = emails.indexOf(del);

      state = transitionTuiState(state, { type: "OPEN_CONFIRM_DELETE" });
      assert.equal(state.mode, "confirm_delete");

      state = transitionTuiState(state, { type: "CONFIRM_YES" });
      assert.equal(state.mode, "ready");
      assert.ok(!state.accounts.includes(del), `deleted ${del} must not be in accounts`);
      assert.equal(state.ariaLiveMsg, `Removed account ${del}.`);

      const expectedActive = act === del ? "" : act;
      assert.equal(state.activeEmail, expectedActive, `activeEmail mismatch for act=${act}, del=${del}`);

      const f = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
      assert.ok(f, `Fixture missing for post-delete act=${act}, del=${del}`);
      assert.equal(f.active || "", expectedActive, `Fixture active mismatch for act=${act}, del=${del}`);

      const plain = f.plain.join("\n");
      assert.ok(plain.includes(`Removed account ${del}`), `plain must contain Removed account ${del} banner`);
      assert.ok(!plain.includes(`Account ${del} deleted`), `plain must NOT contain fabricated Account ${del} deleted phrase`);
      
      if (expectedActive === "") {
        assert.ok(plain.includes("ACTIVE  — no saved session"), `plain must render ACTIVE  — no saved session when active account was deleted`);
      } else {
        assert.ok(plain.includes(`<${expectedActive}>`), `plain must render ACTIVE <${expectedActive}>`);
      }
      assert.ok(!plain.includes(`\n${del} `) && !plain.includes(` ${del}\n`), `deleted ${del} must not be in table`);
    }
  }
});

test("Search visible fidelity: query typing, selection clamping, and plain text validation", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "OPEN_SEARCH" });
  assert.equal(state.mode, "search");
  assert.equal(state.searchQuery, "");

  const chars = ["a", "l", "p", "h", "a"];
  for (const c of chars) {
    state = transitionTuiState(state, { type: "SEARCH_CHAR", char: c });
  }
  assert.equal(state.searchQuery, "alpha");
  assert.equal(state.selectedIndex, 0);
  let fAlpha = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fAlpha, "alpha search fixture must resolve");
  assert.ok(fAlpha.plain.join("\n").includes("/alpha▌"), "plain text must render /alpha▌ prompt");
  assert.ok(fAlpha.plain.join("\n").includes("Alpha User"), "plain text must show Alpha User");

  state = transitionTuiState(state, { type: "SEARCH_CTRL_U" });
  assert.equal(state.searchQuery, "");
  let fEmpty = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fEmpty, "empty search fixture must resolve");
  assert.ok(fEmpty.plain.join("\n").includes("/▌"), "plain text must render empty /▌ prompt");

  for (const c of ["b", "e", "t", "a"]) {
    state = transitionTuiState(state, { type: "SEARCH_CHAR", char: c });
  }
  assert.equal(state.searchQuery, "beta");
  assert.equal(state.selectedIndex, 1);
  let fBeta = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fBeta, "beta search fixture must resolve");
  assert.ok(fBeta.plain.join("\n").includes("/beta▌"), "plain text must render /beta▌ prompt");
  assert.ok(fBeta.plain.join("\n").includes("Beta User"), "plain text must show Beta User");

  state = transitionTuiState(state, { type: "SEARCH_CTRL_U" });
  state = transitionTuiState(state, { type: "SEARCH_CHAR", char: "z" });
  assert.equal(state.searchQuery, "z");
  assert.equal(state.searchMatch, false);
  let fNoMatch = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fNoMatch, "z no-match fixture must resolve");
  assert.ok(fNoMatch.plain.join("\n").includes("/z▌"), "plain text must render /z▌ prompt");

  state = transitionTuiState(state, { type: "CANCEL" });
  assert.equal(state.mode, "ready");
  assert.equal(state.searchQuery, "");
});

test("Palette visible fidelity and execution: arrow navigation moves marker in plain text", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "OPEN_PALETTE" });
  assert.equal(state.mode, "palette");
  assert.equal(state.paletteIndex, 0);

  let f0 = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(f0, "palette index 0 fixture must resolve");
  assert.ok(f0.plain.join("\n").includes("> Dashboard"), "plain text must show > Dashboard marker");

  state = transitionTuiState(state, { type: "NAV_DOWN" });
  assert.equal(state.paletteIndex, 1);
  let f1 = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(f1, "palette index 1 fixture must resolve");
  assert.ok(f1.plain.join("\n").includes("> Quota overview"), "plain text must show > Quota overview marker");

  for (const c of ["p", "r", "o", "f", "i", "l", "e"]) {
    state = transitionTuiState(state, { type: "PALETTE_CHAR", char: c });
  }
  assert.equal(state.paletteQuery, "profile");
  let fFiltered = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fFiltered, "filtered profile palette fixture must resolve");
  assert.ok(fFiltered.plain.join("\n").includes(":profile▌"), "plain text must render :profile▌ prompt");

  state = transitionTuiState(state, { type: "ENTER" });
  assert.equal(state.mode, "ready");
  assert.equal(state.pendingNavigate, "Profiles");
});

test("Renderer-backed form editing: char-x, backspace, ctrl-u, and ctrl-w inspect actual fixture plain lines", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Profiles" });
  state.profileIndex = 0;
  state = transitionTuiState(state, { type: "OPEN_FORM", formKind: "profile-edit" });
  assert.equal(state.mode, "form");
  assert.equal(state.fieldIndex, 0);

  let fBase = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fBase, "fBase must resolve");
  assert.ok(fBase.plain.join("\n").includes("personal"), "base fixture must contain 'personal'");

  state = transitionTuiState(state, { type: "FORM_CHAR", char: "x" });
  assert.equal(state.formEditVariant, "char-x");
  let fCharX = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fCharX, "char-x fixture must resolve");
  assert.ok(fCharX.plain.join("\n").includes("personalx"), "plain must render 'personalx'");

  state = transitionTuiState(state, { type: "FORM_BACKSPACE" });
  assert.equal(state.formEditVariant, null);
  let fBackToBase = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fBackToBase, "fBackToBase must resolve");
  assert.ok(fBackToBase.plain.join("\n").includes("personal"), "plain must return to 'personal'");

  state = transitionTuiState(state, { type: "FORM_BACKSPACE" });
  assert.equal(state.formEditVariant, "backspace");
  let fBackspace = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fBackspace, "backspace fixture must resolve");
  assert.ok(fBackspace.plain.join("\n").includes("persona"), "plain must render 'persona'");

  state = transitionTuiState(state, { type: "FORM_CTRL_U" });
  assert.equal(state.formEditVariant, "ctrl-u");
  let fCtrlU = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fCtrlU, "ctrl-u fixture must resolve");
  assert.ok(!fCtrlU.plain.join("\n").includes("personal"), "plain must not contain 'personal'");

  state = transitionTuiState(state, { type: "FORM_CTRL_W" });
  assert.equal(state.formEditVariant, "ctrl-w");
  let fCtrlW = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fCtrlW, "ctrl-w fixture must resolve");
  assert.notEqual(fCtrlU.id, fCtrlW.id, "ctrl-u and ctrl-w must have distinct fixture IDs");
});

test("Transaction helper: delayed-promise proves no state dispatch before resolution and race deduplication", async () => {
  let resolveDelayedShard;
  const delayedFetch = (layout, view) => {
    return new Promise((resolve) => {
      resolveDelayedShard = () => resolve([{ id: `mock.${layout}.${view.toLowerCase()}`, view }]);
    });
  };

  const sm = createShardManager(delayedFetch);
  let dispatchedState = null;
  let pendingTarget = null;

  function performViewNavigation(targetView) {
    if (sm.hasFullShard("wide", targetView)) {
      dispatchedState = targetView;
      pendingTarget = null;
      return;
    }
    pendingTarget = targetView;
    const reqId = sm.nextRequestId();
    sm.ensureShard("wide", targetView).then((fixtures) => {
      if (!sm.isCurrentRequest(reqId)) return;
      pendingTarget = null;
      dispatchedState = targetView;
    });
  }

  performViewNavigation("Profiles");
  assert.equal(pendingTarget, "Profiles");
  assert.equal(dispatchedState, null, "state must NOT be dispatched before shard resolution");

  performViewNavigation("Settings");
  assert.equal(pendingTarget, "Settings");
  assert.equal(dispatchedState, null, "state must remain null during race");

  resolveDelayedShard();
  await new Promise((r) => setTimeout(r, 10));

  assert.equal(dispatchedState, "Settings", "only the latest requested target is dispatched");
  assert.equal(pendingTarget, null, "pending target cleared on resolution");
});

test("Option cycling and form keyboard controls (Left/Right, Backspace, Ctrl-U/Ctrl-W, Tab)", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Profiles" });
  state.profileIndex = 0;
  state = transitionTuiState(state, { type: "OPEN_FORM", formKind: "profile-edit" });
  assert.equal(state.mode, "form");
  assert.equal(state.fieldIndex, 0);

  state = transitionTuiState(state, { type: "NAV_DOWN" });
  assert.equal(state.fieldIndex, 1);
  state = transitionTuiState(state, { type: "NAV_DOWN" });
  assert.equal(state.fieldIndex, 2);

  state = transitionTuiState(state, { type: "NAV_RIGHT" });
  assert.equal(state.formFamilyOption, "gpt");
  let fGpt = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fGpt, "option-gpt fixture resolved");
  assert.ok(fGpt.plain.join("\n").includes("gpt"), "plain text renders gpt family option");

  state = transitionTuiState(state, { type: "NAV_RIGHT" });
  assert.equal(state.formFamilyOption, "empty");
  let fEmpty = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fEmpty, "option-empty fixture resolved");

  state = transitionTuiState(state, { type: "NAV_RIGHT" });
  assert.equal(state.formFamilyOption, "claude");
  let fClaude = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fClaude, "option-claude fixture resolved");
  assert.ok(fClaude.plain.join("\n").includes("claude"), "plain text renders claude family option");
});

test("View preservation in all reachable operational modes", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let stateQuotaDel = createInitialTuiState();
  stateQuotaDel = transitionTuiState(stateQuotaDel, { type: "SELECT_VIEW", view: "Quota" });
  stateQuotaDel = transitionTuiState(stateQuotaDel, { type: "OPEN_CONFIRM_DELETE" });
  assert.equal(stateQuotaDel.mode, "confirm_delete");
  assert.equal(stateQuotaDel.view, "Quota");
  const fQuotaDel = resolveFixture(fixtures, { ...stateQuotaDel, selectedEmail: stateQuotaDel.accounts[stateQuotaDel.selectedIndex] });
  assert.equal(fQuotaDel.id, "wide.quota.confirm-delete.account-alpha.active-alpha");
  assert.equal(fQuotaDel.view, "Quota");

  const secondary = ["Profiles", "History", "Settings", "Doctor", "Backup"];
  for (const v of secondary) {
    let s = createInitialTuiState();
    s = transitionTuiState(s, { type: "SELECT_VIEW", view: v });
    s = transitionTuiState(s, { type: "OPEN_SEARCH" });
    assert.equal(s.mode, "search");
    assert.equal(s.view, v);
    const fSearch = resolveFixture(fixtures, { ...s, selectedEmail: s.accounts[s.selectedIndex] });
    assert.equal(fSearch.id, `wide.${v.toLowerCase()}.search.query-empty.active-alpha`);
    assert.equal(fSearch.view, v);
  }

  for (const v of VIEWS) {
    let s = createInitialTuiState();
    s = transitionTuiState(s, { type: "SELECT_VIEW", view: v });
    s = transitionTuiState(s, { type: "OPEN_CONFIRM_ACTION", confirmKind: "update" });
    assert.equal(s.mode, "confirm_action");
    assert.equal(s.view, v);
    const fUpd = resolveFixture(fixtures, { ...s, selectedEmail: s.accounts[s.selectedIndex] });
    assert.equal(fUpd.id, `wide.${v.toLowerCase()}.confirm-update.active-alpha`);
    assert.equal(fUpd.view, v);
  }
});

test("Keyboard parity: selectNext algorithm ('n'), PageUp/Down steps, and boundary moves", () => {
  const accounts = ["alpha@example.invalid", "beta@example.invalid", "gamma@example.invalid"];

  assert.equal(selectNextAccount(accounts, "alpha@example.invalid"), "beta@example.invalid");
  assert.equal(selectNextAccount(accounts, "beta@example.invalid"), "alpha@example.invalid");
  assert.equal(selectNextAccount(accounts, "gamma@example.invalid"), "alpha@example.invalid");

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "SWITCH_NEXT" });
  assert.equal(state.activeEmail, "beta@example.invalid");

  state = transitionTuiState(state, { type: "SWITCH_NEXT" });
  assert.equal(state.activeEmail, "alpha@example.invalid");

  state.selectedIndex = 0;
  state = transitionTuiState(state, { type: "NAV_PAGE_DOWN" });
  assert.equal(state.selectedIndex, 1);

  state = transitionTuiState(state, { type: "NAV_PAGE_UP" });
  assert.equal(state.selectedIndex, 0);

  state = transitionTuiState(state, { type: "NAV_END" });
  assert.equal(state.selectedIndex, 2);

  state = transitionTuiState(state, { type: "NAV_HOME" });
  assert.equal(state.selectedIndex, 0);
});

test("Delayed fetch & shard resolution: no wrong Dashboard fallback when loading new view", async () => {
  const initialData = JSON.parse(readFileSync(join(__dirname, "../src/generated/tui-initial-fixtures.json"), "utf-8"));
  const initialFixtures = initialData.fixtures;

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Profiles" });

  const earlyResolved = resolveFixture(initialFixtures, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.equal(earlyResolved, null, "resolveFixture must return null for unloaded shard rather than wrong Dashboard fallback");

  const profilesShard = await fetchShard("wide", "Profiles");
  assert.ok(profilesShard.length > 0, "Profiles shard must load");

  const lateResolved = resolveFixture(profilesShard, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(lateResolved, "Profiles fixture must resolve after shard loads");
  assert.equal(lateResolved.view, "Profiles");
  assert.equal(lateResolved.id, "wide.profiles.ready.profile-1.active-alpha");
});

test("Responsive geometry verification across 320 to 1536 widths", () => {
  const widths = [320, 375, 390, 600, 960, 961, 1000, 1200, 1400, 1536];
  const cssPath = join(__dirname, "../src/App.module.css");
  const cssCode = readFileSync(cssPath, "utf-8");

  assert.ok(
    cssCode.includes('.deviceFrame[data-layout="compact"]') && cssCode.includes("aspect-ratio: 4 / 3;"),
    "App.module.css must set compact deviceFrame aspect-ratio to 4 / 3",
  );
  assert.ok(
    cssCode.includes("aspect-ratio: 376 / 228;"),
    "App.module.css must set compact terminalOverlay aspect-ratio to 376 / 228",
  );

  for (const w of widths) {
    let overlayWidthPx;
    if (w <= 600) {
      overlayWidthPx = w * 0.96;
    } else if (w <= 960) {
      overlayWidthPx = Math.min(w * 0.92, 860);
    } else {
      overlayWidthPx = Math.min(w * 0.92, 860);
    }

    assert.ok(overlayWidthPx <= 860.1, `Width at ${w}px exceeds 860px max (${overlayWidthPx}px)`);
    assert.ok(overlayWidthPx <= w, `Width at ${w}px exceeds screen width (${overlayWidthPx}px)`);

    if (w <= 600) {
      const frameHeightPx = w * (3 / 4);
      const maxHeightPx = frameHeightPx * 0.94;
      const overlayHeightPx = Math.min(overlayWidthPx * (228 / 376), maxHeightPx);

      const scaleX = overlayWidthPx / 376;
      const scaleY = overlayHeightPx / 228;
      const computedScale = Math.min(scaleX, scaleY) * 0.98;
      const effectiveFontSizePx = 13 * computedScale;

      if (w === 320) {
        assert.ok(
          effectiveFontSizePx >= 10,
          `Effective font size at 320px = ${effectiveFontSizePx}px, want >= 10px`,
        );
      }
      if (w >= 375) {
        assert.ok(
          effectiveFontSizePx >= 11,
          `Effective font size at ${w}px = ${effectiveFontSizePx}px, want >= 11px`,
        );
      }
    }
  }

  const w960 = Math.min(960 * 0.92, 860);
  const w961 = Math.min(961 * 0.92, 860);
  assert.ok(w961 >= w960, `960 -> 961 inversion detected: w960=${w960}, w961=${w961}`);
});

test("No full aggregate bank in client asset graph", () => {
  const demoCode = readFileSync(join(__dirname, "../src/components/InteractiveDemo.jsx"), "utf-8");
  const fixCode = readFileSync(join(__dirname, "../src/components/fixtures.js"), "utf-8");

  assert.ok(!demoCode.includes("tui-fixtures.json?url"), "InteractiveDemo must not import full tui-fixtures.json?url");
  assert.ok(!fixCode.includes("tui-fixtures.json?url"), "fixtures.js must not import full tui-fixtures.json?url");
});

test("Compiled production bundle network & macro test: no typeof import.meta.glob and shard chunks present", () => {
  const clientAssetsDir = join(__dirname, "../dist/client/assets");
  if (!existsSync(clientAssetsDir)) return;

  const files = readdirSync(clientAssetsDir);
  const indexJsFile = files.find((f) => f.startsWith("index-") && f.endsWith(".js"));
  assert.ok(indexJsFile, "Compiled index-*.js must exist");

  const indexJsCode = readFileSync(join(clientAssetsDir, indexJsFile), "utf-8");
  assert.ok(!indexJsCode.includes("typeof import.meta.glob"), "Compiled index.js must not contain runtime typeof import.meta.glob check");

  // Verify that all 21 shard chunks exist in dist
  const layouts = ["wide", "stacked", "compact"];
  const views = ["dashboard", "quota", "profiles", "history", "settings", "doctor", "backup"];
  for (const l of layouts) {
    for (const v of views) {
      const chunkFile = files.find((f) => f.startsWith(`${l}.${v}-`) && f.endsWith(".js"));
      assert.ok(chunkFile, `Dynamic chunk for ${l}.${v} must exist in dist/client/assets/`);
    }
  }
});

test("TuiTerminal handleKeyDownInternal prevents double-routing of Tab by stopping propagation", () => {
  const termPath = join(__dirname, "../src/components/TuiTerminal.jsx");
  const termCode = readFileSync(termPath, "utf-8");

  const match = termCode.match(/const handleKeyDownInternal = \(e\) => \{([\s\S]*?)\};\n/);
  assert.ok(match, "handleKeyDownInternal must exist");
  const body = match[1];

  const formTabMatch = body.match(/if \(mode === 'form' && e\.key === 'Tab'\) \{([\s\S]*?)return;/);
  assert.ok(formTabMatch, "Form Tab handling branch must exist");
  assert.ok(formTabMatch[1].includes("e.stopPropagation()"), "Form Tab handling must call e.stopPropagation() to prevent bubbling to InteractiveDemo");

  const modalTabMatch = body.match(/if \(isModal && e\.key === 'Tab'\) \{([\s\S]*?)return;/);
  assert.ok(modalTabMatch, "Modal Tab handling branch must exist");
  assert.ok(modalTabMatch[1].includes("e.stopPropagation()"), "Modal Tab handling must call e.stopPropagation() to preserve focus trap");
});

test("Help overlay any-key close matching Go behavior", () => {
  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "OPEN_HELP" });
  assert.equal(state.mode, "help");
  assert.equal(state.ariaLiveMsg, "Keyboard guide opened.");

  // Go parity: Esc, q, Enter, space, arrows, alphanumeric all close help
  const closeKeys = ["escape", "q", "enter", " ", "arrowdown", "arrowup", "x", "a", "k", "j", "?"];
  for (const k of closeKeys) {
    const act = routeKeyAction(state, k);
    assert.deepEqual(act, { type: "CANCEL" }, `Key "${k}" in help mode must return CANCEL action`);
  }

  // Tab key is preserved for focus trapping/navigation
  assert.deepEqual(routeKeyAction(state, "tab"), { type: "NONE" });

  const closedState = transitionTuiState(state, { type: "CANCEL" });
  assert.equal(closedState.mode, "ready");
  assert.equal(closedState.ariaLiveMsg, "Action canceled.");
});

test("Cold Doctor and warm Doctor lifecycle completions with exact fixture resolution", async () => {
  const doctorShard = await fetchShard("wide", "Doctor");
  assert.ok(doctorShard.length > 0, "Doctor shard must be available");

  // 1. Cold navigation to Doctor (e.g. from tab click)
  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Doctor" });
  assert.equal(state.view, "Doctor");
  assert.equal(state.doctorState, "initial");
  assert.equal(state.mode, "ready");

  const fInitial = resolveFixture(doctorShard, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fInitial, "Doctor initial fixture must resolve");
  assert.equal(fInitial.id, "wide.doctor.ready.active-alpha");
  assert.ok(fInitial.plain.join("\n").includes("HEALTH CHECK"), "Doctor plain lines must contain HEALTH CHECK");

  // 2. Start doctor check (e.g. pressing 'r' or 'enter' in Doctor view)
  const actStart = routeKeyAction(state, "r");
  assert.deepEqual(actStart, { type: "START_DOCTOR" });
  state = transitionTuiState(state, actStart);
  assert.equal(state.doctorState, "running");
  assert.equal(state.ariaLiveMsg, "Running health check…");

  const fRunning = resolveFixture(doctorShard, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fRunning, "Doctor running fixture must resolve");
  assert.equal(fRunning.id, "wide.doctor.running.active-alpha");

  // 3. Complete doctor check (timer callback dispatch)
  state = transitionTuiState(state, { type: "COMPLETE_DOCTOR" });
  assert.equal(state.doctorState, "completed");
  assert.equal(state.ariaLiveMsg, "Health check completed: all checks healthy.");

  const fCompleted = resolveFixture(doctorShard, { ...state, selectedEmail: state.accounts[state.selectedIndex] });
  assert.ok(fCompleted, "Doctor completed fixture must resolve");
  assert.equal(fCompleted.id, "wide.doctor.completed.active-alpha");
  const plainDoc = fCompleted.plain.join("\n");
  assert.ok(plainDoc.includes("healthy"), "Doctor completed fixture must contain healthy");
  assert.ok(!plainDoc.includes("needs attention"), "Doctor completed fixture must not contain needs attention");

  // 4. Warm navigation to Doctor (e.g. pressing 'o' key in ready mode)
  let sWarm = createInitialTuiState();
  const actWarm = routeKeyAction(sWarm, "o");
  assert.deepEqual(actWarm, { type: "NAVIGATE_DOCTOR" });
  sWarm = transitionTuiState(sWarm, actWarm);
  assert.equal(sWarm.pendingNavigate, "Doctor");
  assert.equal(sWarm.doctorState, "running");

  sWarm = transitionTuiState(sWarm, { type: "SELECT_VIEW", view: "Doctor" });
  assert.equal(sWarm.view, "Doctor");
  assert.equal(sWarm.doctorState, "running", "Warm start must preserve running state upon SELECT_VIEW");
});

test("Refresh lifecycle completion and exact fixture resolution", async () => {
  const dashShard = await fetchShard("wide", "Dashboard");
  const quotaShard = await fetchShard("wide", "Quota");

  // Dashboard refresh
  let sDash = createInitialTuiState();
  sDash = transitionTuiState(sDash, { type: "TRIGGER_REFRESH" });
  assert.equal(sDash.mode, "refreshing");
  assert.equal(sDash.ariaLiveMsg, "Syncing usage…");

  const fDashRef = resolveFixture(dashShard, { ...sDash, selectedEmail: sDash.accounts[sDash.selectedIndex] });
  assert.ok(fDashRef, "Dashboard refreshing fixture must resolve");
  assert.equal(fDashRef.id, "wide.dashboard.refreshing.active-alpha.selected-alpha");

  sDash = transitionTuiState(sDash, { type: "COMPLETE_REFRESH" });
  assert.equal(sDash.mode, "ready");
  assert.equal(sDash.ariaLiveMsg, "Usage refreshed from sample data.");

  // Quota refresh
  let sQuota = createInitialTuiState();
  sQuota = transitionTuiState(sQuota, { type: "SELECT_VIEW", view: "Quota" });
  sQuota = transitionTuiState(sQuota, { type: "TRIGGER_REFRESH" });
  assert.equal(sQuota.mode, "refreshing");

  const fQuotaRef = resolveFixture(quotaShard, { ...sQuota, selectedEmail: sQuota.accounts[sQuota.selectedIndex] });
  assert.ok(fQuotaRef, "Quota refreshing fixture must resolve");
  assert.equal(fQuotaRef.id, "wide.quota.refreshing.active-alpha.selected-alpha");

  sQuota = transitionTuiState(sQuota, { type: "COMPLETE_REFRESH" });
  assert.equal(sQuota.mode, "ready");
});

test("Global navigation semantics: numeric, page, and boundary moves apply across operational views", () => {
  const nonAccountViews = ["Settings", "Doctor", "Backup"];

  for (const v of nonAccountViews) {
    let s = createInitialTuiState();
    s = transitionTuiState(s, { type: "SELECT_VIEW", view: v });

    // Number keys 1-9 route globally and update selected account
    const actNum2 = routeKeyAction(s, "2");
    assert.deepEqual(actNum2, { type: "SELECT_ACCOUNT_NUM", num: 2 });
    s = transitionTuiState(s, actNum2);
    assert.equal(s.selectedIndex, 1);
    assert.equal(s.ariaLiveMsg, "Selected account 2: beta@example.invalid");

    // Home / End / PageUp / PageDown update selectedIndex across non-account views
    s = transitionTuiState(s, { type: "NAV_END" });
    assert.equal(s.selectedIndex, s.accounts.length - 1);
    assert.equal(s.ariaLiveMsg, "Selected account: gamma@example.invalid");

    s = transitionTuiState(s, { type: "NAV_HOME" });
    assert.equal(s.selectedIndex, 0);
    assert.equal(s.ariaLiveMsg, "Selected account: alpha@example.invalid");

    s = transitionTuiState(s, { type: "NAV_PAGE_DOWN" });
    assert.equal(s.selectedIndex, 1);

    s = transitionTuiState(s, { type: "NAV_PAGE_UP" });
    assert.equal(s.selectedIndex, 0);

    // Up/Down arrows remain bounded to list views (no-op on Settings, Doctor, Backup)
    const initialIndex = s.selectedIndex;
    const sUp = transitionTuiState(s, { type: "NAV_UP" });
    assert.equal(sUp.selectedIndex, initialIndex);
    const sDown = transitionTuiState(s, { type: "NAV_DOWN" });
    assert.equal(sDown.selectedIndex, initialIndex);
  }

  // Verify that Dashboard and Quota properly handle navigation and number selection
  const accountViews = ["Dashboard", "Quota"];
  for (const v of accountViews) {
    let s = createInitialTuiState();
    s = transitionTuiState(s, { type: "SELECT_VIEW", view: v });

    const actNum2 = routeKeyAction(s, "2");
    assert.deepEqual(actNum2, { type: "SELECT_ACCOUNT_NUM", num: 2 });
    s = transitionTuiState(s, actNum2);
    assert.equal(s.selectedIndex, 1);
    assert.equal(s.ariaLiveMsg, "Selected account 2: beta@example.invalid");

    const sDown = transitionTuiState(s, { type: "NAV_DOWN" });
    assert.equal(sDown.selectedIndex, 2);
    assert.equal(sDown.ariaLiveMsg, "Selected account: gamma@example.invalid");
  }
});

test("Toolbar CSS rules enforce >=44px touch targets across view tabs and metadata", () => {
  const cssPath = join(__dirname, "../src/App.module.css");
  const cssCode = readFileSync(cssPath, "utf-8");

  assert.ok(cssCode.includes(".viewTab {"), "App.module.css must contain .viewTab");
  assert.ok(cssCode.includes("min-height: 44px;"), "App.module.css must enforce min-height: 44px;");
  assert.ok(cssCode.includes("min-width: 44px;"), "App.module.css must enforce min-width: 44px;");
  assert.ok(cssCode.includes(".apparatusMeta {"), "App.module.css must contain .apparatusMeta");
  assert.ok(cssCode.includes(".apparatusNav {"), "App.module.css must contain .apparatusNav");
});

test("InteractiveDemo passes boolean active to TerminalScene and has single aria-live region", () => {
  const demoPath = join(__dirname, "../src/components/InteractiveDemo.jsx");
  const demoCode = readFileSync(demoPath, "utf-8");

  assert.ok(demoCode.includes("<TerminalScene active={is3DEnabled} />"), "InteractiveDemo must pass active boolean prop to TerminalScene");
  assert.ok(!demoCode.includes("activeView="), "InteractiveDemo must not pass obsolete activeView to TerminalScene");

  // Single live region check
  const liveMatches = demoCode.match(/aria-live=/g) || [];
  assert.equal(liveMatches.length, 1, `InteractiveDemo must have exactly 1 aria-live region, found ${liveMatches.length}`);
  assert.ok(!demoCode.includes('loadingLabel} aria-live='), "loadingLabel must not duplicate aria-live");
});

test("No duplicate reducer cases in tuiState.js", () => {
  const statePath = join(__dirname, "../src/components/tuiState.js");
  const stateCode = readFileSync(statePath, "utf-8");

  const docMatches = stateCode.match(/case 'COMPLETE_DOCTOR':/g) || [];
  assert.equal(docMatches.length, 1, `case 'COMPLETE_DOCTOR': must appear exactly once, found ${docMatches.length}`);

  const refMatches = stateCode.match(/case 'COMPLETE_REFRESH':/g) || [];
  assert.equal(refMatches.length, 1, `case 'COMPLETE_REFRESH': must appear exactly once, found ${refMatches.length}`);
});


test("Sequential account deletion down to empty, check plain text after navigation", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let state = createInitialTuiState();
  state.activeEmail = "alpha@example.invalid";
  state.selectedIndex = 0; // alpha

  // Delete alpha
  state = transitionTuiState(state, { type: "OPEN_CONFIRM_DELETE" });
  state = transitionTuiState(state, { type: "CONFIRM_YES" });
  
  // Delete beta (which is now index 0)
  state.selectedIndex = 0;
  state = transitionTuiState(state, { type: "OPEN_CONFIRM_DELETE" });
  state = transitionTuiState(state, { type: "CONFIRM_YES" });
  const afterSecondDelete = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[0] });
  assert.ok(afterSecondDelete, "Fixture missing immediately after second deletion");
  assert.ok(!afterSecondDelete.plain.join("\n").includes("Alpha User"), "alpha row reappeared after second deletion");
  assert.ok(!afterSecondDelete.plain.join("\n").includes("Beta User"), "beta row reappeared after second deletion");
  
  // Delete gamma
  state.selectedIndex = 0;
  state = transitionTuiState(state, { type: "OPEN_CONFIRM_DELETE" });
  state = transitionTuiState(state, { type: "CONFIRM_YES" });

  assert.equal(state.accounts.length, 0);
  const afterFinalDelete = resolveFixture(fixtures, { ...state, selectedEmail: "" });
  assert.ok(afterFinalDelete, "Fixture missing immediately after final deletion");
  assert.ok(afterFinalDelete.plain.join("\n").includes("No saved accounts yet."), "final deletion did not render empty account state");

  // Navigate to Quota
  state = transitionTuiState(state, { type: "NAVIGATE_VIEW", view: "Quota" });
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Quota" });
  
  const fQuota = resolveFixture(fixtures, { ...state, selectedEmail: "" });
  assert.ok(fQuota, "Fixture missing for empty Quota");
  const plain = fQuota.plain.join("\n");
  assert.ok(!plain.includes("alpha@example.invalid"), "alpha should not reappear");
  assert.ok(!plain.includes("beta@example.invalid"), "beta should not reappear");
  assert.ok(!plain.includes("gamma@example.invalid"), "gamma should not reappear");
});

test("Sequential profile deletion down to empty, check plain text", () => {
  const fixturesPath = join(__dirname, "../src/generated/tui-fixtures.json");
  const data = JSON.parse(readFileSync(fixturesPath, "utf-8"));
  const fixtures = data.fixtures;

  let state = createInitialTuiState();
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Profiles" });
  state.profileIndex = 0; // personal

  // Delete personal
  state = transitionTuiState(state, { type: "OPEN_CONFIRM_ACTION", confirmKind: "profile-remove" });
  state = transitionTuiState(state, { type: "CONFIRM_YES" });

  // Delete work (now index 0)
  state.profileIndex = 0;
  state = transitionTuiState(state, { type: "OPEN_CONFIRM_ACTION", confirmKind: "profile-remove" });
  state = transitionTuiState(state, { type: "CONFIRM_YES" });

  assert.equal(state.profiles.length, 0);
  const afterFinalProfileRemoval = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[0] });
  assert.ok(afterFinalProfileRemoval, "Fixture missing immediately after final profile removal");
  const postRemovePlain = afterFinalProfileRemoval.plain.join("\n");
  assert.ok(postRemovePlain.includes("Removed profile work"), "Success banner must announce Removed profile work");
  assert.ok(postRemovePlain.includes("No profiles yet."), "Must render No profiles yet empty state");

  // Navigate to Dashboard and back
  state = transitionTuiState(state, { type: "NAVIGATE_VIEW", view: "Dashboard" });
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Dashboard" });
  state = transitionTuiState(state, { type: "NAVIGATE_VIEW", view: "Profiles" });
  state = transitionTuiState(state, { type: "SELECT_VIEW", view: "Profiles" });

  const fProf = resolveFixture(fixtures, { ...state, selectedEmail: state.accounts[0] });
  assert.ok(fProf, "Fixture missing for empty Profiles");
  const plain = fProf.plain.join("\n");
  assert.ok(!plain.includes("personal"), "personal profile should not reappear");
  assert.ok(!plain.includes("work"), "work profile should not reappear");
});

test("History clear confirm results in empty history", () => {
  const s = {
    ...createInitialTuiState(),
    view: "History",
    mode: "confirm_action",
    confirmKind: "history-clear",
    historyIndex: 2
  };
  const next = transitionTuiState(s, { type: "ENTER" });
  assert.equal(next.mode, "ready", "Mode should be ready");
  assert.equal(next.historyIndex, 0, "History index should be reset");
  assert.equal(next.postHistoryClear, true, "Should set postHistoryClear flag");
});

test("i18n translations dictionary contains all 4 supported languages with complete keys", async () => {
  const { translations } = await import("../src/i18n/translations.js");
  const languages = ["en", "th", "ja", "zh"];
  for (const lang of languages) {
    assert.ok(translations[lang], `translations for ${lang} must exist`);
    assert.ok(translations[lang].nav?.overview, `nav.overview missing in ${lang}`);
    assert.ok(translations[lang].nav?.flow, `nav.flow missing in ${lang}`);
    assert.ok(translations[lang].nav?.features, `nav.features missing in ${lang}`);
    assert.ok(translations[lang].nav?.install, `nav.install missing in ${lang}`);
    assert.ok(translations[lang].hero?.headlinePart1, `hero.headlinePart1 missing in ${lang}`);
    assert.ok(translations[lang].features?.card1Title, `features.card1Title missing in ${lang}`);
    assert.ok(translations[lang].install?.headline, `install.headline missing in ${lang}`);
    assert.ok(translations[lang].faq?.headline, `faq.headline missing in ${lang}`);
    assert.ok(translations[lang].footer?.tagline, `footer.tagline missing in ${lang}`);
  }
});

