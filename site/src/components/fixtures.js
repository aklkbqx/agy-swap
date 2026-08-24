export const VIEWS = ['Dashboard', 'Quota', 'Profiles', 'History', 'Settings', 'Doctor', 'Backup'];

export const ACCOUNTS = [
  { email: 'alpha@example.invalid', name: 'Alpha User' },
  { email: 'beta@example.invalid', name: 'Beta User' },
  { email: 'gamma@example.invalid', name: 'Gamma User' },
];

export const PROFILES = [
  { name: 'personal', account: 'alpha@example.invalid', family: 'gemini' },
  { name: 'work', account: 'beta@example.invalid', family: 'claude' },
];

export const HISTORY_EVENTS = [
  { kind: 'switch', email: 'alpha@example.invalid' },
  { kind: 'quota-refresh', email: 'beta@example.invalid' },
  { kind: 'profile-apply', email: 'gamma@example.invalid' },
];

export const FORM_FIELD_COUNTS = {
  'profile-create': 4,
  'profile-edit': 4,
  'tags': 1,
  'settings': 13,
  'alias': 2,
  'binding': 3,
  'target': 2,
  'history-export': 1,
  'backup-export': 3,
  'backup-import': 3,
  'backup-verify': 2,
};

const SHORT_NAMES = {
  '': 'none',
  'alpha@example.invalid': 'alpha',
  'beta@example.invalid': 'beta',
  'gamma@example.invalid': 'gamma',
};

export function getAccountContext(accounts) {
  if (!accounts || accounts.length === 0) return 'accounts-empty';
  if (accounts.length >= 3) return '';
  const s = new Set(accounts);
  if (accounts.length === 2) {
    if (!s.has('gamma@example.invalid')) return 'accounts-no-gamma';
    if (!s.has('beta@example.invalid')) return 'accounts-no-beta';
    if (!s.has('alpha@example.invalid')) return 'accounts-no-alpha';
  }
  if (accounts.length === 1) {
    if (s.has('alpha@example.invalid')) return 'accounts-only-alpha';
    if (s.has('beta@example.invalid')) return 'accounts-only-beta';
    if (s.has('gamma@example.invalid')) return 'accounts-only-gamma';
  }
  return '';
}

export function getProfileContext(profiles) {
  if (!profiles || profiles.length === 0) return 'profiles-empty';
  if (profiles.length >= 2) return '';
  if (!profiles.includes('work')) return 'profiles-no-work';
  if (!profiles.includes('personal')) return 'profiles-no-personal';
  return '';
}

let currentGlobalFixture = null;
const fixtureListeners = new Set();

export function setGlobalTuiFixture(fixture) {
  currentGlobalFixture = fixture;
  for (const listener of fixtureListeners) {
    try {
      listener(fixture);
    } catch (e) {}
  }
}

export function getGlobalTuiFixture() {
  return currentGlobalFixture;
}

export function subscribeTuiFixture(listener) {
  fixtureListeners.add(listener);
  if (currentGlobalFixture) {
    try {
      listener(currentGlobalFixture);
    } catch (e) {}
  }
  return () => {
    fixtureListeners.delete(listener);
  };
}

const shardCache = new Map();

export async function fetchShard(layout, view) {
  const key = `${layout}.${view.toLowerCase()}`;
  if (shardCache.has(key)) {
    return shardCache.get(key);
  }

  // Node.js test environment check
  if (typeof process !== 'undefined' && process.versions?.node && !process.env.VITE_TEST_BROWSER) {
    try {
      const fsMod = 'node:fs';
      const pathMod = 'node:path';
      const urlMod = 'node:url';
      const { readFileSync } = await import(/* @vite-ignore */ fsMod);
      const { join, dirname } = await import(/* @vite-ignore */ pathMod);
      const { fileURLToPath } = await import(/* @vite-ignore */ urlMod);
      const curDir = dirname(fileURLToPath(import.meta.url));
      const shardPath = join(curDir, `../generated/shards/${key}.json`);
      const data = JSON.parse(readFileSync(shardPath, 'utf-8'));
      const fixtures = data.fixtures;
      if (!Array.isArray(fixtures) || fixtures.length === 0) {
        throw new Error(`Shard ${key} contained no fixtures in node test`);
      }
      shardCache.set(key, fixtures);
      return fixtures;
    } catch (err) {
      throw err;
    }
  }

  // Browser / Vite runtime: loads through statically analyzed shardLoaders module
  const { loadBrowserShard } = await import('./shardLoaders.js');
  const fixtures = await loadBrowserShard(key);
  if (!Array.isArray(fixtures) || fixtures.length === 0) {
    throw new Error(`Shard ${key} contained no fixtures in browser runtime`);
  }
  shardCache.set(key, fixtures);
  return fixtures;
}

