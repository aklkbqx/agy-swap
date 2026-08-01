import { useState } from 'react';
import { Terminal, ChevronUp, ChevronDown, CheckCircle2, RotateCw } from 'lucide-react';
import { APP_VERSION } from '../version';

interface QuotaBucket {
  name: string;
  remaining: number;
  reset: string;
}

interface QuotaGroup {
  name: string;
  buckets: QuotaBucket[];
}

interface Account {
  name: string;
  email: string;
  avatar: string;
  color: string;
  active: boolean;
  tier: string;
  quota?: QuotaGroup[];
}

const lowestRemaining = (account: Account) => {
  const buckets = account.quota?.flatMap((group) => group.buckets) ?? [];
  return buckets.length ? Math.min(...buckets.map((bucket) => bucket.remaining)) : undefined;
};

const quotaBar = (remaining: number) => {
  const filled = Math.round(remaining / 100 * 12);
  return `${'█'.repeat(filled)}${'░'.repeat(12 - filled)}`;
};

export const TuiSimulator = () => {
  const [accounts, setAccounts] = useState<Account[]>([
    {
      name: 'Dev Alpha', email: 'alpha@example.com', avatar: 'GM', color: '#A65B00', active: false, tier: 'Google AI Pro',
      quota: [
        { name: 'Gemini Models', buckets: [{ name: 'Weekly Limit', remaining: 66.38, reset: '5d 10h' }, { name: 'Five Hour Limit', remaining: 99.86, reset: '3h 42m' }] },
        { name: 'Claude and GPT models', buckets: [{ name: 'Weekly Limit', remaining: 99.19, reset: '6d 22h' }, { name: 'Five Hour Limit', remaining: 97.57, reset: '3h 41m' }] },
      ],
    },
    {
      name: 'Dev Beta', email: 'beta@example.com', avatar: 'KT', color: '#00A676', active: false, tier: 'Google AI Pro',
      quota: [
        { name: 'Gemini Models', buckets: [{ name: 'Weekly Limit', remaining: 82.01, reset: '5d 10h' }, { name: 'Five Hour Limit', remaining: 99.86, reset: '3h 42m' }] },
        { name: 'Claude and GPT models', buckets: [{ name: 'Weekly Limit', remaining: 99.19, reset: '6d 22h' }, { name: 'Five Hour Limit', remaining: 97.57, reset: '3h 42m' }] },
      ],
    },
    {
      name: 'Dev Gamma', email: 'gamma@example.com', avatar: 'IF', color: '#0070A6', active: false, tier: 'Google AI Pro',
      quota: [
        { name: 'Gemini Models', buckets: [{ name: 'Weekly Limit', remaining: 82.1, reset: '5d 10h' }, { name: 'Five Hour Limit', remaining: 93.38, reset: '12m' }] },
        { name: 'Claude and GPT models', buckets: [{ name: 'Weekly Limit', remaining: 30.72, reset: '5d 8h' }, { name: 'Five Hour Limit', remaining: 97.57, reset: '3h 46m' }] },
      ],
    },
    {
      name: 'Dev Free', email: 'free@example.com', avatar: 'AK', color: '#A6005B', active: true, tier: 'Free',
      quota: [
        { name: 'Gemini Models', buckets: [{ name: 'Weekly Limit', remaining: 0, reset: '3d 9h' }] },
        { name: 'Claude and GPT models', buckets: [{ name: 'Weekly Limit', remaining: 0, reset: '5d 9h' }] },
      ],
    },
  ]);
  const [selectedIndex, setSelectedIndex] = useState(3);
  const [lastActionMessage, setLastActionMessage] = useState('');

  const handleNavigate = (direction: number) => {
    setSelectedIndex((current) => (current + direction + accounts.length) % accounts.length);
    setLastActionMessage('');
  };

  const handleSelectActive = () => {
    setAccounts((current) => current.map((account, index) => ({ ...account, active: index === selectedIndex })));
    setLastActionMessage(`Switched active session to ${accounts[selectedIndex].email}`);
  };

  const handleAutoRotate = () => {
    let nextIndex = selectedIndex;
    for (let offset = 1; offset <= accounts.length; offset += 1) {
      const candidate = (selectedIndex + offset) % accounts.length;
      const remaining = lowestRemaining(accounts[candidate]);
      if (remaining !== undefined && remaining > 0) {
        nextIndex = candidate;
        break;
      }
    }
    setSelectedIndex(nextIndex);
    setAccounts((current) => current.map((account, index) => ({ ...account, active: index === nextIndex })));
    setLastActionMessage(`Auto-rotated to account with available quota: ${accounts[nextIndex].email}`);
  };

  const highlightedAccount = accounts[selectedIndex] ?? accounts[0];
  const activeAccount = accounts.find((account) => account.active);

  return (
    <section id="simulator" className="scroll-mt-24 py-24 relative z-10">
      <div className="max-w-5xl mx-auto px-6">
        <div className="text-center mb-12">
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight mb-3">Interactive TUI Simulator</h2>
          <p className="text-zinc-400 max-w-xl mx-auto text-base">Explore the account list and the weekly and 5-hour quota groups returned by Google.</p>
        </div>

        <div className="mx-auto max-w-3xl overflow-x-auto rounded-lg border border-zinc-700 bg-zinc-950 font-mono text-xs shadow-xl shadow-black/50 sm:text-sm" aria-label="Interactive terminal account simulator">
          <div className="flex items-center justify-between border-b border-zinc-800 bg-zinc-900 px-4 py-3 select-none">
            <div className="flex items-center gap-2"><div className="w-3 h-3 rounded-full bg-red-500/80" /><div className="w-3 h-3 rounded-full bg-amber-500/80" /><div className="w-3 h-3 rounded-full bg-emerald-500/80" /></div>
            <div className="flex items-center gap-2 text-zinc-400 text-xs"><Terminal aria-hidden="true" className="w-3.5 h-3.5" /><span>agy-swap — zsh — 82x24</span></div>
            <div className="w-12" />
          </div>

          <div className="min-h-[460px] min-w-[700px] bg-zinc-950 p-6 leading-relaxed text-zinc-300">
            <div className="flex items-center gap-2 mb-2"><span className="font-bold text-orange-400">AGY SWAP</span><span className="text-zinc-400">v{APP_VERSION} · Google Antigravity Session Manager</span></div>
            <div className="text-zinc-800 select-none mb-3">─────────────────────────────────────────────────────────────────────────────</div>
            <div className="mb-4">
              <span className="font-bold text-white">Active:</span>{' '}
              {activeAccount ? <span><span className="text-emerald-400">●</span>{' '}<span className="font-bold px-1 rounded text-black text-[11px]" style={{ backgroundColor: activeAccount.color }}>{activeAccount.avatar}</span>{' '}<span className="font-bold text-white">{activeAccount.name}</span>{' '}<span className="text-zinc-400">&lt;{activeAccount.email}&gt;</span>{' '}<span className="text-emerald-400">(Saved)</span></span> : <span className="text-zinc-400">○ Not logged in</span>}
            </div>

            <div className="mb-4">
              <div className="font-bold text-white mb-1">ACCOUNTS <span className="text-zinc-400 font-normal">({accounts.length} total)</span></div>
              <div className="space-y-1">
                {accounts.map((account, index) => {
                  const selected = index === selectedIndex;
                  const remaining = lowestRemaining(account);
                  const status = remaining === undefined ? 'Usage unavailable' : remaining <= 0 ? 'Limited' : `${remaining.toFixed(0)}% lowest remaining`;
                  return (
                    <button type="button" key={account.email} onClick={() => { setSelectedIndex(index); setLastActionMessage(''); }} aria-pressed={selected} aria-label={`Select ${account.name}, ${status}`} className={`flex w-full items-center gap-2 rounded px-1 py-1 text-left transition-colors ${selected ? 'bg-zinc-800/60' : 'hover:bg-zinc-900/40'}`}>
                      <span className={selected ? 'text-orange-400 font-bold' : 'text-transparent'}>❯</span><span className="text-zinc-400">[{index + 1}]</span>
                      <span className="font-bold px-1 rounded text-black text-[10px]" style={{ backgroundColor: account.color }}>{account.avatar}</span>
                      <span className={account.active ? 'text-emerald-400' : 'text-zinc-400'}>{account.active ? '●' : '○'}</span>
                      <span className={selected ? 'font-bold text-white' : 'text-zinc-300'}>{account.name}</span><span className="text-zinc-400">&lt;{account.email}&gt;</span><span className="text-zinc-500">[{account.tier}]</span>
                      <span className={`ml-auto ${remaining !== undefined && remaining <= 0 ? 'text-red-400' : 'text-zinc-300'}`}>{status}</span>
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="mb-4 rounded-lg border border-zinc-800/80 bg-zinc-900/40 p-3">
              <div className="font-bold text-white mb-2">QUOTA TRACKER <span className="text-zinc-400 font-normal">(Highlighted: {highlightedAccount.name})</span></div>
              {highlightedAccount.quota ? (
                <div className="space-y-2 text-xs">
                  {highlightedAccount.quota.map((group) => (
                    <div key={group.name}>
                      <div className="font-bold text-zinc-200">{group.name}</div>
                      {group.buckets.map((bucket) => (
                        <div key={bucket.name} className="flex items-center gap-2 pl-3">
                          <span className="w-28 text-zinc-400">{bucket.name}</span>
                          <span role="progressbar" aria-label={`${group.name} ${bucket.name} remaining`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={bucket.remaining} className={bucket.remaining <= 10 ? 'text-red-400' : bucket.remaining <= 30 ? 'text-amber-400' : 'text-emerald-400'}>[{quotaBar(bucket.remaining)}]</span>
                          <span>{bucket.remaining.toFixed(2)}% remaining</span><span className="text-zinc-500">· resets in {bucket.reset}</span>
                        </div>
                      ))}
                    </div>
                  ))}
                  <div className="text-zinc-500">Google quota synced 0m ago · [r] refresh</div>
                </div>
              ) : <div className="text-zinc-400">Usage unavailable · no recent cooldown error</div>}
            </div>

            <div className="text-zinc-800 select-none mb-1">─────────────────────────────────────────────────────────────────────────────</div>
            {lastActionMessage ? <div className="text-emerald-400 flex items-center gap-1.5"><CheckCircle2 aria-hidden="true" className="w-3.5 h-3.5" /><span>{lastActionMessage}</span></div> : <div className="text-zinc-400 text-xs">Navigate: [↑/↓] │ Select: [Enter] │ Shortcuts: [a] Add [r] Refresh [t] Tier [d] Delete [q] Quit</div>}
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-center gap-3 mt-6">
          <button type="button" onClick={() => handleNavigate(-1)} className="flex min-h-11 items-center gap-1.5 rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-2 font-mono text-xs font-medium text-zinc-200 transition-colors hover:border-orange-400 hover:text-white"><ChevronUp aria-hidden="true" className="w-4 h-4" /><span>[↑] Up</span></button>
          <button type="button" onClick={() => handleNavigate(1)} className="flex min-h-11 items-center gap-1.5 rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-2 font-mono text-xs font-medium text-zinc-200 transition-colors hover:border-orange-400 hover:text-white"><ChevronDown aria-hidden="true" className="w-4 h-4" /><span>[↓] Down</span></button>
          <button type="button" onClick={handleSelectActive} className="flex min-h-11 items-center gap-1.5 rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-2 font-mono text-xs font-medium text-zinc-200 transition-colors hover:border-orange-400 hover:text-white"><span>[Enter] Switch Selected</span></button>
          <button type="button" onClick={handleAutoRotate} className="flex min-h-11 items-center gap-2 rounded-lg border border-orange-400/60 bg-orange-500/10 px-4 py-2 font-mono text-xs font-semibold text-orange-300 transition-colors hover:bg-orange-500/20"><RotateCw aria-hidden="true" className="w-3.5 h-3.5" /><span>Auto-Rotate Next</span></button>
        </div>
      </div>
    </section>
  );
};
