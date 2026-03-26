import { Routes, Route } from 'react-router'
import { Navbar } from './components/layout/Navbar'
import { Footer } from './components/layout/Footer'
import { useWebSocketUpdates } from './hooks/useWebSocket'
import BoardPage from './pages/BoardPage'
import TaskPage from './pages/TaskPage'
import SettingsPage from './pages/SettingsPage'
import WizardPage from './pages/WizardPage'
import SprintClosePage from './pages/SprintClosePage'

export default function App() {
  useWebSocketUpdates()

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1">
        <Routes>
          <Route path="/" element={<BoardPage />} />
          <Route path="/task/:id" element={<TaskPage />} />
          <Route path="/wizard" element={<WizardPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/sprint/close" element={<SprintClosePage />} />
        </Routes>
      </main>
      <Footer />
    </div>
  )
}
