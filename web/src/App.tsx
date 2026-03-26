import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { BoardPage } from './pages/BoardPage'
import { useWebSocket } from './hooks/useWebSocket'

function AppContent() {
  useWebSocket()

  return (
    <Routes>
      <Route path="/" element={<BoardPage />} />
      <Route path="/task/:id" element={<div>Task Detail - Coming Soon</div>} />
      <Route path="/settings" element={<div>Settings - Coming Soon</div>} />
      <Route path="*" element={<div style={{ padding: '2rem', color: 'var(--muted)' }}>Page not found</div>} />
    </Routes>
  )
}

function App() {
  return (
    <BrowserRouter basename="/new">
      <AppContent />
    </BrowserRouter>
  )
}

export default App
