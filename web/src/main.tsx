import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { createHttpApi } from "./api/http";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App api={createHttpApi()} />
  </StrictMode>,
);
