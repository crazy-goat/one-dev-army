import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { BoardPage } from './pages/BoardPage'
import { TaskPage } from './pages/TaskPage'
import { useWebSocket } from './hooks/useWebSocket'
import { ErrorBoundary } from './components/common/ErrorBoundary'

function AppContent() {
  useWebSocket()

  return (
    <Routes>
      <Route path="/" element={<BoardPage />} />
      <Route path="/task/:id" element={<TaskPage />} />
      <Route path="/settings" element={<div>Settings - Coming Soon</div>} />
      <Route path="*" element={<div style={{ padding: '2rem', color: 'var(--muted)' }}>Page not found</div>} />
    </Routes>
  )
}

function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter basename="/new">
        <AppContent />
      </BrowserRouter>
    </ErrorBoundary>
  )
}

export default App
