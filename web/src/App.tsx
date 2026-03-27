import { Routes, Route } from 'react-router'
import { Navbar } from './components/layout/Navbar'
import { Footer } from './components/layout/Footer'
import { useWebSocketUpdates } from './hooks/useWebSocket'
import { AppContext } from './AppContext'
import BoardPage from './pages/BoardPage'
import TaskPage from './pages/TaskPage'
import SettingsPage from './pages/SettingsPage'
import WizardPage from './pages/WizardPage'
import SprintClosePage from './pages/SprintClosePage'

export default function App() {
  const { wsConnected, onLogStream } = useWebSocketUpdates()

  return (
    <AppContext.Provider value={{ wsConnected, onLogStream }}>
      <div className="min-h-screen bg-gray-950 text-gray-200 flex flex-col">
        <Navbar />
        <main className="flex-1 p-4 overflow-auto">
          <Routes>
            <Route path="/" element={<BoardPage />} />
            <Route path="/task/:id" element={<TaskPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/wizard" element={<WizardPage />} />
            <Route path="/sprint/close" element={<SprintClosePage />} />
          </Routes>
        </main>
        <Footer />
      </div>
    </AppContext.Provider>
  )
}
