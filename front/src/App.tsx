import {
  BrowserRouter as Router,
  Routes,
  Route,
} from "react-router-dom";
import StreamList from "./pages/StreamList.tsx";
import Player from "./pages/Player.tsx";
import './App.css'
import './styles/common.css'

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/" element={<StreamList />} />
        <Route path="/player/:streamId" element={<Player />} />
      </Routes>
    </Router>
  )
}

export default App
