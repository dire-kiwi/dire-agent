import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { DocsApp } from "./docs/DocsApp";
import "./index.css";

const Root = window.location.pathname.startsWith("/docs") ? DocsApp : App;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
