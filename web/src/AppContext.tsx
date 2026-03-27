import { createContext } from 'react'

import type { LogStreamPayload } from './api/types'

interface AppContextValue {
  wsConnected: boolean
  onLogStream: (handler: (payload: LogStreamPayload) => void) => () => void
}

const AppContext = createContext<AppContextValue>({
  wsConnected: false,
  onLogStream: () => () => {},
})

export { AppContext }
export type { AppContextValue }
