import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { TripPage } from "./pages/TripPage";
import { TripsPage } from "./pages/TripsPage";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<TripsPage />} />
        {/* A share link is the whole credential: the token in the path both
            addresses the trip and says whether you may edit it. */}
        <Route path="/t/:token" element={<TripPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
