import { Navbar } from './components/Navbar';
import { Hero } from './components/Hero';
import { TuiSimulator } from './components/TuiSimulator';
import { Features } from './components/Features';
import { QuotaCalculator } from './components/QuotaCalculator';
import { CommandsTable } from './components/CommandsTable';
import { Footer } from './components/Footer';

export function App() {
  return (
    <div className="min-h-screen bg-[#09090B] text-zinc-100 selection:bg-orange-500 selection:text-white font-sans">
      <Navbar />
      <main>
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
