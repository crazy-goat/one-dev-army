import { Routes, Route } from 'react-router'
import { Navbar } from './components/layout/Navbar'
import { Footer } from './components/layout/Footer'
import { useWebSocketUpdates } from './hooks/useWebSocket'

// Placeholder pages (will be replaced in Tasks 12-16)
function Placeholder({ name }: { name: string }) {
  return (
    <div className="flex items-center justify-center flex-1">
      <div className="text-center">
        <h1 className="text-2xl font-bold text-white mb-2">{name}</h1>
        <p className="text-gray-400">Coming soon</p>
      </div>
    </div>
  )
}

export default function App() {
  useWebSocketUpdates()

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1">
        <Routes>
          <Route path="/" element={<Placeholder name="Board" />} />
          <Route path="/task/:id" element={<Placeholder name="Task Detail" />} />
          <Route path="/wizard" element={<Placeholder name="Wizard" />} />
          <Route path="/settings" element={<Placeholder name="Settings" />} />
          <Route path="/sprint/close" element={<Placeholder name="Sprint Close" />} />
        </Routes>
      </main>
      <Footer />
    </div>
  )
}
