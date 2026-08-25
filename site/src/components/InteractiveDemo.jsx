import React, { useState, useReducer, useRef, Suspense, useMemo, useCallback, useEffect } from 'react';
import styles from '../App.module.css';
import TuiTerminal from './TuiTerminal';
import AmbientDust from './AmbientDust';
import { useTranslation } from '../i18n/I18nContext';
import { VIEWS, ACCOUNTS, PROFILES, HISTORY_EVENTS, FORM_FIELD_COUNTS, resolveFixture, setGlobalTuiFixture } from './fixtures.js';
import { createInitialTuiState, transitionTuiState, routeKeyAction } from './tuiState.js';
import { createShardManager } from './shardManager.js';
import initialFixturesData from '../generated/tui-initial-fixtures.json';

export { VIEWS, ACCOUNTS, PROFILES, HISTORY_EVENTS, FORM_FIELD_COUNTS, resolveFixture } from './fixtures.js';
export { createInitialTuiState, transitionTuiState, routeKeyAction } from './tuiState.js';

const TerminalScene = React.lazy(() => import('./TerminalScene'));

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false };
  }
  static getDerivedStateFromError() {
    return { hasError: true };
  }
  componentDidCatch() {
    if (this.props.onError) this.props.onError();
  }
  render() {
    if (this.state.hasError) return null;
    return this.props.children;
  }
}

