import { Navbar } from './components/Navbar';
import { Hero } from './components/Hero';
import { TuiSimulator } from './components/TuiSimulator';
import { Features } from './components/Features';
import { QuotaCalculator } from './components/QuotaCalculator';
import { CommandsTable } from './components/CommandsTable';
import { Footer } from './components/Footer';

export function App() {
  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 selection:bg-orange-500 selection:text-black font-sans">
        <a href="#main-content" className="sr-only focus:not-sr-only fixed left-4 top-4 z-[60] rounded bg-orange-400 px-4 py-2 font-semibold text-zinc-950">
          Skip to content
        </a>
        <Navbar />
        <main id="main-content">
          <Hero />
          <TuiSimulator />
          <Features />
          <QuotaCalculator />
          <CommandsTable />
        </main>
        <Footer />
    </div>
  );
}

export default App;
