import { useState } from 'react';
import { AlertTriangle, Info, Sliders } from 'lucide-react';

export const QuotaCalculator = () => {
  const [tier, setTier] = useState<'paid' | 'free' | 'unavailable'>('paid');
  const [weekly, setWeekly] = useState(66);
  const [fiveHour, setFiveHour] = useState(99);

  const meter = (label: string, value: number) => (
    <div className="space-y-1" key={label}>
      <div className="flex items-center justify-between"><span>{label}</span><span>{value}% remaining</span></div>
      <div role="progressbar" aria-label={`${label} remaining`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={value} className="h-2 overflow-hidden rounded-sm bg-zinc-800">
        <div className={`h-full ${value <= 10 ? 'bg-red-400' : value <= 30 ? 'bg-amber-400' : 'bg-emerald-400'}`} style={{ width: `${value}%` }} />
      </div>
    </div>
  );

  return (
    <section id="calculator" className="scroll-mt-24 py-20 relative z-10">
      <div className="max-w-5xl mx-auto px-6">
        <div className="relative overflow-hidden rounded-lg border border-zinc-800 bg-zinc-900/60 p-6 sm:p-10">
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-center">
            <div className="lg:col-span-6 space-y-6">
              <div>
                <div className="inline-flex items-center gap-2 text-xs font-mono text-orange-400 mb-2"><Sliders aria-hidden="true" className="w-3.5 h-3.5" /><span>GOOGLE QUOTA PREVIEW</span></div>
                <h2 className="text-2xl font-bold text-white tracking-tight mb-2">Render only the buckets Google returns</h2>
                <p className="text-sm text-zinc-400 leading-relaxed">Paid accounts currently include weekly and 5-hour limits. Free accounts include weekly limits only.</p>
              </div>

              <div className="space-y-2">
                <label htmlFor="quota-tier" className="block text-xs font-medium text-zinc-300">Quota response</label>
                <select id="quota-tier" value={tier} onChange={(event) => setTier(event.target.value as typeof tier)} className="min-h-11 w-full rounded border border-zinc-700 bg-zinc-950 px-3 text-sm text-zinc-200 focus:border-orange-400 focus:outline-none">
                  <option value="paid">Google AI paid tier</option><option value="free">Free tier</option><option value="unavailable">Refresh unavailable</option>
                </select>
              </div>

              {tier !== 'unavailable' && <div className="space-y-4">
                <div className="space-y-2">
                  <div className="flex justify-between text-xs"><label htmlFor="weekly-remaining" className="font-medium text-zinc-300">Weekly remaining</label><output htmlFor="weekly-remaining" className="font-mono font-bold text-cyan-300">{weekly}%</output></div>
                  <input id="weekly-remaining" type="range" min="0" max="100" value={weekly} onChange={(event) => setWeekly(Number(event.target.value))} className="h-11 w-full cursor-pointer accent-cyan-500" />
                </div>
                {tier === 'paid' && <div className="space-y-2">
                  <div className="flex justify-between text-xs"><label htmlFor="five-hour-remaining" className="font-medium text-zinc-300">5-hour remaining</label><output htmlFor="five-hour-remaining" className="font-mono font-bold text-cyan-300">{fiveHour}%</output></div>
                  <input id="five-hour-remaining" type="range" min="0" max="100" value={fiveHour} onChange={(event) => setFiveHour(Number(event.target.value))} className="h-11 w-full cursor-pointer accent-cyan-500" />
                </div>}
              </div>}
            </div>

            <div className="lg:col-span-6">
              <div className="rounded-lg border border-zinc-700 bg-zinc-950 p-5 font-mono text-xs leading-relaxed text-zinc-300 shadow-lg">
                <div className="mb-3 text-zinc-400 select-none">// TUI rendering preview</div>
                {tier === 'unavailable' ? <div className="flex items-start gap-2 text-amber-400"><AlertTriangle aria-hidden="true" className="mt-0.5 h-3.5 w-3.5 shrink-0" /><span>Usage unavailable · cached data kept when available</span></div> : <div className="space-y-4">
                  <div className="flex items-start gap-2 text-emerald-400"><Info aria-hidden="true" className="mt-0.5 h-3.5 w-3.5 shrink-0" /><span>Tier synced from Google: {tier === 'paid' ? 'Google AI Pro' : 'Free'}</span></div>
                  <div className="space-y-3"><div className="font-bold text-zinc-200">Gemini Models</div>{meter('Weekly Limit', weekly)}{tier === 'paid' && meter('Five Hour Limit', fiveHour)}</div>
                </div>}
                <div className="mt-4 border-t border-zinc-800 pt-3 text-zinc-400">Free responses never receive a fabricated 5-hour bucket.</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
