import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "@/shared/ui/ThemeToggle";
import { Root } from "./Root";
import { SessionGate } from "./SessionGate";
import "./styles.css";

const root = document.getElementById("root");
if (root === null) {
  throw new Error("Application root element was not found");
}

createRoot(root).render(
  <StrictMode>
    <ThemeProvider>
      <SessionGate>
        <Root />
      </SessionGate>
    </ThemeProvider>
  </StrictMode>,
);
