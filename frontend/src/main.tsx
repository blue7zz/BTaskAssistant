import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";

if (/Mac/i.test(`${navigator.platform} ${navigator.userAgent}`)) {
  document.documentElement.dataset.platform = "darwin";
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
