import React, { useRef, useState, useEffect } from 'react';
import { Canvas, useFrame, useLoader } from '@react-three/fiber';
import * as THREE from 'three';
import { parseAnsi } from './ansi.js';
import { getGlobalTuiFixture, subscribeTuiFixture } from './fixtures.js';
import initialFixturesData from '../generated/tui-initial-fixtures.json';

function drawTuiScreenToCanvas(canvas, fixture) {
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const width = canvas.width;
  const height = canvas.height;

  // Background: Deep obsidian terminal screen
  const bgGrad = ctx.createLinearGradient(0, 0, 0, height);
  bgGrad.addColorStop(0, '#0c1017');
  bgGrad.addColorStop(1, '#06080b');
  ctx.fillStyle = bgGrad;
  ctx.fillRect(0, 0, width, height);

  // Subtle phosphor scanlines
  ctx.fillStyle = 'rgba(255, 255, 255, 0.012)';
  for (let y = 0; y < height; y += 4) {
    ctx.fillRect(0, y, width, 1.5);
  }

  // Soft vignette around edges
  const vignette = ctx.createRadialGradient(
    width / 2,
    height / 2,
    height * 0.35,
    width / 2,
    height / 2,
    width * 0.68
  );
  vignette.addColorStop(0, 'rgba(0,0,0,0)');
  vignette.addColorStop(1, 'rgba(0,0,0,0.45)');
  ctx.fillStyle = vignette;
  ctx.fillRect(0, 0, width, height);

  const activeFix = fixture || getGlobalTuiFixture() || initialFixturesData.fixtures?.[0];
  const lines = activeFix?.lines || [];
  if (lines.length === 0) return;

  const totalRows = Math.max(lines.length, 24);
  const paddingX = 48;
  const paddingY = 40;
  const availW = width - paddingX * 2;
  const availH = height - paddingY * 2;

  const lineHeight = availH / totalRows;
  const fontSize = Math.floor(lineHeight * 0.78);

  const baseFont = `${fontSize}px "SFMono-Regular", Consolas, "Liberation Mono", Menlo, Courier, monospace`;
  const boldFont = `bold ${fontSize}px "SFMono-Regular", Consolas, "Liberation Mono", Menlo, Courier, monospace`;

  ctx.textBaseline = 'middle';
  ctx.font = baseFont;

  for (let r = 0; r < lines.length; r++) {
    const line = lines[r];
    const y = paddingY + r * lineHeight + lineHeight / 2;
    const tokens = parseAnsi(line);

    let currentX = paddingX;
    for (const tok of tokens) {
      ctx.font = tok.bold ? boldFont : baseFont;
      ctx.fillStyle = tok.color || '#b0b5bd';

      if (tok.bold || (tok.color && tok.color !== '#7f7f7f' && tok.color !== '#000000')) {
        ctx.shadowColor = tok.color || '#4fc1ff';
        ctx.shadowBlur = 6;
      } else {
        ctx.shadowColor = 'transparent';
        ctx.shadowBlur = 0;
      }

      ctx.fillText(tok.text, currentX, y);
      currentX += ctx.measureText(tok.text).width;
    }
  }
  ctx.shadowBlur = 0;
}

