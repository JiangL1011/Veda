import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { hostPlatform } from "./lib/utils";
import { installWindowDragCursor } from "./lib/windowDrag";
import "vditor/dist/index.css";
import "./index.css";

document.documentElement.dataset.platform = hostPlatform();
installWindowDragCursor();

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