export default function InteractiveDemo({ is3DEnabled, on3DError }) {
  const { t } = useTranslation();
  const shardManagerRef = useRef(null);
  if (!shardManagerRef.current) {
    shardManagerRef.current = createShardManager();
  }

  const isMountedRef = useRef(true);
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const [shardRevision, setShardRevision] = useState(0);
  const [pendingView, setPendingView] = useState(null);
  const [state, dispatch] = useReducer(transitionTuiState, null, createInitialTuiState);
  const [layout, setLayout] = useState('wide');

  const containerRef = useRef(null);
  const tuiRef = useRef(null);
  const lastVisibleFixtureRef = useRef(null);
  const pendingActionRef = useRef(null);

  // Responsive layout selection with shard pre-check
  const handleApertureCapacity = useCallback(({ cols, rows }) => {
    let nextLayout = 'stacked';
    if (cols >= 92 && rows >= 18) {
      nextLayout = 'wide';
    } else if (cols < 64 || rows < 16) {
      nextLayout = 'compact';
    }

    if (nextLayout === layout) return;

    const sm = shardManagerRef.current;
    if (state.view === 'Dashboard' && !sm.hasFullShard(layout, 'Dashboard')) {
      setLayout(nextLayout);
    } else if (sm.hasFullShard(nextLayout, state.view)) {
      setLayout(nextLayout);
    } else {
      sm.ensureShard(nextLayout, state.view)
        .then(() => {
          if (!isMountedRef.current) return;
          setShardRevision((r) => r + 1);
          setLayout(nextLayout);
        })
        .catch(() => {});
    }
  }, [layout, state.view]);

  // Transactional view navigation: ensures target shard is fully loaded before dispatching SELECT_VIEW
  const openView = useCallback((targetView) => {
    if (state.view === targetView && !pendingView) return;

    const sm = shardManagerRef.current;
    if (sm.hasFullShard(layout, targetView)) {
      setPendingView(null);
      dispatch({ type: 'SELECT_VIEW', view: targetView });
      return;
    }

    setPendingView(targetView);
    const reqId = sm.nextRequestId();

    sm.ensureShard(layout, targetView)
      .then(() => {
        if (!isMountedRef.current || !sm.isCurrentRequest(reqId)) return;
        setShardRevision((r) => r + 1);
        setPendingView(null);
        dispatch({ type: 'SELECT_VIEW', view: targetView });
      })
      .catch(() => {
        if (isMountedRef.current && sm.isCurrentRequest(reqId)) {
          setPendingView(null);
          dispatch({ type: 'ANNOUNCE_SAFE_NOTICE', msg: `Failed to load ${targetView} view.` });
        }
      });
  }, [layout, state.view, pendingView]);

  useEffect(() => {
    if (state.pendingNavigate) {
      openView(state.pendingNavigate);
    }
  }, [state.pendingNavigate, openView]);

  useEffect(() => {
    let timer;
    if (state.mode === 'refreshing') {
      timer = setTimeout(() => {
        if (isMountedRef.current) {
          dispatch({ type: 'COMPLETE_REFRESH' });
        }
      }, 750);
    }
    return () => clearTimeout(timer);
  }, [state.mode]);

  useEffect(() => {
    let timer;
    if (state.doctorState === 'running') {
      timer = setTimeout(() => {
        if (isMountedRef.current) {
          dispatch({ type: 'COMPLETE_DOCTOR' });
        }
      }, 750);
    }
    return () => clearTimeout(timer);
  }, [state.doctorState]);

  const safeAccounts = state.accounts || [];
  const safeSelectedIndex = Math.min(Math.max(0, state.selectedIndex || 0), Math.max(0, safeAccounts.length - 1));
  const selectedEmail = safeAccounts.length > 0 ? (safeAccounts[safeSelectedIndex] || safeAccounts[0]) : '';

  const currentShardFixtures = useMemo(() => {
    const sm = shardManagerRef.current;
    const loaded = sm.getLoadedShard(layout, state.view);
    if (loaded && loaded.length > 0) return loaded;
    return initialFixturesData.fixtures || [];
  }, [layout, state.view, shardRevision]);

  const currentFixture = useMemo(() => {
    const resolved = resolveFixture(currentShardFixtures, {
      layout,
      view: state.view,
      mode: state.mode,
      accounts: state.accounts,
      profiles: state.profiles,
      history: state.history,
      activeEmail: state.activeEmail,
      selectedEmail,
      profileIndex: state.profileIndex,
      historyIndex: state.historyIndex,
      formKind: state.formKind,
      fieldIndex: state.fieldIndex,
      formFamilyOption: state.formFamilyOption,
      formEditVariant: state.formEditVariant,
      confirmKind: state.confirmKind,
      paletteQuery: state.paletteQuery,
      paletteIndex: state.paletteIndex,
      paletteFiltered: state.paletteFiltered,
      searchQuery: state.searchQuery,
      searchMatch: state.searchMatch,
      doctorState: state.doctorState,
      doctorHealthy: state.doctorHealthy,
      postDeleteResult: state.postDeleteResult,
      postRemoveProfileResult: state.postRemoveProfileResult,
      postHistoryClear: state.postHistoryClear,
    });

    if (resolved) {
      lastVisibleFixtureRef.current = resolved;
      return resolved;
    }
    return lastVisibleFixtureRef.current || currentShardFixtures[0] || initialFixturesData.fixtures[0];
  }, [
    currentShardFixtures,
    layout,
    state.view,
    state.mode,
    state.accounts,
    state.profiles,
    state.history,
    state.activeEmail,
    selectedEmail,
    state.profileIndex,
    state.historyIndex,
    state.formKind,
    state.fieldIndex,
    state.formFamilyOption,
    state.formEditVariant,
    state.confirmKind,
    state.paletteQuery,
    state.paletteIndex,
    state.paletteFiltered,
    state.searchQuery,
    state.searchMatch,
    state.doctorState,
    state.doctorHealthy,
    state.postDeleteResult,
    state.postRemoveProfileResult,
    state.postHistoryClear,
  ]);

  useEffect(() => {
    if (currentFixture) {
      setGlobalTuiFixture(currentFixture);
    }
  }, [currentFixture]);

  const isModal = state.mode !== 'ready' && state.mode !== 'refreshing';

  // Transactional execution of in-view terminal interactions
  const handleKeyDown = (e) => {
    const action = routeKeyAction(state, e.key, e);

    if (action.type === 'NONE') return;

    if (action.type === 'NAVIGATE_VIEW') {
      e.preventDefault();
      e.stopPropagation();
      openView(action.view);
      return;
    }

    if (action.type === 'NAVIGATE_DOCTOR') {
      e.preventDefault();
      e.stopPropagation();
      dispatch(action);
      return;
    }

    const sm = shardManagerRef.current;

    // If current view full shard is already loaded, dispatch immediately
    if (sm.hasFullShard(layout, state.view)) {
      e.preventDefault();
      e.stopPropagation();
      dispatch(action);
      return;
    }

    // First interaction on current view before full shard load: transactional capture & replay
    e.preventDefault();
    e.stopPropagation();

    if (pendingActionRef.current) {
      return;
    }

    pendingActionRef.current = action;
    setPendingView(state.view);
    const reqId = sm.nextRequestId();

    sm.ensureShard(layout, state.view)
      .then(() => {
        if (!isMountedRef.current || !sm.isCurrentRequest(reqId)) return;
        setShardRevision((r) => r + 1);
        setPendingView(null);
        const actToReplay = pendingActionRef.current;
        pendingActionRef.current = null;
        if (actToReplay) {
          dispatch(actToReplay);
        }
      })
      .catch(() => {
        if (isMountedRef.current && sm.isCurrentRequest(reqId)) {
          setPendingView(null);
          pendingActionRef.current = null;
          dispatch({ type: 'ANNOUNCE_SAFE_NOTICE', msg: `Failed to load ${state.view} data.` });
        }
      });
  };

  const STAGES = [
    {
      view: 'Dashboard',
      step: '01',
      title: t('demo.stage1Title', 'Sub-Millisecond Account Rotation'),
      desc: t('demo.stage1Desc', 'Swap unlimited Google accounts directly in terminal without losing flow.'),
    },
    {
      view: 'Quota',
      step: '02',
      title: t('demo.stage2Title', 'Live Gemini Quota Monitor'),
      desc: t('demo.stage2Desc', 'Real-time model capacity meter and rate-limit cooldown countdowns.'),
    },
    {
      view: 'Profiles',
      step: '03',
      title: t('demo.stage3Title', 'Hardware OS Keychain Security'),
      desc: t('demo.stage3Desc', 'Credentials stay encrypted in local OS Enclave with directory isolation.'),
    },
  ];

  const trackRef = useRef(null);
  const [activeStageIndex, setActiveStageIndex] = useState(0);

  useEffect(() => {
    let animId = null;

    const handleScroll = () => {
      if (!trackRef.current) return;
      const rect = trackRef.current.getBoundingClientRect();
      const windowHeight = window.innerHeight || document.documentElement.clientHeight;
      const totalScrollable = rect.height - windowHeight;
      const currentScroll = -rect.top;
      const rawProgress = totalScrollable > 0 ? currentScroll / totalScrollable : 0;
      const clamped = Math.max(0, Math.min(1, rawProgress));

      let nextStage = 0;
      if (clamped >= 0.66) {
        nextStage = 2;
      } else if (clamped >= 0.33) {
        nextStage = 1;
      } else {
        nextStage = 0;
      }

      setActiveStageIndex(nextStage);
    };

    const onScroll = () => {
      if (animId) cancelAnimationFrame(animId);
      animId = requestAnimationFrame(handleScroll);
    };

    window.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', onScroll, { passive: true });
    handleScroll();

    return () => {
      if (animId) cancelAnimationFrame(animId);
      window.removeEventListener('scroll', onScroll);
      window.removeEventListener('resize', onScroll);
    };
  }, []);

  useEffect(() => {
    const targetView = STAGES[activeStageIndex]?.view;
    if (targetView && state.view !== targetView) {
      openView(targetView);
    }
  }, [activeStageIndex, openView, state.view]);

  const currentStage = STAGES[activeStageIndex] || STAGES[0];

  return (
    <div className={styles.fullscreenPinTrack} ref={trackRef}>
      <div className={styles.stickyPinStage} ref={containerRef}>
        {/* Apple-Style Floating Story Headline */}
        <div className={styles.floatingStoryBadge} role="region" aria-label="Showcase view">
          <div className={styles.storyBadgeHeader}>
            <span className={styles.storyStepNum}>{currentStage.step}</span>
            <span className={styles.storyStepDivider}>/</span>
            <span className={styles.storyStepView}>{currentStage.view}</span>
          </div>
          <h3 className={styles.storyStepTitle}>{currentStage.title}</h3>
          <p className={styles.storyStepDesc}>{currentStage.desc}</p>
          <div className={styles.storyStepDots} aria-hidden="true">
            {STAGES.map((s, idx) => (
              <button
                key={s.view}
                type="button"
                className={`${styles.stepDot} ${activeStageIndex === idx ? styles.activeDot : ''}`}
                onClick={() => openView(s.view)}
                aria-label={`Go to ${s.title}`}
              />
            ))}
          </div>
        </div>

        <div className={styles.deviceFrame} data-layout={layout} data-3d-enabled={is3DEnabled ? 'true' : 'false'}>
          <AmbientDust />

          {is3DEnabled && (
            <ErrorBoundary onError={on3DError}>
              <Suspense fallback={null}>
                <TerminalScene active={is3DEnabled} />
              </Suspense>
            </ErrorBoundary>
          )}

          <div className={styles.terminalOverlay}>
            <TuiTerminal
              tuiRef={tuiRef}
              fixture={currentFixture}
              mode={state.mode}
              isModal={isModal}
              ariaLabel={t('demo.terminalAria', 'Interactive agy-swap terminal. Use arrow keys or numbers to select account, Enter to switch, ? for help.')}
              ariaBusy={pendingView ? 'true' : 'false'}
              onAction={dispatch}
              onKeyDown={handleKeyDown}
              onApertureCapacity={handleApertureCapacity}
            />
          </div>
        </div>

        <div className={styles.srOnly} aria-live="polite">
          {state.ariaLiveMsg}
        </div>
      </div>
    </div>
  );
}
