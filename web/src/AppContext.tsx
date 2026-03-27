import { createContext } from 'react'

import type { LogStreamPayload } from './api/types'

interface AppContextValue {
  wsConnected: boolean
  onLogStream: (handler: (payload: LogStreamPayload) => void) => () => void
}

const AppContext = createContext<AppContextValue>({
  wsConnected: false,
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  onLogStream: () => () => {},
})

export { AppContext }
export type { AppContextValue }
