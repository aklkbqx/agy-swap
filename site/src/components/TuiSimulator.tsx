import { useState } from 'react';
import { motion } from 'framer-motion';
import { Terminal, ChevronUp, ChevronDown, CheckCircle2, RotateCw } from 'lucide-react';

interface Account {
  name: string;
  email: string;
  avatar: string;
  color: string;
  active: boolean;
  claudeStatus: string;
  geminiStatus: string;
}

export const TuiSimulator = () => {
  const [accounts, setAccounts] = useState<Account[]>([
    { name: "Dev Alpha", email: "alpha@example.com", avatar: "GM", color: "#A65B00", active: false, claudeStatus: "Ready", geminiStatus: "Ready" },
    { name: "Dev Beta", email: "beta@example.com", avatar: "KT", color: "#00A676", active: false, claudeStatus: "Ready", geminiStatus: "Ready" },
    { name: "Dev Gamma", email: "gamma@example.com", avatar: "IF", color: "#0070A6", active: false, claudeStatus: "Limited (4h4m)", geminiStatus: "Ready" },
    { name: "Dev Delta", email: "delta@example.com", avatar: "AK", color: "#A6005B", active: true, claudeStatus: "Ready", geminiStatus: "Ready" }
  ]);

  const [selectedIndex, setSelectedIndex] = useState(3);
  const [lastActionMessage, setLastActionMessage] = useState("");

  const handleNavigate = (dir: number) => {
    setSelectedIndex((prev) => (prev + dir + accounts.length) % accounts.length);
    setLastActionMessage("");
  };

  const handleSelectActive = () => {
    setAccounts((prev) =>
      prev.map((acc, i) => ({
        ...acc,
        active: i === selectedIndex,
      }))
    );
    setLastActionMessage(`Switched active session to ${accounts[selectedIndex].email}`);
  };

  const handleAutoRotate = () => {
    let nextIdx = (selectedIndex + 1) % accounts.length;
    if (accounts[nextIdx].claudeStatus.includes("Limited")) {
      nextIdx = (nextIdx + 1) % accounts.length;
    }

    setSelectedIndex(nextIdx);
    setAccounts((prev) =>
      prev.map((acc, i) => ({
        ...acc,
        active: i === nextIdx,
      }))
    );
    setLastActionMessage(`Auto-rotated to ready account: ${accounts[nextIdx].email}`);
  };

  const highlightedAcc = accounts[selectedIndex] || accounts[0];
  const activeAcc = accounts.find((a) => a.active);

  return (
    <section id="simulator" className="py-24 relative z-10">
      <div className="max-w-5xl mx-auto px-6">
        <div className="text-center mb-12">
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight mb-3">
            Interactive TUI Terminal Simulator
          </h2>
          <p className="text-zinc-400 max-w-xl mx-auto text-base">
            Test the exact terminal UI right in your browser. Use the controls below to navigate or simulate 1-click auto-rotation.
          </p>
        </div>

        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="max-w-3xl mx-auto rounded-xl overflow-hidden bg-[#0C0C0E] border border-zinc-800 shadow-2xl shadow-black/80 font-mono text-xs sm:text-sm"
        >
          <div className="flex items-center justify-between px-4 py-3 bg-[#141417] border-b border-zinc-800 select-none">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-red-500/80" />
              <div className="w-3 h-3 rounded-full bg-amber-500/80" />
              <div className="w-3 h-3 rounded-full bg-emerald-500/80" />
            </div>

            <div className="flex items-center gap-2 text-zinc-400 text-xs">
              <Terminal className="w-3.5 h-3.5 text-zinc-500" />
              <span>agy-swap — zsh — 82x24</span>
            </div>

            <div className="w-12" />
          </div>

          <div className="p-6 text-zinc-300 leading-relaxed min-h-[380px] bg-[#0A0A0C]">
            <div className="flex items-center gap-2 mb-2">
              <span className="font-bold text-orange-400">AGY SWAP</span>
              <span className="text-zinc-500">v1.5.0 · Google Antigravity Session Manager</span>
            </div>
            <div className="text-zinc-800 select-none mb-3">─────────────────────────────────────────────────────────────────────────────</div>

            <div className="mb-4">
              <span className="font-bold text-white">Active:</span>{' '}
              {activeAcc ? (
                <span>
                  <span className="text-emerald-400">●</span>{' '}
                  <span 
                    className="font-bold px-1 rounded text-black text-[11px]" 
                    style={{ backgroundColor: activeAcc.color }}
                  >
                    {activeAcc.avatar}
                  </span>{' '}
                  <span className="font-bold text-white">{activeAcc.name}</span>{' '}
                  <span className="text-zinc-500">&lt;{activeAcc.email}&gt;</span>{' '}
                  <span className="text-emerald-400">(Saved)</span>
                </span>
              ) : (
                <span className="text-zinc-500">○ Not logged in</span>
              )}
            </div>

            <div className="mb-4">
              <div className="font-bold text-white mb-1">
                ACCOUNTS <span className="text-zinc-500 font-normal">({accounts.length} total)</span>
              </div>

              <div className="space-y-1">
                {accounts.map((acc, i) => {
                  const isSelected = i === selectedIndex;
                  const isLimited = acc.claudeStatus.includes("Limited");

                  return (
                    <div 
                      key={acc.email} 
                      onClick={() => { setSelectedIndex(i); setLastActionMessage(""); }}
                      className={`flex items-center gap-2 py-0.5 px-1 rounded cursor-pointer transition-colors ${
                        isSelected ? 'bg-zinc-800/60' : 'hover:bg-zinc-900/40'
                      }`}
                    >
                      <span className={isSelected ? 'text-orange-400 font-bold' : 'text-transparent'}>❯</span>
                      <span className="text-zinc-500">[{i + 1}]</span>
                      <span 
                        className="font-bold px-1 rounded text-black text-[10px]" 
                        style={{ backgroundColor: acc.color }}
                      >
                        {acc.avatar}
                      </span>
                      <span className={acc.active ? 'text-emerald-400' : 'text-zinc-600'}>
                        {acc.active ? '●' : '○'}
                      </span>
                      <span className={isSelected ? 'font-bold text-white' : 'text-zinc-300'}>
                        {acc.name}
                      </span>
                      <span className="text-zinc-500">&lt;{acc.email}&gt;</span>
                      <span className="ml-auto">
                        {isLimited ? (
                          <span className="text-red-400 font-semibold">Limited ({acc.claudeStatus.split('(')[1]}</span>
                        ) : (
                          <span className="text-emerald-400">Ready</span>
                        )}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>

            <div className="mb-4 p-3 rounded-lg bg-zinc-900/40 border border-zinc-800/80">
              <div className="font-bold text-white mb-2">
                QUOTA TRACKER <span className="text-zinc-500 font-normal">(Highlighted: {highlightedAcc.name})</span>
              </div>

              <div className="space-y-1 text-xs">
                {highlightedAcc.claudeStatus.includes("Limited") ? (
                  <div className="flex items-center gap-3">
                    <span className="w-28 text-zinc-400">Claude/GPT 5h</span>
                    <span className="text-red-400 font-mono">[░░░░░░░░░░░░] 0%</span>
                    <span className="text-red-400">Limited (Resets in 4h 4m)</span>
                  </div>
                ) : (
                  <div className="flex items-center gap-3">
                    <span className="w-28 text-zinc-400">Claude/GPT 5h</span>
                    <span className="text-emerald-400 font-mono">[████████████] 100%</span>
                    <span className="text-emerald-400">Ready</span>
                  </div>
                )}

                <div className="flex items-center gap-3">
                  <span className="w-28 text-zinc-400">Gemini 5h</span>
                  <span className="text-emerald-400 font-mono">[████████████] 100%</span>
                  <span className="text-emerald-400">Ready</span>
                </div>

                <div className="flex items-center gap-3">
                  <span className="w-28 text-zinc-400">Session Token</span>
                  <span className="text-cyan-400">Auto-refreshes</span>
                </div>
              </div>
            </div>

            <div className="text-zinc-800 select-none mb-1">─────────────────────────────────────────────────────────────────────────────</div>
            {lastActionMessage ? (
              <div className="text-emerald-400 flex items-center gap-1.5">
                <CheckCircle2 className="w-3.5 h-3.5" />
                <span>{lastActionMessage}</span>
              </div>
            ) : (
              <div className="text-zinc-500 text-xs">
                Navigate: [↑/↓] │ Select: [Enter] │ Shortcuts: [a, d, q]
              </div>
            )}
          </div>
        </motion.div>

        <div className="flex flex-wrap items-center justify-center gap-3 mt-6">
          <button 
            onClick={() => handleNavigate(-1)}
            className="flex items-center gap-1.5 px-4 py-2 rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-zinc-700 text-xs font-mono font-medium transition-all"
          >
            <ChevronUp className="w-4 h-4" />
            <span>[↑] Up</span>
          </button>

          <button 
            onClick={() => handleNavigate(1)}
            className="flex items-center gap-1.5 px-4 py-2 rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-zinc-700 text-xs font-mono font-medium transition-all"
          >
            <ChevronDown className="w-4 h-4" />
            <span>[↓] Down</span>
          </button>

          <button 
            onClick={handleSelectActive}
            className="flex items-center gap-1.5 px-4 py-2 rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-zinc-700 text-xs font-mono font-medium transition-all"
          >
            <span>[Enter] Switch Selected</span>
          </button>

          <button 
            onClick={handleAutoRotate}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-orange-500/10 border border-orange-500/30 text-orange-400 hover:bg-orange-500/20 text-xs font-mono font-semibold transition-all shadow-sm"
          >
            <RotateCw className="w-3.5 h-3.5" />
            <span>Auto-Rotate Next (1-Click)</span>
          </button>
        </div>
      </div>
    </section>
  );
};
