

function getFormOptions(formKind, fieldIndex) {
  if (formKind === 'profile-create' || formKind === 'profile-edit') {
    if (fieldIndex === 2) return ['empty', 'claude', 'gemini', 'gpt'];
  }
  if (formKind === 'settings') {
    if (fieldIndex === 0) return ['sticky', 'balanced', 'round-robin'];
    if (fieldIndex === 1) return ['empty', 'claude', 'gemini', 'gpt'];
    if ([3, 4, 6, 7, 8, 10].includes(fieldIndex)) return ['off', 'on'];
  }
  if (formKind === 'binding' && fieldIndex === 2) return ['prompt', 'recommend', 'disabled'];
  if (formKind === 'backup-export' && fieldIndex === 1) return ['off', 'on'];
  if (formKind === 'backup-import' && fieldIndex === 1) return ['off', 'on'];
  return null;
}

function getFormDefault(state, opts) {
  if (state.formKind === 'profile-edit' && state.fieldIndex === 2) {
    return state.profileIndex === 0 ? 'gemini' : 'claude';
  }
  if (state.formKind === 'profile-create' && state.fieldIndex === 2) {
    return 'empty';
  }
  if (state.formKind === 'backup-import' && state.fieldIndex === 1) {
    return 'on'; // Merge is 'on' by default
  }
  if (state.formKind === 'settings' && state.fieldIndex === 10) {
    return 'on'; // history.enabled is 'on' by default
  }
  return opts[0];
}


import { ACCOUNTS, PROFILES, HISTORY_EVENTS, FORM_FIELD_COUNTS } from './fixtures.js';

export const VALID_SEARCH_QUERIES = [
  "", "a", "al", "alp", "alpha", "b", "be", "bet", "beta", "g", "ga", "gam", "gamma", "z", "zz"
];

export const VALID_PALETTE_QUERIES = [
  "", "p", "pr", "pro", "prof", "profile"
];

export function selectNextAccount(accounts, activeEmail) {
  if (!accounts || accounts.length === 0) return '';
  const activeIdx = accounts.indexOf(activeEmail);
  const startIdx = activeIdx >= 0 ? activeIdx : 0;
  for (let offset = 1; offset <= accounts.length; offset++) {
    const candidate = accounts[(startIdx + offset) % accounts.length];
    if (candidate === 'alpha@example.invalid' || candidate === 'beta@example.invalid') {
      return candidate;
    }
  }
  return accounts[(startIdx + 1) % accounts.length];
}

