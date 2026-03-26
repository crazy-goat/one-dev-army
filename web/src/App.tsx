import { createContext, useContext } from 'react'
import { Routes, Route } from 'react-router'
import { Navbar } from './components/layout/Navbar'
import { Footer } from './components/layout/Footer'
import { useWebSocketUpdates } from './hooks/useWebSocket'
import type { LogStreamPayload } from './api/types'
import BoardPage from './pages/BoardPage'
import TaskPage from './pages/TaskPage'
import SettingsPage from './pages/SettingsPage'
import WizardPage from './pages/WizardPage'
import SprintClosePage from './pages/SprintClosePage'

interface AppContextValue {
  wsConnected: boolean
  onLogStream: (handler: (payload: LogStreamPayload) => void) => () => void
}

const AppContext = createContext<AppContextValue>({
  wsConnected: false,
  onLogStream: () => () => {},
})

export function useAppContext() {
  return useContext(AppContext)
}

export default function App() {
  const { wsConnected, onLogStream } = useWebSocketUpdates()

  return (
    <AppContext.Provider value={{ wsConnected, onLogStream }}>
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
    </AppContext.Provider>
  )
}
