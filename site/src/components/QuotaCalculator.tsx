import { useState } from 'react';
import { Sliders, AlertTriangle, CheckCircle2 } from 'lucide-react';

export const QuotaCalculator = () => {
  const [claudePct, setClaudePct] = useState<number>(0);
  const [geminiPct, setGeminiPct] = useState<number>(46);
  const [cooldownMinutes, setCooldownMinutes] = useState<number>(248);

  const renderBar = (pct: number) => {
    const total = 12;
    const filled = Math.round((pct / 100) * total);
    const empty = total - filled;
    return `[${'█'.repeat(filled)}${'░'.repeat(empty)}] ${pct.toString().padStart(3, ' ')}%`;
  };

  const formatTime = (mins: number) => {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  };

  return (
    <section id="calculator" className="py-20 relative z-10">
      <div className="max-w-5xl mx-auto px-6">
        <div className="glass-panel p-8 sm:p-10 rounded-2xl border border-white/10 relative overflow-hidden">
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-center">
            <div className="lg:col-span-6 space-y-6">
              <div>
                <div className="inline-flex items-center gap-2 text-xs font-mono text-orange-400 mb-2">
                  <Sliders className="w-3.5 h-3.5" />
                  <span>INTERACTIVE QUOTA LOGIC</span>
                </div>
                <h3 className="text-2xl font-bold text-white tracking-tight mb-2">
                  Quota & Cooldown Calculator
                </h3>
                <p className="text-sm text-zinc-400 leading-relaxed">
                  Drag the sliders below to simulate how agy-swap parses local Glog files and renders live model capacity.
                </p>
              </div>

              <div className="space-y-2">
                <div className="flex justify-between text-xs font-mono">
                  <span className="text-zinc-300 font-medium">Claude / GPT Quota Capacity</span>
                  <span className={claudePct === 0 ? 'text-red-400 font-bold' : 'text-emerald-400 font-bold'}>
                    {claudePct}%
                  </span>
                </div>
                <input 
                  type="range" 
                  min="0" 
                  max="100" 
                  value={claudePct} 
                  onChange={(e) => setClaudePct(Number(e.target.value))}
                  className="w-full h-2 bg-zinc-800 rounded-lg appearance-none cursor-pointer accent-orange-500"
                />
              </div>

              <div className="space-y-2">
                <div className="flex justify-between text-xs font-mono">
                  <span className="text-zinc-300 font-medium">Gemini Models Capacity</span>
                  <span className={geminiPct === 0 ? 'text-red-400 font-bold' : 'text-emerald-400 font-bold'}>
                    {geminiPct}%
                  </span>
                </div>
                <input 
                  type="range" 
                  min="0" 
                  max="100" 
                  value={geminiPct} 
                  onChange={(e) => setGeminiPct(Number(e.target.value))}
                  className="w-full h-2 bg-zinc-800 rounded-lg appearance-none cursor-pointer accent-amber-500"
                />
              </div>

              <div className="space-y-2">
                <div className="flex justify-between text-xs font-mono">
                  <span className="text-zinc-300 font-medium">5-Hour Cooldown Timer</span>
                  <span className="text-cyan-400 font-bold">{formatTime(cooldownMinutes)}</span>
                </div>
                <input 
                  type="range" 
                  min="1" 
                  max="300" 
                  value={cooldownMinutes} 
                  onChange={(e) => setCooldownMinutes(Number(e.target.value))}
                  className="w-full h-2 bg-zinc-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
                />
              </div>
            </div>

            <div className="lg:col-span-6">
              <div className="rounded-xl p-5 bg-[#0A0A0C] border border-zinc-800 font-mono text-xs text-zinc-300 leading-relaxed shadow-xl">
                <div className="text-zinc-500 mb-2 select-none">// Live TUI Rendering Preview</div>
                
                <div className="space-y-2 py-2">
                  <div className="flex items-center gap-3">
                    <span className="w-28 text-zinc-400">Claude/GPT 5h</span>
                    {claudePct === 0 ? (
                      <>
                        <span className="text-red-400 font-bold">{renderBar(0)}</span>
                        <span className="text-red-400 flex items-center gap-1">
                          <AlertTriangle className="w-3 h-3 shrink-0" />
                          Limited ({formatTime(cooldownMinutes)})
                        </span>
                      </>
                    ) : (
                      <>
                        <span className="text-emerald-400 font-bold">{renderBar(claudePct)}</span>
                        <span className="text-emerald-400 flex items-center gap-1">
                          <CheckCircle2 className="w-3 h-3 shrink-0" />
                          Ready ({formatTime(cooldownMinutes)})
                        </span>
                      </>
                    )}
                  </div>

                  <div className="flex items-center gap-3">
                    <span className="w-28 text-zinc-400">Gemini 5h</span>
                    {geminiPct === 0 ? (
                      <>
                        <span className="text-red-400 font-bold">{renderBar(0)}</span>
                        <span className="text-red-400 flex items-center gap-1">
                          <AlertTriangle className="w-3 h-3 shrink-0" />
                          Limited ({formatTime(cooldownMinutes)})
                        </span>
                      </>
                    ) : (
                      <>
                        <span className="text-amber-400 font-bold">{renderBar(geminiPct)}</span>
                        <span className="text-amber-400 flex items-center gap-1">
                          <CheckCircle2 className="w-3 h-3 shrink-0" />
                          Ready ({formatTime(cooldownMinutes)})
                        </span>
                      </>
                    )}
                  </div>
                </div>

                <div className="mt-3 pt-3 border-t border-zinc-800/80 text-[11px] text-zinc-500">
                  Status Indicator: {claudePct === 0 ? (
                    <span className="text-red-400 font-semibold">🔴 Claude ({formatTime(cooldownMinutes)})</span>
                  ) : (
                    <span className="text-emerald-400 font-semibold">Ready</span>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