export function getPaletteActions(state) {
  const hasAccounts = state && state.accounts && state.accounts.length > 0;
  const view = state ? state.view : 'Dashboard';
  const profiles = state ? (Array.isArray(state.profiles) ? state.profiles : PROFILES.map((p) => p.name)) : [];
  const history = state ? (Array.isArray(state.history) ? state.history : HISTORY_EVENTS) : [];

  const rawActions = [
    { id: 'dashboard', label: 'Dashboard', description: 'Return to the account dashboard', shortcut: 'g', section: 'Navigate', enabled: true, view: 'Dashboard' },
    { id: 'quota', label: 'Quota overview', description: 'Inspect usage and reset windows', shortcut: 'v', section: 'Navigate', enabled: hasAccounts, view: 'Quota' },
    { id: 'profiles', label: 'Profiles', description: 'Create and manage named account profiles', shortcut: 'p', section: 'Navigate', enabled: true, view: 'Profiles' },
    { id: 'history', label: 'History', description: 'Review account switches and quota events', shortcut: 'h', section: 'Navigate', enabled: true, view: 'History' },
    { id: 'settings', label: 'Settings', description: 'Edit policy, notifications, and retention', shortcut: 's', section: 'Navigate', enabled: true, view: 'Settings' },
    { id: 'doctor', label: 'Run health check', description: 'Check storage, vault, session, and platform', shortcut: 'o', section: 'Tools', enabled: true, view: 'Doctor', startDoctor: true },
    { id: 'backup', label: 'Backup & restore', description: 'Export, import, or verify account data', shortcut: 'b', section: 'Tools', enabled: true, view: 'Backup' },
    { id: 'recommend', label: 'Recommend best account', description: 'Score accounts using policy, tags, and cooldowns', shortcut: '', section: 'Tools', enabled: hasAccounts, notice: 'Safe browser preview: account recommendation is enabled in native CLI.' },
    { id: 'forecast', label: 'Forecast quota', description: 'Show remaining capacity and confidence', shortcut: '', section: 'Tools', enabled: hasAccounts, notice: 'Safe browser preview: quota forecast is enabled in native CLI.' },
    { id: 'watch-once', label: 'Run notification check', description: 'Check thresholds once and notify when configured', shortcut: '', section: 'Tools', enabled: hasAccounts, notice: 'Safe browser preview: notifications are enabled in native CLI.' },
    { id: 'metrics', label: 'Export metrics', description: 'Print a Prometheus-compatible snapshot', shortcut: '', section: 'Tools', enabled: true, notice: 'Safe browser preview: Prometheus metrics export is enabled in native CLI.' },
    { id: 'run-now', label: 'Run AGY now', description: 'Launch the configured Antigravity CLI immediately', shortcut: '', section: 'Tools', enabled: true, notice: 'Safe browser preview: running external AGY is enabled in native CLI.' },
    { id: 'statusline-install', label: 'Install statusline hint', description: 'Save the statusline integration command', shortcut: '', section: 'Integrations', enabled: true, notice: 'Safe browser preview: statusline integration is enabled in native CLI.' },
    { id: 'completion', label: 'Print shell completion', description: 'Generate bash completion for setup', shortcut: '', section: 'Integrations', enabled: true, notice: 'Safe browser preview: shell completions are enabled in native CLI.' },
    { id: 'add-account', label: 'Add account', description: 'Sign in and save another account', shortcut: 'a', section: 'Accounts', enabled: true, notice: 'Safe browser preview: adding accounts is enabled in native CLI.' },
    { id: 'switch-account', label: 'Switch selected account', description: 'Make the selected account active', shortcut: 'enter', section: 'Accounts', enabled: hasAccounts, action: 'PERFORM_SWITCH' },
    { id: 'next-account', label: 'Choose next available', description: 'Pick the next healthy account', shortcut: 'n', section: 'Accounts', enabled: hasAccounts, action: 'SWITCH_NEXT' },
    { id: 'refresh', label: 'Refresh quota', description: 'Fetch fresh usage from the provider', shortcut: 'r', section: 'Accounts', enabled: hasAccounts, action: 'TRIGGER_REFRESH' },
    { id: 'edit-tags', label: 'Edit account tags', description: 'Add searchable labels to the selected account', shortcut: 'e', section: 'Accounts', enabled: hasAccounts, form: 'tags' },
    { id: 'toggle-tier', label: 'Toggle manual tier', description: 'Set or clear a local tier override', shortcut: 't', section: 'Accounts', enabled: hasAccounts, notice: 'Safe browser preview: tier toggle is enabled in native CLI.' },
    { id: 'migrate-vault', label: 'Migrate secrets to OS vault', description: 'Move legacy plaintext tokens into secure storage', shortcut: 'm', section: 'Security', enabled: hasAccounts, notice: 'Safe browser preview: secret vault migration is enabled in native CLI.' },
    { id: 'profile-create', label: 'Create profile', description: 'Save a named account preset', shortcut: 'c', section: 'Profiles', enabled: hasAccounts, form: 'profile-create' },
    { id: 'profile-edit', label: 'Edit selected profile', description: 'Change the selected profile', shortcut: 'e', section: 'Profiles', enabled: view === 'Profiles' && profiles.length > 0, form: 'profile-edit' },
    { id: 'profile-remove', label: 'Remove selected profile', description: 'Delete the selected profile', shortcut: 'd', section: 'Profiles', enabled: view === 'Profiles' && profiles.length > 0, confirm: 'profile-remove' },
    { id: 'history-clear', label: 'Clear history', description: 'Remove local action history', shortcut: 'c', section: 'History', enabled: view === 'History' && history.length > 0, confirm: 'history-clear' },
    { id: 'history-export', label: 'Export history', description: 'Write local history as JSON', shortcut: 'x', section: 'History', enabled: view === 'History' && history.length > 0, form: 'history-export' },
    { id: 'settings-edit', label: 'Edit settings', description: 'Change policy and notification defaults', shortcut: 'e', section: 'Settings', enabled: view === 'Settings', form: 'settings' },
    { id: 'settings-reset', label: 'Reset settings', description: 'Restore safe default policy and retention', shortcut: '', section: 'Settings', enabled: view === 'Settings', action: 'SETTINGS_RESET' },
    { id: 'alias-create', label: 'Create alias', description: 'Name an account or profile', shortcut: 'a', section: 'Settings', enabled: view === 'Settings', form: 'alias' },
    { id: 'binding-create', label: 'Create project binding', description: 'Bind a folder to a profile', shortcut: 'b', section: 'Settings', enabled: view === 'Settings', form: 'binding' },
    { id: 'target-create', label: 'Register target', description: 'Add a compatible CLI target', shortcut: 't', section: 'Settings', enabled: view === 'Settings', form: 'target' },
    { id: 'backup-export', label: 'Export backup', description: 'Write metadata-only or encrypted backup', shortcut: 'x', section: 'Backup', enabled: view === 'Backup', form: 'backup-export' },
    { id: 'backup-import', label: 'Import backup', description: 'Restore accounts and settings', shortcut: 'i', section: 'Backup', enabled: view === 'Backup', form: 'backup-import' },
    { id: 'backup-verify', label: 'Verify backup', description: 'Check a backup file without importing', shortcut: 'v', section: 'Backup', enabled: view === 'Backup', form: 'backup-verify' },
    { id: 'update-check', label: 'Check for update', description: 'See whether a matching release asset is available', shortcut: '', section: 'System', enabled: true, notice: 'Safe browser preview: agy-swap is already up to date.' },
    { id: 'update', label: 'Install latest update', description: 'Download, verify, and install the latest release', shortcut: 'u', section: 'System', enabled: true, confirm: 'update' },
    { id: 'quit', label: 'Quit', description: 'Close the interactive console', shortcut: 'q', section: 'System', enabled: true, notice: 'Safe browser preview: interactive demo remains open.' },
  ];

  const query = (state && state.paletteQuery ? state.paletteQuery : '').toLowerCase().trim();
  if (!query) return rawActions;

  return rawActions.filter((a) => {
    const text = `${a.label} ${a.description} ${a.section}`.toLowerCase();
    return text.includes(query);
  });
}

export function movePaletteIndex(items, currentIndex, delta) {
  if (!items || items.length === 0) return 0;
  let index = (currentIndex + delta) % items.length;
  if (index < 0) index += items.length;

  for (let i = 0; i < items.length; i++) {
    if (items[index]?.enabled) {
      return index;
    }
    index = (index + (delta >= 0 ? 1 : -1) + items.length) % items.length;
  }
  return currentIndex;
}

export function createInitialTuiState() {
  return {
    view: 'Dashboard',
    mode: 'ready',
    accounts: ACCOUNTS.map((a) => a.email),
    activeEmail: 'alpha@example.invalid',
    selectedIndex: 0,
    profiles: PROFILES.map((p) => p.name),
    profileIndex: 1,
    history: HISTORY_EVENTS,
    historyIndex: 0,
    postHistoryClear: false,
    formKind: 'settings',
    fieldIndex: 0,
    formFamilyOption: null,
    formEditVariant: null,
    confirmKind: '',
    paletteQuery: '',
    paletteIndex: 0,
    paletteFiltered: false,
    searchQuery: '',
    prevSearchQuery: '',
    prevSelectedIndex: 0,
    searchMatch: true,
    doctorState: 'initial',
    doctorHealthy: false,
    postDeleteResult: null,
    postRemoveProfileResult: null,
    pendingNavigate: null,
    ariaLiveMsg: '',
  };
}

