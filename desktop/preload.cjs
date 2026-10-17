"use strict";
const { contextBridge, ipcRenderer } = require("electron");
contextBridge.exposeInMainWorld("ukinfoDesktop", {
  capabilities: () => ipcRenderer.invoke("ukinfo:capabilities"),
  chooseSource: () => ipcRenderer.invoke("ukinfo:choose-source"),
  demoSource: () => ipcRenderer.invoke("ukinfo:demo"),
  importSource: (selection, limit) =>
    ipcRenderer.invoke("ukinfo:import", { selection, limit }),
  cancelImport: () => ipcRenderer.invoke("ukinfo:cancel-import"),
  onProgress: (callback) => {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on("ukinfo:import-progress", listener);
    return () => ipcRenderer.removeListener("ukinfo:import-progress", listener);
  },
});
