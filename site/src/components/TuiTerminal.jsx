import React, { useState, useEffect, useLayoutEffect, useRef, useCallback } from 'react';
import styles from '../App.module.css';
import { parseAnsi } from './ansi.js';

export { xterm256ToHex, parseAnsi } from './ansi.js';

/**
 * Fixed character-cell TUI terminal renderer.
 * Measures actual rendered intrinsic dimensions untransformed, reports cell capacity,
 * uniformly scales to fit the aperture, and implements modal Tab-trapping.
 */
export default function TuiTerminal({
  fixture,
  mode = 'ready',
  onAction,
  onKeyDown,
  tuiRef,
  className,
  ariaLabel,
  ariaBusy,
  isModal = false,
  onApertureCapacity,
}) {
  const containerRef = useRef(null);
  const screenRef = useRef(null);
  const [scale, setScale] = useState(1);
  const [isMeasured, setIsMeasured] = useState(false);
  const lastReportedRef = useRef({ cols: 0, rows: 0 });

  const updateMetrics = useCallback(() => {
    if (!containerRef.current || !screenRef.current) return;
    const containerW = containerRef.current.clientWidth;
    const containerH = containerRef.current.clientHeight;
    const screenW = screenRef.current.offsetWidth;
    const screenH = screenRef.current.offsetHeight;

    if (containerW > 0 && containerH > 0 && screenW > 0 && screenH > 0) {
      const fixtureCols = fixture?.dimensions?.cols || 100;
      const fixtureRows = fixture?.dimensions?.rows || 24;

      const charW = screenW / fixtureCols;
      const charH = screenH / fixtureRows;

      if (charW > 0 && charH > 0 && onApertureCapacity) {
        const capCols = Math.floor(containerW / charW);
        const capRows = Math.floor(containerH / charH);
        if (
          capCols !== lastReportedRef.current.cols ||
          capRows !== lastReportedRef.current.rows
        ) {
          lastReportedRef.current = { cols: capCols, rows: capRows };
          onApertureCapacity({ cols: capCols, rows: capRows });
        }
      }

      const scaleX = containerW / screenW;
      const scaleY = containerH / screenH;
      const computedScale = Math.min(scaleX, scaleY) * 0.98;
      setScale(Math.max(0.1, computedScale));
      setIsMeasured(true);
    }
  }, [fixture, onApertureCapacity]);

  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new ResizeObserver(() => {
      updateMetrics();
    });
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, [updateMetrics]);

  useLayoutEffect(() => {
    updateMetrics();
  }, [updateMetrics]);

  const handleKeyDownInternal = (e) => {
    if (mode === 'form' && e.key === 'Tab') {
      e.preventDefault();
      e.stopPropagation();
      if (onAction) {
        onAction({ type: e.shiftKey ? 'NAV_UP' : 'NAV_DOWN' });
      }
      return;
    }
    if (isModal && e.key === 'Tab') {
      e.preventDefault();
      e.stopPropagation();
      containerRef.current?.focus();
      return;
    }
    if (onKeyDown) {
      onKeyDown(e);
    }
  };

  const lines = fixture?.lines || [];
  const plain = fixture?.plain || [];

  return (
    <div
      id="tui-terminal"
      ref={(node) => {
        containerRef.current = node;
        if (tuiRef) {
          if (typeof tuiRef === 'function') tuiRef(node);
          else tuiRef.current = node;
        }
      }}
      className={`${styles.tuiContainer} ${className || ''}`}
      tabIndex={0}
      role={isModal ? 'dialog' : 'region'}
      aria-modal={isModal ? 'true' : undefined}
      aria-label={ariaLabel || 'Interactive TUI Terminal Preview'}
      aria-busy={ariaBusy || 'false'}
      onKeyDown={handleKeyDownInternal}
      onClick={() => {
        containerRef.current?.focus();
      }}
      style={{
        width: '100%',
        height: '100%',
        position: 'relative',
        overflow: 'hidden',
        minWidth: 0,
        maxWidth: '100%',
        boxSizing: 'border-box',
        cursor: 'default',
      }}
    >
      {/* Visual character-cell terminal */}
      <pre
        ref={screenRef}
        aria-hidden="true"
        className={styles.tuiScreen}
        style={{
          position: 'absolute',
          left: '50%',
          top: '50%',
          transform: `translate(-50%, -50%) scale(${scale})`,
          transformOrigin: 'center center',
          margin: 0,
          padding: 0,
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
          fontSize: '13px',
          lineHeight: '1.25',
          whiteSpace: 'pre',
          color: '#b0b5bd',
          background: 'transparent',
          pointerEvents: 'auto',
          userSelect: 'none',
          width: 'max-content',
          height: 'max-content',
          opacity: isMeasured ? 1 : 0,
          transition: 'opacity 0.08s ease-in',
        }}
      >
        {lines.map((line, rowIndex) => {
          const tokens = parseAnsi(line);
          const isAlpha = line.includes('alpha@') || line.includes('Alpha');
          const isBeta = line.includes('beta@') || line.includes('Beta');
          const isGamma = line.includes('gamma@') || line.includes('Gamma');
          const isClickableAccount = (isAlpha || isBeta || isGamma) && mode === 'ready';

          const handleRowClick = () => {
            if (containerRef.current) {
              containerRef.current.focus();
            }
            if (!onAction) return;
            if (isAlpha) onAction({ type: 'SELECT_ACCOUNT_NUM', num: 1 });
            else if (isBeta) onAction({ type: 'SELECT_ACCOUNT_NUM', num: 2 });
            else if (isGamma) onAction({ type: 'SELECT_ACCOUNT_NUM', num: 3 });
          };

          return (
            <span
              key={`row-${rowIndex}`}
              onClick={isClickableAccount ? handleRowClick : undefined}
              style={{
                display: 'block',
                whiteSpace: 'pre',
                pointerEvents: isClickableAccount ? 'auto' : 'none',
                cursor: isClickableAccount ? 'pointer' : 'default',
              }}
            >
              {tokens.map((tok, tokIdx) => {
                const style = {};
                if (tok.bold) style.fontWeight = 'bold';
                if (tok.color) style.color = tok.color;
                return Object.keys(style).length > 0 ? (
                  <span key={`t-${rowIndex}-${tokIdx}`} style={style}>
                    {tok.text}
                  </span>
                ) : (
                  tok.text
                );
              })}
            </span>
          );
        })}
      </pre>

      {/* Screen-reader accessible representation */}
      <div className={styles.srOnly}>
        <pre>{plain.join('\n')}</pre>
      </div>
    </div>
  );
}
