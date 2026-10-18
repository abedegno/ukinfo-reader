"use strict";
const { contextBridge, ipcRenderer } = require("electron");
contextBridge.exposeInMainWorld("ukinfoDesktop", {
  researchRead: () => ipcRenderer.invoke("ukinfo:research-read"),
  researchWrite: (value, revision) =>
    ipcRenderer.invoke("ukinfo:research-write", { value, revision }),
  showArchiveFolder: () => ipcRenderer.invoke("ukinfo:show-archive-folder"),
  catalogue: () => ipcRenderer.invoke("ukinfo:catalogue"),
  manageDataset: (selection, action) =>
    ipcRenderer.invoke("ukinfo:manage-dataset", { selection, action }),
  onCatalogueProgress: (callback) => {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on("ukinfo:catalogue-progress", listener);
    return () =>
      ipcRenderer.removeListener("ukinfo:catalogue-progress", listener);
  },
  resumeSetup: () => ipcRenderer.invoke("ukinfo:resume-setup"),
  capabilities: () => ipcRenderer.invoke("ukinfo:capabilities"),
  geographySource: () => ipcRenderer.invoke("ukinfo:geography-source"),
  chooseGeography: () => ipcRenderer.invoke("ukinfo:choose-geography"),
  importGeography: (selection) =>
    ipcRenderer.invoke("ukinfo:import-geography", selection),
  onGeographyProgress: (callback) => {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on("ukinfo:geography-progress", listener);
    return () =>
      ipcRenderer.removeListener("ukinfo:geography-progress", listener);
  },
  chooseMedia: () => ipcRenderer.invoke("ukinfo:choose-media"),
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
