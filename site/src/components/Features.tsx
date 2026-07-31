import { motion } from 'framer-motion';
import { Users, Activity, Zap, ShieldCheck } from 'lucide-react';

const features = [
  {
    icon: Users,
    title: "Google Profile Avatars",
    description: "Automatically fetches user profile photos & renders color-coded initial badges for instant account recognition.",
    accent: "from-orange-500/20 to-amber-500/20",
    iconColor: "text-orange-400"
  },
  {
    icon: Activity,
    title: "Dual Model Quota Intelligence",
    description: "Tracks Claude/GPT models (0% rate limits) and Gemini models (100% capacity) independently in real time.",
    accent: "from-amber-500/20 to-yellow-500/20",
    iconColor: "text-amber-400"
  },
  {
    icon: Zap,
    title: "Smart Auto-Rotation Engine",
    description: "Single command `agy-swap next` skips rate-limited accounts automatically and switches to the next ready session.",
    accent: "from-emerald-500/20 to-teal-500/20",
    iconColor: "text-emerald-400"
  },
  {
    icon: ShieldCheck,
    title: "Native OS Keyring Security",
    description: "Direct integration with macOS Keychain, Windows Credential Manager, and Linux Secret Service.",
    accent: "from-cyan-500/20 to-blue-500/20",
    iconColor: "text-cyan-400"
  }
];

export const Features = () => {
  return (
    <section id="features" className="py-24 relative z-10">
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
          {features.map((feat, index) => {
            const Icon = feat.icon;
            return (
              <motion.div
                key={feat.title}
                initial={{ opacity: 0, y: 20 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.5, delay: index * 0.1 }}
                className="glass-panel glass-panel-hover p-6 rounded-2xl flex flex-col justify-between"
              >
                <div>
                  <div className={`w-12 h-12 rounded-xl bg-gradient-to-br ${feat.accent} border border-white/10 flex items-center justify-center mb-6`}>
                    <Icon className={`w-6 h-6 ${feat.iconColor}`} />
                  </div>

                  <h3 className="text-lg font-bold text-white mb-2 font-sans tracking-tight">
                    {feat.title}
                  </h3>

                  <p className="text-sm text-zinc-400 leading-relaxed font-normal">
                    {feat.description}
                  </p>
                </div>
              </motion.div>
            );
          })}
        </div>
      </div>
    </section>
  );
};
