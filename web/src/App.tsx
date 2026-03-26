import { BrowserRouter, Routes, Route } from 'react-router-dom'

function App() {
  return (
    <BrowserRouter basename="/new">
      <Routes>
        <Route path="/" element={<div>ODA Dashboard - Coming Soon</div>} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
