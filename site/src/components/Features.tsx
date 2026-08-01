import { Users, Activity, Zap, ShieldCheck } from 'lucide-react';

const features = [
  {
    icon: Users,
    title: "Recognizable account labels",
    description: "Fetches the account name when available and renders deterministic initial badges for quick recognition.",
    iconColor: "text-orange-400"
  },
  {
    icon: Activity,
    title: "Real Google quota",
    description: "Shows the weekly and optional 5-hour buckets returned by Google, including remaining percentage and reset time.",
    iconColor: "text-amber-400"
  },
  {
    icon: Zap,
    title: "Deterministic rotation",
    description: "`agy-swap next` selects by the matching Gemini or Claude/GPT quota group, with local cooldowns as a fallback.",
    iconColor: "text-emerald-400"
  },
  {
    icon: ShieldCheck,
    title: "Protected credential storage",
    description: "Uses owner-only local profile files and mirrors the active session to macOS Keychain, Windows Credential Manager or Linux Secret Service.",
    iconColor: "text-cyan-400"
  }
];

export const Features = () => {
  return (
    <section id="features" className="scroll-mt-24 py-24 relative z-10">
      <div className="max-w-6xl mx-auto px-6">
        <div className="text-center mb-16">
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight mb-3">
            Engineered for Developer Productivity
          </h2>
          <p className="text-zinc-400 max-w-xl mx-auto text-base">
            High performance, zero dependencies, and complete control over your Google Antigravity CLI sessions.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {features.map((feat) => {
            const Icon = feat.icon;
            return (
              <div
                key={feat.title}
                className="flex flex-col justify-between rounded-lg border border-zinc-800 bg-zinc-900/70 p-6 transition-colors hover:border-zinc-700"
              >
                <div>
                  <div className="mb-6 flex h-12 w-12 items-center justify-center rounded border border-zinc-700 bg-zinc-950">
                    <Icon aria-hidden="true" className={`w-6 h-6 ${feat.iconColor}`} />
                  </div>

                  <h3 className="text-lg font-bold text-white mb-2 font-sans tracking-tight">
                    {feat.title}
                  </h3>

                  <p className="text-sm text-zinc-400 leading-relaxed font-normal">
                    {feat.description}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
};