/**
 * Resolves the exact fixture from the loaded fixture bank matching current application state.
 * Returns null if the exact fixture is unavailable in the currently loaded bank.
 */
export function resolveFixture(fixtures, {
  layout = 'wide',
  view = 'Dashboard',
  mode = 'ready',
  accounts = ['alpha@example.invalid', 'beta@example.invalid', 'gamma@example.invalid'],
  profiles = ['personal', 'work'],
  history = HISTORY_EVENTS,
  activeEmail = 'alpha@example.invalid',
  selectedEmail = 'alpha@example.invalid',
  profileIndex = 1,
  historyIndex = 0,
  formKind = 'settings',
  fieldIndex = 0,
  formFamilyOption = null,
  formEditVariant = null,
  confirmKind = '',
  paletteQuery = '',
  paletteIndex = 0,
  paletteFiltered = false,
  searchQuery = '',
  searchMatch = true,
  doctorState = 'initial',
  doctorHealthy = false,
  postDeleteResult = null,
  postRemoveProfileResult = null,
  postHistoryClear = false,
}) {
  if (!fixtures || fixtures.length === 0) return null;

  const actShort = (activeEmail === '' || activeEmail === undefined) ? 'none' : (SHORT_NAMES[activeEmail] || 'alpha');
  const selShort = SHORT_NAMES[selectedEmail] || 'alpha';
  const viewLower = view.toLowerCase();
  const accountCtx = getAccountContext(accounts);
  const profileCtx = getProfileContext(profiles);
  const canUsePostDeleteResult = Boolean(postDeleteResult);
  const canUsePostRemoveProfileResult = Boolean(postRemoveProfileResult);

  const infix = accountCtx ? `.${accountCtx}.active-${actShort}` : `.active-${actShort}`;

  let targetId = '';

  if (mode === 'help') {
    if (view === 'Dashboard' || view === 'Quota') {
      if (accountCtx === 'accounts-empty') {
        targetId = `${layout}.${viewLower}.help${infix}`;
      } else {
        targetId = `${layout}.${viewLower}.help${infix}.selected-${selShort}`;
      }
    } else {
      targetId = `${layout}.${viewLower}.help${infix}`;
    }
  } else if (mode === 'palette') {
    const qSlug = paletteQuery ? paletteQuery.toLowerCase() : 'empty';
    if (view === 'Dashboard' || view === 'Quota') {
      if (accountCtx === 'accounts-empty') {
        targetId = `${layout}.${viewLower}.palette.query-${qSlug}.index-${paletteIndex}${infix}`;
      } else {
        targetId = `${layout}.${viewLower}.palette.query-${qSlug}.index-${paletteIndex}${infix}.selected-${selShort}`;
      }
    } else {
      targetId = `${layout}.${viewLower}.palette.query-${qSlug}.index-${paletteIndex}${infix}`;
    }
  } else if (mode === 'search') {
    const qSlug = searchQuery ? searchQuery.toLowerCase() : 'empty';
    if (view === 'Dashboard' || view === 'Quota') {
      if (accountCtx === 'accounts-empty') {
        targetId = `${layout}.${viewLower}.search.query-${qSlug}${infix}`;
      } else {
        targetId = `${layout}.${viewLower}.search.query-${qSlug}${infix}.selected-${selShort}`;
      }
    } else {
      targetId = `${layout}.${viewLower}.search.query-${qSlug}${infix}`;
    }
  } else if (mode === 'confirm_delete') {
    targetId = `${layout}.${viewLower}.confirm-delete.account-${selShort}${infix}`;
  } else if (mode === 'confirm_action') {
    if (confirmKind === 'profile-remove') {
      targetId = `${layout}.profiles.confirm-remove${infix}`;
    } else if (confirmKind === 'update') {
      targetId = `${layout}.${viewLower}.confirm-update${infix}`;
    } else {
      targetId = `${layout}.history.confirm-action${infix}`;
    }
  } else if (mode === 'form') {
    const editSuffix = formEditVariant ? `.edit-${formEditVariant}` : '';
    const optSuffix = formFamilyOption ? `.option-${formFamilyOption}` : '';
    const fieldSuffix = editSuffix || optSuffix;

    if (view === 'Dashboard' || view === 'Quota') {
      targetId = `${layout}.${viewLower}.form.tags.field-0${fieldSuffix}${infix}`;
    } else if (formKind === 'profile-edit') {
      targetId = `${layout}.profiles.form.profile-edit.profile-${profileIndex}.field-${fieldIndex}${fieldSuffix}${infix}`;
    } else if (formKind === 'history-export') {
      targetId = `${layout}.history.form.history-export.field-0${fieldSuffix}${infix}`;
    } else if (formKind === 'settings') {
      targetId = `${layout}.settings.form.settings.field-${fieldIndex}${fieldSuffix}${infix}`;
    } else {
      targetId = `${layout}.${viewLower}.form.${formKind}.field-${fieldIndex}${fieldSuffix}${infix}`;
    }
  } else if (mode === 'refreshing') {
    if (accountCtx === 'accounts-empty') {
      targetId = `${layout}.${viewLower}.refreshing${infix}`;
    } else {
      targetId = `${layout}.${viewLower}.refreshing${infix}.selected-${selShort}`;
    }
  } else {
    // Ready mode
    if (canUsePostDeleteResult && (postDeleteResult.view === 'Dashboard' || postDeleteResult.view === 'Quota') && (view === 'Dashboard' || view === 'Quota')) {
      const delShort = SHORT_NAMES[postDeleteResult.deletedEmail] || 'beta';
      const priorAccountCtx = getAccountContext([...accounts, postDeleteResult.deletedEmail]);
      const priorInfix = priorAccountCtx ? `.${priorAccountCtx}.active-${actShort}` : `.active-${actShort}`;
      targetId = `${layout}.${viewLower}.post-delete.deleted-${delShort}${priorInfix}`;
    } else if (canUsePostRemoveProfileResult && view === 'Profiles') {
      const remProf = postRemoveProfileResult.removedProfile || 'work';
      const pCtxStr = profileCtx === 'profiles-empty' ? '.profiles-empty' : '';
      targetId = `${layout}.profiles.post-remove.${remProf}${pCtxStr}${infix}`;
    } else if (view === 'Dashboard') {
      if (!accountCtx && activeEmail === 'beta@example.invalid' && selectedEmail === 'beta@example.invalid') {
        targetId = `${layout}.dashboard.switched.account-beta`;
      } else if (!accountCtx && activeEmail === 'gamma@example.invalid' && selectedEmail === 'gamma@example.invalid') {
        targetId = `${layout}.dashboard.switched.account-gamma`;
      } else if (accountCtx === 'accounts-empty') {
        targetId = `${layout}.dashboard.ready${infix}`;
      } else {
        targetId = `${layout}.dashboard.ready${infix}.selected-${selShort}`;
      }
    } else if (view === 'Quota') {
      if (accountCtx === 'accounts-empty') {
        targetId = `${layout}.quota.ready${infix}`;
      } else {
        targetId = `${layout}.quota.ready${infix}.selected-${selShort}`;
      }
    } else if (view === 'Profiles') {
      if (profileCtx) {
        targetId = `${layout}.profiles.ready.${profileCtx}${infix}`;
      } else {
        targetId = `${layout}.profiles.ready.profile-${profileIndex}${infix}`;
      }
    } else if (view === 'History') {
      if (postHistoryClear || (Array.isArray(history) && history.length === 0)) {
        targetId = `${layout}.history.post-clear${infix}`;
      } else {
        targetId = `${layout}.history.ready.index-${historyIndex}${infix}`;
      }
    } else if (view === 'Settings') {
      targetId = `${layout}.settings.ready${infix}`;
    } else if (view === 'Doctor') {
      const docState = (doctorState === 'running' || doctorState === 'completed') ? doctorState : 'ready';
      targetId = `${layout}.doctor.${docState}${infix}`;
    } else if (view === 'Backup') {
      targetId = `${layout}.backup.ready${infix}`;
    }
  }

  const found = fixtures.find((f) => f.id === targetId);
  if (found) return found;

  // Specific targeted aliases
  if (mode === 'palette') {
    const palType = paletteFiltered ? 'filtered' : 'unfiltered';
    const altPal = fixtures.find((f) => f.id === `${layout}.${viewLower}.palette.${palType}${infix}` || f.id.includes(`.palette.${palType}`));
    if (altPal) return altPal;
  }

  if (mode === 'search') {
    const sType = searchMatch ? 'match' : 'no-match';
    const altSearch = fixtures.find((f) => f.id === `${layout}.${viewLower}.search.${sType}${infix}` || f.id.includes(`.search.${sType}`));
    if (altSearch) return altSearch;
  }



  if (mode === 'form' && formKind === 'profile-edit') {
    const foundProfForm = fixtures.find((f) => f.id === `${layout}.profiles.form.profile-edit.field-${fieldIndex}${infix}`);
    if (foundProfForm) return foundProfForm;
  }



  if (accountCtx) {
    const altId = targetId.replace(`.${accountCtx}`, '');
    const foundAlt = fixtures.find((f) => f.id === altId);
    if (foundAlt) return foundAlt;
  }

  const foundCanonical = fixtures.find((f) => f.id === `${layout}.${viewLower}.ready.active-alpha.selected-alpha` || f.id === `${layout}.${viewLower}.ready`);
  if (foundCanonical && viewLower === 'dashboard') return foundCanonical;

  return null;
}
