import { Routes, Route } from 'react-router'

import { AppContext } from './AppContext'
import { Footer } from './components/layout/Footer'
import { Navbar } from './components/layout/Navbar'
import { useWebSocketUpdates } from './hooks/useWebSocket'
import BoardPage from './pages/BoardPage'
import PlanSprintPage from './pages/PlanSprintPage'
import SettingsPage from './pages/SettingsPage'
import SprintClosePage from './pages/SprintClosePage'
import TaskPage from './pages/TaskPage'
import WizardPage from './pages/WizardPage'

export default function App() {
  const { wsConnected, onLogStream } = useWebSocketUpdates()

  return (
    <AppContext.Provider value={{ wsConnected, onLogStream }}>
      <div className="min-h-screen bg-gray-950 text-gray-200 flex flex-col">
        <Navbar />
        <main className="flex-1 p-4 overflow-auto">
          <Routes>
            <Route path="/" element={<BoardPage />} />
            <Route path="/sprint/plan" element={<PlanSprintPage />} />
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
