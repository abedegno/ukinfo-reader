"use strict";
(() => {
  const bridge = window.ukinfoDesktop;
  if (!bridge?.geographySource) return;
  const byId = (id) => document.getElementById(id);
  let selected = null,
    running = false,
    source = null;
  const message = (text) => {
    byId("postcode-message").textContent = text;
  };
  const busy = (value) => {
    running = value;
    byId("postcode-choose").disabled = value || !source;
    byId("postcode-start").disabled = value || !selected || !source;
    byId("postcode-cancel").hidden = !value;
    byId("postcode-cancel").disabled = false;
    byId("postcode-progress").hidden = !value;
  };
  async function status() {
    const response = await fetch("/api/geography");
    if (!response.ok)
      throw new Error("Unable to check the current postcode reference.");
    const value = await response.json();
    byId("postcode-reference-status").textContent = value.available
      ? `Installed: ONSPD ${value.reference.release} · ${value.reference.entries.toLocaleString()} Great Britain postcodes.`
      : "No postcode reference installed yet.";
    return value;
  }
  byId("postcode-setup-open").addEventListener("click", async () => {
    byId("postcode-setup-dialog").showModal();
    if (running) return;
    busy(false);
    try {
      source = await bridge.geographySource();
      byId("postcode-release").textContent = source.release;
      byId("postcode-download").href = source.url;
      await status();
      message(
        "Download only the listed release; other ZIPs will fail verification.",
      );
    } catch (error) {
      message(error.message);
    }
    busy(false);
  });
  byId("postcode-setup-close").addEventListener("click", () =>
    byId("postcode-setup-dialog").close(),
  );
  byId("postcode-choose").addEventListener("click", async () => {
    busy(true);
    try {
      const value = await bridge.chooseGeography();
      if (value) {
        selected = value;
        byId("postcode-selected").textContent = value.label;
        message("Ready to verify and import.");
      }
    } catch (error) {
      message(error.message);
    } finally {
      busy(false);
    }
  });
  bridge.onGeographyProgress((state) => {
    if (!running) return;
    const p = state.progress || {};
    const names = {
      "verifying postcode archive": "Verifying the ONSPD ZIP…",
      "loading postcodes": "Loading postcode locations…",
      "checking postcode archive": "Checking the source has not changed…",
      "indexing postcodes": "Preparing nearby search…",
    };
    message(
      `${names[p.phase] || p.phase || "Importing…"}${p.done ? ` · ${p.done.toLocaleString()} postcodes` : ""}`,
    );
  });
  byId("postcode-start").addEventListener("click", async () => {
    if (running || !selected) return;
    busy(true);
    message(
      "Verifying the ONSPD ZIP… You can close this panel and keep searching.",
    );
    try {
      await bridge.importGeography(selected.selection);
      message(
        "Postcode locations are ready. Nearby search and maps are now available.",
      );
    } catch (error) {
      message(error.message);
    } finally {
      busy(false);
      try {
        await status();
      } catch (error) {
        message(error.message);
      }
      window.dispatchEvent(new Event("ukinfo-geography-changed"));
    }
  });
  byId("postcode-cancel").addEventListener("click", async () => {
    byId("postcode-cancel").disabled = true;
    message("Canceling postcode import…");
    try {
      await bridge.cancelImport();
    } catch (error) {
      message(error.message);
    }
  });
  bridge
    .capabilities()
    .then((c) => {
      byId("postcode-setup-open").hidden = !c.canImportGeography;
    })
    .catch(() => {});
})();
