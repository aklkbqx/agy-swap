import { useState } from 'react';
import { Check, Copy, Terminal, X } from 'lucide-react';
import { APP_VERSION } from '../version';

type CopyState = 'idle' | 'copied' | 'error';

export const Hero = () => {
  const [copyState, setCopyState] = useState<CopyState>('idle');
  const command = "curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash";

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopyState('copied');
    } catch {
      setCopyState('error');
    }
    window.setTimeout(() => setCopyState('idle'), 2000);
  };

  return (
    <section className="relative flex min-h-[78vh] flex-col justify-center overflow-hidden border-b border-zinc-800 pt-32 pb-20">
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(to_right,#27272a33_1px,transparent_1px),linear-gradient(to_bottom,#27272a33_1px,transparent_1px)] bg-[size:4rem_4rem] [mask-image:linear-gradient(to_bottom,#000,transparent_80%)]" />

      <div className="relative z-10 mx-auto w-full max-w-5xl px-6 text-center">
        <div className="mb-7 inline-flex items-center gap-2 rounded border border-zinc-700 bg-zinc-900 px-3 py-1.5 font-mono text-xs text-zinc-300">
          <Terminal aria-hidden="true" className="h-3.5 w-3.5 text-orange-400" />
          <span>agy-swap v{APP_VERSION}</span>
        </div>

        <h1 className="mx-auto mb-6 max-w-4xl text-4xl leading-tight font-extrabold tracking-tight text-white sm:text-6xl">
          Switch Google Antigravity accounts without repeating browser login
        </h1>

        <p className="mx-auto mb-10 max-w-2xl text-base leading-relaxed text-zinc-300 sm:text-lg">
          A dependency-free Python TUI for saved sessions, real Google quota usage and model-aware account rotation.
        </p>

        <div className="mx-auto max-w-3xl rounded-lg border border-zinc-700 bg-zinc-900 text-left shadow-xl shadow-black/30">
          <div className="flex items-center justify-between border-b border-zinc-700 px-4 py-2 font-mono text-xs text-zinc-400">
            <span>Verified installer</span>
            <span>SHA-256 checked</span>
          </div>
          <div className="flex items-center gap-3 p-3 sm:p-4">
            <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs text-zinc-200 sm:text-sm">
              <span aria-hidden="true" className="mr-2 text-orange-400">$</span>{command}
            </code>
            <button
              type="button"
              onClick={handleCopy}
              className="flex min-h-11 shrink-0 items-center gap-1.5 rounded-md border border-zinc-600 bg-zinc-800 px-3 py-2 text-xs font-semibold text-zinc-100 hover:border-orange-400"
              aria-live="polite"
            >
              {copyState === 'copied' ? <Check aria-hidden="true" className="h-4 w-4 text-emerald-400" /> : copyState === 'error' ? <X aria-hidden="true" className="h-4 w-4 text-red-400" /> : <Copy aria-hidden="true" className="h-4 w-4" />}
              {copyState === 'copied' ? 'Copied' : copyState === 'error' ? 'Copy failed' : 'Copy'}
            </button>
          </div>
        </div>
      </div>
    </section>
  );
};
