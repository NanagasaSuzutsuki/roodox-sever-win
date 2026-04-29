import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import ErrorBoundary from "./ErrorBoundary";
import "./styles.css";
import { workbenchStorageKeys } from "./workbench/persistence";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary storageKeys={workbenchStorageKeys}>
      <App />
    </ErrorBoundary>
  </React.StrictMode>
);