/**
 * Pure router: maps raw keyboard input into typed application actions.
 */
export function routeKeyAction(state, keyName, e = {}) {
  const key = (keyName || '').toLowerCase();
  const ctrl = Boolean(e.ctrlKey || e.metaKey);

  if (state.mode === 'ready') {
    if (ctrl && (key === 'k' || key === 'K')) return { type: 'OPEN_PALETTE' };
    if (key === ':') return { type: 'OPEN_PALETTE' };
    if (key === '?') return { type: 'OPEN_HELP' };
    if (key === '/') return { type: 'OPEN_SEARCH' };
    if (key === 'p') return { type: 'NAVIGATE_VIEW', view: 'Profiles' };
    if (key === 'h') return { type: 'NAVIGATE_VIEW', view: 'History' };
    if (key === 's') return { type: 'NAVIGATE_VIEW', view: 'Settings' };
    if (key === 'o') return { type: 'NAVIGATE_DOCTOR' };
    if (key === 'b') {
      if (state.view === 'Settings') return { type: 'OPEN_FORM', formKind: 'binding' };
      if (state.view === 'Dashboard') return { type: 'NAVIGATE_VIEW', view: 'Backup' };
      return { type: 'NAVIGATE_VIEW', view: 'Dashboard' };
    }
    if (key === 'v') {
      if (state.view === 'Backup') return { type: 'OPEN_FORM', formKind: 'backup-verify' };
      return { type: 'NAVIGATE_VIEW', view: 'Quota' };
    }
    if (key === 'x') {
      if (state.view === 'Backup') return { type: 'OPEN_FORM', formKind: 'backup-export' };
      if (state.view === 'History') {
        if (!state.history || state.history.length === 0) return { type: 'NONE' };
        return { type: 'OPEN_FORM', formKind: 'history-export' };
      }
      return { type: 'NONE' };
    }
    if (key === 'i') {
      if (state.view === 'Backup') return { type: 'OPEN_FORM', formKind: 'backup-import' };
      return { type: 'NONE' };
    }
    if (key === 'e') {
      if (state.view === 'Settings') return { type: 'OPEN_FORM', formKind: 'settings' };
      if (state.view === 'Profiles') {
        if (!state.profiles || state.profiles.length === 0) return { type: 'NONE' };
        return { type: 'OPEN_FORM', formKind: 'profile-edit' };
      }
      if (state.view === 'Dashboard' || state.view === 'Quota') {
        if (!state.accounts || state.accounts.length === 0) return { type: 'NONE' };
        return { type: 'OPEN_FORM', formKind: 'tags' };
      }
      return { type: 'NONE' };
    }
    if (key === 'c') {
      if (state.view === 'Profiles') return { type: 'OPEN_FORM', formKind: 'profile-create' };
      if (state.view === 'History') {
        if (!state.history || state.history.length === 0) return { type: 'NONE' };
        return { type: 'OPEN_CONFIRM_ACTION', confirmKind: 'history-clear' };
      }
      return { type: 'NONE' };
    }
    if (key === 'a') {
      if (state.view === 'Settings') return { type: 'OPEN_FORM', formKind: 'alias' };
      return { type: 'ANNOUNCE_SAFE_NOTICE', msg: 'Safe browser preview: adding account is enabled in native CLI.' };
    }
    if (key === 't') {
      if (state.view === 'Settings') return { type: 'OPEN_FORM', formKind: 'target' };
      return { type: 'ANNOUNCE_SAFE_NOTICE', msg: 'Safe browser preview: manual tier toggle is enabled in native CLI.' };
    }
    if (key === 'd' || key === 'delete' || key === 'backspace') {
      if (state.view === 'Profiles') {
        if (!state.profiles || state.profiles.length === 0) return { type: 'NONE' };
        return { type: 'OPEN_CONFIRM_ACTION', confirmKind: 'profile-remove' };
      }
      if (state.view === 'Dashboard' || state.view === 'Quota') {
        if (!state.accounts || state.accounts.length === 0) return { type: 'NONE' };
        return { type: 'OPEN_CONFIRM_DELETE' };
      }
      return { type: 'NONE' };
    }
    if (key === 'r') {
      if (state.view === 'Doctor') return { type: 'START_DOCTOR' };
      if (state.view === 'Dashboard' || state.view === 'Quota') return { type: 'TRIGGER_REFRESH' };
      return { type: 'NONE' };
    }
    if (key === 'n') return { type: 'SWITCH_NEXT' };
    if (key === 'l') return { type: 'ANNOUNCE_SAFE_NOTICE', msg: 'Safe browser preview: logout is enabled in native CLI.' };
    if (key === 'm') return { type: 'ANNOUNCE_SAFE_NOTICE', msg: 'Safe browser preview: vault migration is enabled in native CLI.' };
    if (key === 'u') return { type: 'OPEN_CONFIRM_ACTION', confirmKind: 'update' };
    if (key === 'enter') return { type: 'ENTER' };
    if (key === 'arrowup' || key === 'k') return { type: 'NAV_UP' };
    if (key === 'arrowdown' || key === 'j') return { type: 'NAV_DOWN' };
    if (key === 'pageup') return { type: 'NAV_PAGE_UP' };
    if (key === 'pagedown') return { type: 'NAV_PAGE_DOWN' };
    if (key === 'home') return { type: 'NAV_HOME' };
    if (key === 'end') return { type: 'NAV_END' };
    if (key >= '1' && key <= '9') {
      return { type: 'SELECT_ACCOUNT_NUM', num: parseInt(key, 10) };
    }
    if (key === 'escape' || key === 'q') return { type: 'CANCEL' };
    return { type: 'NONE' };
  }

  if (state.mode === 'help') {
    if (key === 'tab') return { type: 'NONE' };
    return { type: 'CANCEL' };
  }

  if (state.mode === 'form') {
    if (key === 'arrowup' || (key === 'tab' && e.shiftKey)) return { type: 'NAV_UP' };
    if (key === 'arrowdown' || (key === 'tab' && !e.shiftKey)) return { type: 'NAV_DOWN' };
    if (key === 'arrowleft') return { type: 'NAV_LEFT' };
    if (key === 'arrowright') return { type: 'NAV_RIGHT' };
    if (key === 'enter') return { type: 'ENTER' };
    if (key === 'escape') return { type: 'CANCEL' };
    if (key === 'backspace') return { type: 'FORM_BACKSPACE' };
    if (ctrl && (key === 'u' || key === 'U')) return { type: 'FORM_CTRL_U' };
    if (ctrl && (key === 'w' || key === 'W')) return { type: 'FORM_CTRL_W' };
    if (key.length === 1 && !ctrl && !e.altKey) return { type: 'FORM_CHAR', char: key };
    return { type: 'NONE' };
  }

  if (state.mode === 'search') {
    if (key === 'escape') return { type: 'CANCEL' };
    if (key === 'enter') return { type: 'ENTER' };
    if (key === 'backspace') return { type: 'SEARCH_BACKSPACE' };
    if (ctrl && (key === 'u' || key === 'U')) return { type: 'SEARCH_CTRL_U' };
    if (ctrl && (key === 'w' || key === 'W')) return { type: 'SEARCH_CTRL_W' };
    if (key.length === 1 && !ctrl && !e.altKey) return { type: 'SEARCH_CHAR', char: key };
    return { type: 'NONE' };
  }

  if (state.mode === 'palette') {
    if (key === 'arrowup' || key === 'k') return { type: 'NAV_UP' };
    if (key === 'arrowdown' || key === 'j') return { type: 'NAV_DOWN' };
    if (key === 'pageup') return { type: 'NAV_PAGE_UP' };
    if (key === 'pagedown') return { type: 'NAV_PAGE_DOWN' };
    if (key === 'home') return { type: 'NAV_HOME' };
    if (key === 'end') return { type: 'NAV_END' };
    if (key === 'enter') return { type: 'ENTER' };
    if (key === 'escape') return { type: 'CANCEL' };
    if (key === 'backspace') return { type: 'PALETTE_BACKSPACE' };
    if (ctrl && (key === 'u' || key === 'U')) return { type: 'PALETTE_CTRL_U' };
    if (ctrl && (key === 'w' || key === 'W')) return { type: 'PALETTE_CTRL_W' };
    if (key.length === 1 && !ctrl && !e.altKey) return { type: 'PALETTE_CHAR', char: key };
    return { type: 'NONE' };
  }

  if (state.mode.startsWith('confirm')) {
    if (key === 'y' || key === 'enter') return { type: 'CONFIRM_YES' };
    if (key === 'n' || key === 'escape') return { type: 'CANCEL' };
    return { type: 'NONE' };
  }

  return { type: 'NONE' };
}