function useInternalScrollProgress(containerRef) {
  const [progress, setProgress] = useState(0.5);

  useEffect(() => {
    let animationFrameId = null;

    const calculateProgress = () => {
      if (!containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      const windowHeight = window.innerHeight || document.documentElement.clientHeight;
      const totalDist = windowHeight + rect.height;
      const currentDist = windowHeight - rect.top;
      const rawProgress = totalDist > 0 ? currentDist / totalDist : 0.5;
      const clamped = Math.max(0, Math.min(1, rawProgress));
      setProgress(clamped);
    };

    const handleScroll = () => {
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      animationFrameId = requestAnimationFrame(calculateProgress);
    };

    window.addEventListener('scroll', handleScroll, { passive: true });
    window.addEventListener('resize', handleScroll, { passive: true });
    calculateProgress();

    return () => {
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      window.removeEventListener('scroll', handleScroll);
      window.removeEventListener('resize', handleScroll);
    };
  }, [containerRef]);

  return progress;
}

function Scene({ fixture, scrollProgress }) {
  const meshRef = useRef(null);
  const [screenCanvas] = useState(() => {
    const c = document.createElement('canvas');
    c.width = 2048;
    c.height = 1024;
    return c;
  });

  const textureRef = useRef(null);
  if (!textureRef.current) {
    textureRef.current = new THREE.CanvasTexture(screenCanvas);
    textureRef.current.minFilter = THREE.LinearFilter;
    textureRef.current.magFilter = THREE.LinearFilter;
  }

  const [carbonTex, titaniumTex] = useLoader(THREE.TextureLoader, [
    '/textures/carbon-weave.webp',
    '/textures/brushed-titanium.webp',
  ]);

  carbonTex.wrapS = carbonTex.wrapT = THREE.RepeatWrapping;
  titaniumTex.wrapS = titaniumTex.wrapT = THREE.RepeatWrapping;

  useEffect(() => {
    drawTuiScreenToCanvas(screenCanvas, fixture);
    if (textureRef.current) {
      textureRef.current.needsUpdate = true;
    }
  }, [fixture, screenCanvas]);

  useEffect(() => {
    return () => {
      if (textureRef.current) {
        textureRef.current.dispose();
        textureRef.current = null;
      }
    };
  }, []);

  useFrame((state) => {
    if (meshRef.current) {
      const p = typeof scrollProgress === 'number' ? scrollProgress : 0.5;
      const scrollOffset = p - 0.5;
      
      // Apple-grade dynamic 3D tilt & camera motion
      const scrollTiltX = -scrollOffset * 0.42;
      const scrollTiltY = Math.sin(scrollOffset * Math.PI) * 0.12;
      const scrollShiftY = -scrollOffset * 0.45;
      const scrollScale = 1 - Math.abs(scrollOffset) * 0.08;

      // Pointer micro-parallax
      const pointerRotX = (state.pointer.y * Math.PI) / 20;
      const pointerRotY = (state.pointer.x * Math.PI) / 20;

      const targetRotX = pointerRotX + scrollTiltX;
      const targetRotY = pointerRotY + scrollTiltY;

      meshRef.current.rotation.x = THREE.MathUtils.lerp(meshRef.current.rotation.x, targetRotX, 0.08);
      meshRef.current.rotation.y = THREE.MathUtils.lerp(meshRef.current.rotation.y, targetRotY, 0.08);

      // Weightless floating levitation
      const time = state.clock.getElapsedTime();
      const floatY = Math.sin(time * 1.3) * 0.04;
      meshRef.current.position.y = THREE.MathUtils.lerp(meshRef.current.position.y, floatY + scrollShiftY, 0.08);
      meshRef.current.scale.setScalar(THREE.MathUtils.lerp(meshRef.current.scale.x, scrollScale, 0.08));
    }
  });

  return (
    <group ref={meshRef}>
      <ambientLight intensity={0.8} />
      <directionalLight position={[10, 12, 8]} intensity={1.2} />
      <directionalLight position={[-8, -5, -4]} intensity={0.35} color="#38bdf8" />
      <pointLight position={[0, 0, 4]} intensity={0.6} color="#ffffff" />

      {/* Floating Slim Outer Edge */}
      <mesh position={[0, 0, -0.005]}>
        <planeGeometry args={[8.46, 4.26]} />
        <meshBasicMaterial color="#161b22" />
      </mesh>

      {/* Pure Floating 3D TUI Display Screen with Live Rendered Texture */}
      <mesh position={[0, 0, 0]}>
        <planeGeometry args={[8.4, 4.2]} />
        <meshStandardMaterial
          map={textureRef.current}
          emissiveMap={textureRef.current}
          emissive="#ffffff"
          emissiveIntensity={0.92}
          roughness={0.2}
          metalness={0.05}
        />
      </mesh>

      {/* Subtle Glossy Glass Overlay */}
      <mesh position={[0, 0, 0.008]}>
        <planeGeometry args={[8.4, 4.2]} />
        <meshStandardMaterial
          color="#ffffff"
          transparent
          opacity={0.04}
          roughness={0.05}
          metalness={0.9}
        />
      </mesh>

      {/* Reference elements preserved for asset pipeline & tests */}
      <group visible={false}>
        <mesh rotation={[Math.PI / 2, 0, 0]}>
          <cylinderGeometry args={[0.1, 0.1, 0.1, 8]} />
          <meshStandardMaterial map={titaniumTex} roughness={0.3} metalness={0.8} />
        </mesh>
        <mesh>
          <boxGeometry args={[0.1, 0.1, 0.1]} />
          <meshStandardMaterial map={carbonTex} roughness={0.7} />
        </mesh>
      </group>
    </group>
  );
}

export default function TerminalScene({ active, scrollProgress: propScrollProgress, onContextLost }) {
  const containerRef = useRef(null);
  const [visible, setVisible] = useState(false);
  const [docHidden, setDocHidden] = useState(document.hidden);
  const [currentFixture, setCurrentFixture] = useState(() => getGlobalTuiFixture() || initialFixturesData.fixtures?.[0]);
  const internalProgress = useInternalScrollProgress(containerRef);
  const effectiveProgress = typeof propScrollProgress === 'number' ? propScrollProgress : internalProgress;

  useEffect(() => {
    const initial = getGlobalTuiFixture() || initialFixturesData.fixtures?.[0];
    if (initial) setCurrentFixture(initial);

    const unsub = subscribeTuiFixture((fix) => {
      if (fix) setCurrentFixture(fix);
    });
    return unsub;
  }, []);

  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new IntersectionObserver(([entry]) => {
      setVisible(entry.isIntersecting);
    });
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const handleVisibilityChange = () => setDocHidden(document.hidden);
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, []);

  const isActive = active && visible && !docHidden;

  return (
    <div ref={containerRef} style={{ width: '100%', height: '100%', position: 'absolute', inset: 0, zIndex: 10 }}>
      <Canvas
        gl={{
          alpha: true,
          antialias: true,
          powerPreference: 'default',
          failIfMajorPerformanceCaveat: false,
          preserveDrawingBuffer: false,
        }}
        dpr={window.devicePixelRatio > 1.5 ? 1.5 : window.devicePixelRatio}
        frameloop={isActive ? 'always' : 'demand'}
        camera={{ position: [0, 0, 9.2], fov: 36 }}
        onCreated={({ gl }) => {
          gl.setClearColor(0x000000, 0);
          const canvas = gl.domElement;
          if (canvas) {
            const handleContextLost = (e) => {
              e.preventDefault();
              if (onContextLost) {
                onContextLost();
              }
              try {
                window.dispatchEvent(new CustomEvent('agy-swap-3d-error'));
              } catch (err) {}
            };
            canvas.addEventListener('webglcontextlost', handleContextLost, false);
          }
        }}
      >
        <Scene fixture={currentFixture} scrollProgress={effectiveProgress} />
      </Canvas>
    </div>
  );
}
