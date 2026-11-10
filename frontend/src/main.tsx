import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// Outfit (SIL OFL), bundled rather than fetched from Google Fonts: a deployment's pages ask nothing
// of an outside server, and work on a network without Internet access.
import "@fontsource-variable/outfit";
import "./index.css";
import "swiper/swiper-bundle.css";
import "flatpickr/dist/flatpickr.css";
import "./i18n";
import App from "./App.tsx";
import { ThemeProvider } from "./context/ThemeContext.tsx";
import { loadRuntimeConfig } from "./runtimeConfig";

// The server's runtime configuration first, so nothing renders with a build's defaults by mistake.
loadRuntimeConfig().then(() => {
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <ThemeProvider>
        <App />
      </ThemeProvider>
    </StrictMode>,
  );
});