export function transitionTuiState(state, action) {
  const currentAccounts = state.accounts || [];
  const currentProfiles = state.profiles || [];
  const currentHistory = state.history || [];
  const safeSelectedIndex = Math.min(Math.max(0, state.selectedIndex || 0), Math.max(0, currentAccounts.length - 1));
  const safeProfileIndex = Math.min(Math.max(0, state.profileIndex || 0), Math.max(0, currentProfiles.length - 1));
  const selectedEmail = currentAccounts.length > 0 ? (currentAccounts[safeSelectedIndex] || currentAccounts[0]) : '';
  const selectedProfile = currentProfiles.length > 0 ? (currentProfiles[safeProfileIndex] || currentProfiles[0]) : '';

  switch (action.type) {
    case 'SELECT_VIEW': {
      const targetView = action.view;
      return {
        ...state,
        view: targetView,
        mode: 'ready',
        confirmKind: '',
        formEditVariant: null,
        formFamilyOption: null,
        postDeleteResult: null,
        postRemoveProfileResult: null,
        pendingNavigate: null,
        selectedIndex: safeSelectedIndex,
        profileIndex: safeProfileIndex,
        doctorState: (targetView === 'Doctor' && state.pendingNavigate === 'Doctor') ? state.doctorState : (targetView === 'Doctor' ? 'initial' : state.doctorState),
        doctorHealthy: (targetView === 'Doctor' && state.pendingNavigate === 'Doctor') ? Boolean(state.doctorHealthy) : false,
        ariaLiveMsg: `${targetView} view opened.`,
      };
    }

    case 'NAVIGATE_VIEW': {
      return {
        ...state,
        pendingNavigate: action.view,
      };
    }

    case 'NAVIGATE_DOCTOR': {
      return {
        ...state,
        pendingNavigate: 'Doctor',
        doctorState: 'running',
        doctorHealthy: false,
        ariaLiveMsg: 'Doctor health check running…',
      };
    }

    case 'START_DOCTOR': {
      return {
        ...state,
        doctorState: 'running',
        doctorHealthy: false,
        ariaLiveMsg: 'Running health check…',
      };
    }

    case 'NAV_UP': {
      if (state.mode === 'ready') {
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          const target = (safeProfileIndex - 1 + currentProfiles.length) % currentProfiles.length;
          return { ...state, profileIndex: target, postRemoveProfileResult: null, ariaLiveMsg: `Selected profile: ${currentProfiles[target]}` };
        } else if (state.view === 'History') {
          if (currentHistory.length === 0) return state;
          const target = (state.historyIndex - 1 + currentHistory.length) % currentHistory.length;
          return { ...state, historyIndex: target, ariaLiveMsg: `Selected history event ${target + 1}` };
        } else if (state.view === 'Dashboard' || state.view === 'Quota') {
          if (currentAccounts.length === 0) return state;
          const prev = (safeSelectedIndex - 1 + currentAccounts.length) % currentAccounts.length;
          return { ...state, selectedIndex: prev, postDeleteResult: null, ariaLiveMsg: `Selected account: ${currentAccounts[prev]}` };
        }
        return state;
      }
      if (state.mode === 'form') {
        const total = FORM_FIELD_COUNTS[state.formKind] || 1;
        const prev = (state.fieldIndex - 1 + total) % total;
        return {
          ...state,
          fieldIndex: prev,
          formEditVariant: null,
          formFamilyOption: null,
          ariaLiveMsg: `Field ${prev + 1} of ${total}`,
        };
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        const nextIdx = movePaletteIndex(items, state.paletteIndex, -1);
        return {
          ...state,
          paletteIndex: nextIdx,
          ariaLiveMsg: `Selected: ${items[nextIdx]?.label || 'Action'}`,
        };
      }
      return state;
    }

    case 'NAV_DOWN': {
      if (state.mode === 'ready') {
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          const target = (safeProfileIndex + 1) % currentProfiles.length;
          return { ...state, profileIndex: target, postRemoveProfileResult: null, ariaLiveMsg: `Selected profile: ${currentProfiles[target]}` };
        } else if (state.view === 'History') {
          if (currentHistory.length === 0) return state;
          const target = (state.historyIndex + 1) % currentHistory.length;
          return { ...state, historyIndex: target, ariaLiveMsg: `Selected history event ${target + 1}` };
        } else if (state.view === 'Dashboard' || state.view === 'Quota') {
          if (currentAccounts.length === 0) return state;
          const next = (safeSelectedIndex + 1) % currentAccounts.length;
          return { ...state, selectedIndex: next, postDeleteResult: null, ariaLiveMsg: `Selected account: ${currentAccounts[next]}` };
        }
        return state;
      }
      if (state.mode === 'form') {
        const total = FORM_FIELD_COUNTS[state.formKind] || 1;
        const next = (state.fieldIndex + 1) % total;
        return {
          ...state,
          fieldIndex: next,
          formEditVariant: null,
          formFamilyOption: null,
          ariaLiveMsg: `Field ${next + 1} of ${total}`,
        };
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        const nextIdx = movePaletteIndex(items, state.paletteIndex, 1);
        return {
          ...state,
          paletteIndex: nextIdx,
          ariaLiveMsg: `Selected: ${items[nextIdx]?.label || 'Action'}`,
        };
      }
      return state;
    }

    case 'NAV_LEFT': {
      if (state.mode === 'form') {
        const opts = getFormOptions(state.formKind, state.fieldIndex);
        if (opts) {
          const cur = state.formFamilyOption || getFormDefault(state, opts);
          const idx = opts.indexOf(cur) >= 0 ? opts.indexOf(cur) : 0;
          const prev = (idx - 1 + opts.length) % opts.length;
          return {
            ...state,
            formFamilyOption: opts[prev],
            formEditVariant: null,
            ariaLiveMsg: `Option: ${opts[prev]}`,
          };
        }
      }
      return state;
    }

    case 'NAV_RIGHT': {
      if (state.mode === 'form') {
        const opts = getFormOptions(state.formKind, state.fieldIndex);
        if (opts) {
          const cur = state.formFamilyOption || getFormDefault(state, opts);
          const idx = opts.indexOf(cur) >= 0 ? opts.indexOf(cur) : 0;
          const next = (idx + 1) % opts.length;
          return {
            ...state,
            formFamilyOption: opts[next],
            formEditVariant: null,
            ariaLiveMsg: `Option: ${opts[next]}`,
          };
        }
      }
      return state;
    }

    case 'NAV_PAGE_UP': {
      if (state.mode === 'ready') {
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          const target = Math.max(0, safeProfileIndex - 5);
          return { ...state, profileIndex: target, postRemoveProfileResult: null, ariaLiveMsg: `Selected profile: ${currentProfiles[target]}` };
        } else if (state.view === 'History') {
          if (currentHistory.length === 0) return state;
          const target = Math.max(0, state.historyIndex - 5);
          return { ...state, historyIndex: target, ariaLiveMsg: `Selected history event ${target + 1}` };
        } else {
          if (currentAccounts.length === 0) return state;
          const step = Math.max(1, Math.floor(currentAccounts.length / 2));
          const target = Math.max(0, safeSelectedIndex - step);
          return { ...state, selectedIndex: target, postDeleteResult: null, ariaLiveMsg: `Selected account: ${currentAccounts[target]}` };
        }
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        const nextIdx = movePaletteIndex(items, state.paletteIndex, -5);
        return {
          ...state,
          paletteIndex: nextIdx,
          ariaLiveMsg: `Selected: ${items[nextIdx]?.label || 'Action'}`,
        };
      }
      return state;
    }

    case 'NAV_PAGE_DOWN': {
      if (state.mode === 'ready') {
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          const target = Math.min(currentProfiles.length - 1, safeProfileIndex + 5);
          return { ...state, profileIndex: target, postRemoveProfileResult: null, ariaLiveMsg: `Selected profile: ${currentProfiles[target]}` };
        } else if (state.view === 'History') {
          if (currentHistory.length === 0) return state;
          const target = Math.min(currentHistory.length - 1, state.historyIndex + 5);
          return { ...state, historyIndex: target, ariaLiveMsg: `Selected history event ${target + 1}` };
        } else {
          if (currentAccounts.length === 0) return state;
          const step = Math.max(1, Math.floor(currentAccounts.length / 2));
          const target = Math.min(currentAccounts.length - 1, safeSelectedIndex + step);
          return { ...state, selectedIndex: target, postDeleteResult: null, ariaLiveMsg: `Selected account: ${currentAccounts[target]}` };
        }
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        const nextIdx = movePaletteIndex(items, state.paletteIndex, 5);
        return {
          ...state,
          paletteIndex: nextIdx,
          ariaLiveMsg: `Selected: ${items[nextIdx]?.label || 'Action'}`,
        };
      }
      return state;
    }

    case 'NAV_HOME': {
      if (state.mode === 'ready') {
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          const target = 0;
          return { ...state, profileIndex: target, postRemoveProfileResult: null, ariaLiveMsg: `Selected profile: ${currentProfiles[target]}` };
        } else if (state.view === 'History') {
          if (currentHistory.length === 0) return state;
          const target = 0;
          return { ...state, historyIndex: target, ariaLiveMsg: `Selected history event ${target + 1}` };
        } else {
          if (currentAccounts.length === 0) return state;
          return { ...state, selectedIndex: 0, postDeleteResult: null, ariaLiveMsg: `Selected account: ${currentAccounts[0]}` };
        }
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        const firstIdx = items.findIndex((a) => a.enabled);
        return {
          ...state,
          paletteIndex: firstIdx >= 0 ? firstIdx : 0,
          ariaLiveMsg: 'Top of palette',
        };
      }
      return state;
    }

    case 'NAV_END': {
      if (state.mode === 'ready') {
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          const target = Math.max(0, currentProfiles.length - 1);
          return { ...state, profileIndex: target, postRemoveProfileResult: null, ariaLiveMsg: `Selected profile: ${currentProfiles[target]}` };
        } else if (state.view === 'History') {
          if (currentHistory.length === 0) return state;
          const target = Math.max(0, currentHistory.length - 1);
          return { ...state, historyIndex: target, ariaLiveMsg: `Selected history event ${target + 1}` };
        } else {
          if (currentAccounts.length === 0) return state;
          const target = Math.max(0, currentAccounts.length - 1);
          return { ...state, selectedIndex: target, postDeleteResult: null, ariaLiveMsg: `Selected account: ${currentAccounts[target]}` };
        }
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        let lastIdx = items.length - 1;
        while (lastIdx >= 0 && !items[lastIdx]?.enabled) {
          lastIdx--;
        }
        return {
          ...state,
          paletteIndex: Math.max(0, lastIdx),
          ariaLiveMsg: 'Bottom of palette',
        };
      }
      return state;
    }

    case 'SELECT_ACCOUNT_NUM': {
      const idx = action.num - 1;
      if (state.mode === 'ready') {
        if (idx >= 0 && idx < currentAccounts.length) {
          return {
            ...state,
            selectedIndex: idx,
            postDeleteResult: null,
            ariaLiveMsg: `Selected account ${action.num}: ${currentAccounts[idx]}`,
          };
        }
      }
      return state;
    }

    case 'SWITCH_NEXT': {
      if (state.mode === 'ready') {
        if (currentAccounts.length === 0) return state;
        const nextEmail = selectNextAccount(currentAccounts, state.activeEmail);
        const nextIdx = currentAccounts.indexOf(nextEmail);
        return {
          ...state,
          selectedIndex: nextIdx >= 0 ? nextIdx : safeSelectedIndex,
          activeEmail: nextEmail,
          postDeleteResult: null,
          ariaLiveMsg: `Switched to ${nextEmail}.`,
        };
      }
      return state;
    }

    case 'ENTER': {
      if (state.mode === 'ready') {
        if (state.view === 'Dashboard' || state.view === 'Quota') {
          if (currentAccounts.length === 0) return state;
          return {
            ...state,
            activeEmail: selectedEmail,
            postDeleteResult: null,
            ariaLiveMsg: `Switched to ${selectedEmail}.`,
          };
        }
        if (state.view === 'Profiles') {
          if (currentProfiles.length === 0) return state;
          return {
            ...state,
            formKind: 'profile-edit',
            fieldIndex: 0,
            formFamilyOption: null,
            formEditVariant: null,
            mode: 'form',
            postRemoveProfileResult: null,
            ariaLiveMsg: `Editing profile ${selectedProfile}.`,
          };
        }
        if (state.view === 'Settings') {
          return {
            ...state,
            formKind: 'settings',
            fieldIndex: 0,
            formEditVariant: null,
            mode: 'form',
            ariaLiveMsg: 'Settings form opened.',
          };
        }
        if (state.view === 'Doctor') {
          return {
            ...state,
            doctorState: 'running',
            doctorHealthy: false,
            ariaLiveMsg: 'Running health check…',
          };
        }
      }
      if (state.mode === 'form') {
        const total = FORM_FIELD_COUNTS[state.formKind] || 1;
        if (state.fieldIndex < total - 1) {
          return {
            ...state,
            fieldIndex: state.fieldIndex + 1,
            formEditVariant: null,
            formFamilyOption: null,
            ariaLiveMsg: `Field ${state.fieldIndex + 2} of ${total}`,
          };
        }
        return {
          ...state,
          mode: 'ready',
          formEditVariant: null,
          ariaLiveMsg: `Saved ${state.formKind}.`,
        };
      }
      if (state.mode === 'confirm_delete') {
        const delEmail = selectedEmail;
        const remaining = currentAccounts.filter((e) => e !== delEmail);
        const newActive = state.activeEmail === delEmail ? '' : state.activeEmail;
        const newSelIdx = 0;
        return {
          ...state,
          accounts: remaining,
          activeEmail: newActive,
          selectedIndex: newSelIdx,
          postDeleteResult: { deletedEmail: delEmail, priorActiveEmail: state.activeEmail, view: state.view },
          mode: 'ready',
          ariaLiveMsg: `Removed account ${delEmail}.`,
        };
      }
      if (state.mode === 'confirm_action') {
        if (state.confirmKind === 'profile-remove') {
          const remProf = selectedProfile;
          const remaining = currentProfiles.filter((p) => p !== remProf);
          return {
            ...state,
            profiles: remaining,
            profileIndex: Math.max(0, Math.min(state.profileIndex, remaining.length - 1)),
            postRemoveProfileResult: { removedProfile: remProf },
            confirmKind: '',
            mode: 'ready',
            ariaLiveMsg: `Removed profile ${remProf}.`,
          };
        }
        if (state.confirmKind === 'history-clear') {
          return {
            ...state,
            history: [],
            historyIndex: 0,
            postHistoryClear: true,
            confirmKind: '',
            mode: 'ready',
            ariaLiveMsg: 'History cleared.',
          };
        }
        if (state.confirmKind === 'update') {
          return {
            ...state,
            confirmKind: '',
            mode: 'ready',
            ariaLiveMsg: 'Safe preview: agy-swap is already at the latest release v2.1.2.',
          };
        }
        return {
          ...state,
          confirmKind: '',
          mode: 'ready',
          ariaLiveMsg: 'Action confirmed.',
        };
      }
      if (state.mode === 'palette') {
        const items = getPaletteActions(state);
        const act = items[state.paletteIndex] || items[0];
        if (!act) return { ...state, mode: 'ready' };

        if (act.view) {
          return {
            ...state,
            mode: 'ready',
            pendingNavigate: act.view,
            doctorState: act.startDoctor ? 'running' : state.doctorState,
            doctorHealthy: act.startDoctor ? false : state.doctorHealthy,
            ariaLiveMsg: `Opening ${act.view}…`,
          };
        }
        if (act.form) {
          return transitionTuiState({ ...state, mode: 'ready' }, { type: 'OPEN_FORM', formKind: act.form });
        }
        if (act.confirm) {
          return transitionTuiState({ ...state, mode: 'ready' }, { type: 'OPEN_CONFIRM_ACTION', confirmKind: act.confirm });
        }
        if (act.action === 'SWITCH_NEXT') {
          return transitionTuiState({ ...state, mode: 'ready' }, { type: 'SWITCH_NEXT' });
        }
        if (act.action === 'TRIGGER_REFRESH') {
          return transitionTuiState({ ...state, mode: 'ready' }, { type: 'TRIGGER_REFRESH' });
        }
        if (act.action === 'PERFORM_SWITCH') {
          return transitionTuiState({ ...state, mode: 'ready' }, { type: 'ENTER' });
        }
        if (act.action === 'SETTINGS_RESET') {
          return {
            ...state,
            mode: 'ready',
            ariaLiveMsg: 'Settings reset in safe preview.',
          };
        }
        return {
          ...state,
          mode: 'ready',
          ariaLiveMsg: act.notice || 'Palette action closed in safe preview.',
        };
      }
      if (state.mode === 'search') {
        return {
          ...state,
          mode: 'ready',
          ariaLiveMsg: state.searchMatch ? `Search applied: ${state.searchQuery}` : 'No matching accounts',
        };
      }
      return state;
    }

    case 'CONFIRM_YES': {
      return transitionTuiState(state, { type: 'ENTER' });
    }

    case 'CANCEL': {
      if (state.mode === 'search') {
        return {
          ...state,
          mode: 'ready',
          searchQuery: state.prevSearchQuery,
          selectedIndex: state.prevSelectedIndex,
          ariaLiveMsg: 'Search canceled.',
        };
      }
      return {
        ...state,
        mode: 'ready',
        confirmKind: '',
        formFamilyOption: null,
        formEditVariant: null,
        ariaLiveMsg: 'Action canceled.',
      };
    }

    case 'OPEN_HELP': {
      return {
        ...state,
        mode: 'help',
        postDeleteResult: null,
        postRemoveProfileResult: null,
        ariaLiveMsg: 'Keyboard guide opened.',
      };
    }

    case 'OPEN_SEARCH': {
      return {
        ...state,
        mode: 'search',
        searchMatch: true,
        searchQuery: '',
        prevSearchQuery: state.searchQuery || '',
        prevSelectedIndex: safeSelectedIndex,
        postDeleteResult: null,
        postRemoveProfileResult: null,
        ariaLiveMsg: 'Search opened.',
      };
    }

    case 'SEARCH_CHAR': {
      if (state.mode === 'search') {
        const nextQ = (state.searchQuery || '') + action.char.toLowerCase();
        const isValid = VALID_SEARCH_QUERIES.some((v) => v === nextQ || v.startsWith(nextQ));
        if (!isValid) {
          return {
            ...state,
            ariaLiveMsg: 'Safe browser preview supports searching alpha, beta, gamma, or z; full search is enabled in native CLI.',
          };
        }

        // Clamp selection to match
        const matchingIdx = currentAccounts.findIndex((e) => e.toLowerCase().includes(nextQ));
        const matchFound = matchingIdx >= 0;
        return {
          ...state,
          searchQuery: nextQ,
          searchMatch: matchFound,
          selectedIndex: matchFound ? matchingIdx : safeSelectedIndex,
          ariaLiveMsg: matchFound ? `Search matched: ${currentAccounts[matchingIdx]}` : 'No matching accounts',
        };
      }
      return state;
    }

    case 'SEARCH_BACKSPACE': {
      if (state.mode === 'search') {
        const nextQ = (state.searchQuery || '').slice(0, -1);
        const matchingIdx = currentAccounts.findIndex((e) => e.toLowerCase().includes(nextQ));
        const matchFound = matchingIdx >= 0 || nextQ === '';
        return {
          ...state,
          searchQuery: nextQ,
          searchMatch: matchFound,
          selectedIndex: matchingIdx >= 0 ? matchingIdx : safeSelectedIndex,
          ariaLiveMsg: nextQ ? `Search: ${nextQ}` : 'Search cleared',
        };
      }
      return state;
    }

    case 'SEARCH_CTRL_U': {
      if (state.mode === 'search') {
        return {
          ...state,
          searchQuery: '',
          searchMatch: true,
          ariaLiveMsg: 'Search query cleared.',
        };
      }
      return state;
    }

    case 'SEARCH_CTRL_W': {
      if (state.mode === 'search') {
        const q = (state.searchQuery || '').trimEnd();
        const lastIdx = q.lastIndexOf(' ');
        const nextQ = lastIdx >= 0 ? q.slice(0, lastIdx) : '';
        const matchingIdx = currentAccounts.findIndex((e) => e.toLowerCase().includes(nextQ));
        const matchFound = matchingIdx >= 0 || nextQ === '';
        return {
          ...state,
          searchQuery: nextQ,
          searchMatch: matchFound,
          selectedIndex: matchingIdx >= 0 ? matchingIdx : safeSelectedIndex,
          ariaLiveMsg: nextQ ? `Search: ${nextQ}` : 'Search cleared',
        };
      }
      return state;
    }

    case 'OPEN_PALETTE': {
      return {
        ...state,
        mode: 'palette',
        paletteQuery: action.query || '',
        paletteFiltered: action.filtered || Boolean(action.query),
        paletteIndex: 0,
        postDeleteResult: null,
        postRemoveProfileResult: null,
        ariaLiveMsg: 'Action palette opened.',
      };
    }

    case 'PALETTE_CHAR': {
      if (state.mode === 'palette') {
        const nextQ = (state.paletteQuery || '') + action.char.toLowerCase();
        const isValid = VALID_PALETTE_QUERIES.some((v) => v === nextQ || v.startsWith(nextQ));
        if (!isValid) {
          return {
            ...state,
            ariaLiveMsg: 'Safe preview supports filtering profile/p actions; full filter is enabled in native CLI.',
          };
        }
        return {
          ...state,
          paletteQuery: nextQ,
          paletteIndex: 0,
          paletteFiltered: true,
          ariaLiveMsg: `Filtered: ${nextQ}`,
        };
      }
      return state;
    }

    case 'PALETTE_BACKSPACE': {
      if (state.mode === 'palette') {
        const nextQ = (state.paletteQuery || '').slice(0, -1);
        return {
          ...state,
          paletteQuery: nextQ,
          paletteIndex: 0,
          paletteFiltered: Boolean(nextQ),
          ariaLiveMsg: nextQ ? `Filtered: ${nextQ}` : 'All actions',
        };
      }
      return state;
    }

    case 'PALETTE_CTRL_U': {
      if (state.mode === 'palette') {
        return {
          ...state,
          paletteQuery: '',
          paletteIndex: 0,
          paletteFiltered: false,
          ariaLiveMsg: 'Palette filter cleared.',
        };
      }
      return state;
    }

    case 'PALETTE_CTRL_W': {
      if (state.mode === 'palette') {
        const q = (state.paletteQuery || '').trimEnd();
        const lastIdx = q.lastIndexOf(' ');
        const nextQ = lastIdx >= 0 ? q.slice(0, lastIdx) : '';
        return {
          ...state,
          paletteQuery: nextQ,
          paletteIndex: 0,
          paletteFiltered: Boolean(nextQ),
          ariaLiveMsg: nextQ ? `Filtered: ${nextQ}` : 'All actions',
        };
      }
      return state;
    }

    case 'OPEN_FORM': {
      if (action.formKind === 'profile-edit' && currentProfiles.length === 0) {
        return state;
      }
      if (action.formKind === 'history-export' && currentHistory.length === 0) {
        return state;
      }
      return {
        ...state,
        mode: 'form',
        formKind: action.formKind,
        fieldIndex: 0,
        formFamilyOption: null,
        formEditVariant: null,
        postDeleteResult: null,
        postRemoveProfileResult: null,
        ariaLiveMsg: `${action.formKind} form opened.`,
      };
    }

    case 'FORM_BACKSPACE': {
      if (state.mode === 'form') {
        const nextVariant = state.formEditVariant === 'char-x' ? null : 'backspace';
        return {
          ...state,
          formEditVariant: nextVariant,
          ariaLiveMsg: 'Backspace in form field.',
        };
      }
      return state;
    }

    case 'FORM_CTRL_U': {
      if (state.mode === 'form') {
        return {
          ...state,
          formEditVariant: 'ctrl-u',
          ariaLiveMsg: 'Cleared form field with Ctrl-U.',
        };
      }
      return state;
    }

    case 'FORM_CTRL_W': {
      if (state.mode === 'form') {
        return {
          ...state,
          formEditVariant: 'ctrl-w',
          ariaLiveMsg: 'Deleted word in form field with Ctrl-W.',
        };
      }
      return state;
    }

    case 'FORM_CHAR': {
      if (state.mode === 'form') {
        if (action.char === 'x' || action.char === 'X') {
          return {
            ...state,
            formEditVariant: 'char-x',
            ariaLiveMsg: 'Entered x in form field.',
          };
        }
        return {
          ...state,
          ariaLiveMsg: 'Safe browser preview supports typing x; full editing is enabled in native CLI.',
        };
      }
      return state;
    }

    case 'OPEN_CONFIRM_DELETE': {
      return {
        ...state,
        mode: 'confirm_delete',
        ariaLiveMsg: `Confirm delete ${selectedEmail}`,
      };
    }

    case 'OPEN_CONFIRM_ACTION': {
      if (action.confirmKind === 'profile-remove' && currentProfiles.length === 0) {
        return state;
      }
      if (action.confirmKind === 'history-clear' && currentHistory.length === 0) {
        return state;
      }
      return {
        ...state,
        mode: 'confirm_action',
        confirmKind: action.confirmKind,
        ariaLiveMsg: `Confirm ${action.confirmKind}`,
      };
    }

    case 'TRIGGER_REFRESH': {
      return {
        ...state,
        mode: 'refreshing',
        ariaLiveMsg: 'Syncing usage…',
      };
    }

    case 'COMPLETE_REFRESH': {
      return {
        ...state,
        mode: 'ready',
        ariaLiveMsg: 'Usage refreshed from sample data.',
      };
    }

    case 'COMPLETE_DOCTOR': {
      return {
        ...state,
        doctorState: 'completed',
        doctorHealthy: true,
        ariaLiveMsg: 'Health check completed: all checks healthy.',
      };
    }

    case 'ANNOUNCE_SAFE_NOTICE': {
      return {
        ...state,
        ariaLiveMsg: action.msg,
      };
    }

    default:
      return state;
  }
}
