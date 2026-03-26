import { Routes, Route } from 'react-router'

function Placeholder({ name }: { name: string }) {
  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="text-center">
        <h1 className="text-3xl font-bold text-white mb-2">ODA Dashboard</h1>
        <p className="text-gray-400">{name} — coming soon</p>
        <a href="/" className="text-blue-400 hover:text-blue-300 mt-4 inline-block">
          ← Back to classic dashboard
        </a>
      </div>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Placeholder name="Board" />} />
      <Route path="/task/:id" element={<Placeholder name="Task Detail" />} />
      <Route path="/wizard" element={<Placeholder name="Wizard" />} />
      <Route path="/settings" element={<Placeholder name="Settings" />} />
      <Route path="/sprint/close" element={<Placeholder name="Sprint Close" />} />
    </Routes>
  )
}
