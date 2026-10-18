"use strict";
(() => {
  const bridge = window.ukinfoDesktop;
  if (!bridge?.catalogue) return;
  const $ = (id) => document.getElementById(id);
  const el = (tag, text, className) => {
    const node = document.createElement(tag);
    if (text != null) node.textContent = text;
    if (className) node.className = className;
    return node;
  };
  const size = (bytes) => {
    if (!Number.isFinite(bytes)) return "Unavailable";
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    let value = bytes,
      unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit++;
    }
    return `${value.toLocaleString(undefined, { maximumFractionDigits: 1 })} ${units[unit]}`;
  };
  let report = null,
    busy = false;
  const action = (label, callback) => {
    const button = el("button", label);
    button.type = "button";
    button.disabled = busy || Boolean(report?.state?.running);
    button.addEventListener("click", callback);
    return button;
  };
  function openImport(detail = {}) {
    $("catalogue-dialog").close();
    window.dispatchEvent(new CustomEvent("ukinfo-import-request", { detail }));
  }
  function render() {
    $("catalogue-storage").replaceChildren();
    for (const [label, bytes] of [
      ["Database files", report.database_bytes],
      ["Shared search dictionaries & addresses", report.shared_bytes],
      ["Postcode reference", report.geography_bytes],
      ["Free on archive volume", report.free_bytes],
    ]) {
      const item = el("div");
      item.append(el("dt", label), el("dd", size(bytes)));
      $("catalogue-storage").append(item);
    }
    $("catalogue-editions").replaceChildren();
    for (const item of report.datasets) {
      const card = el("article", null, "catalogue-edition");
      const demo = item.manifest.edition === "synthetic-demo";
      const full = report.datasets.some(
        (d) =>
          d.manifest.edition === item.manifest.edition && !d.manifest.sample,
      );
      card.append(
        el("h3", item.manifest.title),
        el(
          "p",
          `${demo ? "Invented demo" : item.manifest.sample ? "Sample" : "Complete import"} · ${item.entries.toLocaleString()} entries · ${size(item.bytes)} dedicated storage`,
          "catalogue-meta",
        ),
      );
      const date = new Date(item.imported_at);
      card.append(
        el(
          "p",
          `${Number.isNaN(date.valueOf()) ? "Import date unavailable" : `Imported ${date.toLocaleString()}`} · ${item.address_ready ? "Street, town and county search ready" : "Address search needs finishing"}`,
          "note",
        ),
      );
      const actions = el("div", null, "catalogue-actions");
      if (item.manifest.sample && !full)
        actions.append(
          action("Add complete edition", () =>
            openImport({
              edition: item.manifest.edition,
              title: item.manifest.title,
              full: true,
              previous: report.setup?.edition === item.manifest.edition,
            }),
          ),
        );
      if (item.manifest.sample && full)
        card.append(
          el(
            "p",
            "A complete import is available. All-editions search already prefers it; this sample can be removed when you are ready.",
            "note",
          ),
        );
      if (!item.address_ready)
        actions.append(
          action("Finish address search", () => manage(item, "index")),
        );
      const remove = action("Remove edition…", () => manage(item, "remove"));
      remove.className = "quiet-danger";
      actions.append(remove);
      card.append(actions);
      $("catalogue-editions").append(card);
    }
    $("catalogue-empty").hidden = report.datasets.length > 0;
    const previous = report.setup;
    const target = previous
      ? previous.edition + (previous.limit ? `:sample:${previous.limit}` : "")
      : "";
    const pending =
      previous &&
      !report.datasets.some((d) => d.key === target) &&
      previous.status !== "complete";
    $("catalogue-continue").hidden = !pending;
    $("catalogue-continue-label").textContent = pending
      ? `${previous.title || previous.label} · ${["running", "preparing"].includes(previous.status) ? "Setup was interrupted" : "Setup is unfinished"}. Recheck the source and restart the import from the beginning.`
      : "";
    $("catalogue-resume").disabled = busy || Boolean(report.state?.running);
    $("catalogue-geo-status").textContent = report.geography
      ? `ONSPD ${report.geography.release} · ${report.geography.entries.toLocaleString()} Great Britain postcodes`
      : "No postcode reference installed. Nearby search and maps need postcode setup.";
    if (report.state?.running)
      $("catalogue-message").textContent =
        `Working: ${report.state.progress?.phase || "catalogue operation"}. You can keep searching existing editions.`;
    if (report.setupWarning)
      $("catalogue-message").textContent = report.setupWarning;
  }
  async function refresh() {
    report = await bridge.catalogue();
    render();
  }
  async function manage(item, type) {
    if (busy) return;
    busy = true;
    render();
    $("catalogue-message").textContent =
      type === "remove"
        ? "Review the removal confirmation…"
        : "Preparing address search… This may take several minutes.";
    $("catalogue-cancel").hidden = false;
    try {
      const result = await bridge.manageDataset(item.selection, type);

      $("catalogue-message").textContent = result.canceled
        ? "Edition kept."
        : type === "remove"
          ? "Edition removed. Research lists and notes are preserved."
          : "Address search is ready.";
    } catch (error) {
      $("catalogue-message").textContent = error.message;
    } finally {
      busy = false;
      $("catalogue-cancel").hidden = true;
      try {
        await refresh();
      } catch (error) {
        $("catalogue-message").textContent = error.message;
      }
      window.dispatchEvent(new Event("ukinfo-catalogue-changed"));
    }
  }
  $("catalogue-open").addEventListener("click", async () => {
    $("catalogue-dialog").showModal();
    $("catalogue-dialog").scrollTop = 0;
    $("catalogue-message").textContent = "Loading your archive…";
    try {
      await refresh();
      $("catalogue-dialog").scrollTop = 0;
      if (!report.state?.running && !report.setupWarning)
        $("catalogue-message").textContent = "";
    } catch (error) {
      $("catalogue-message").textContent = error.message;
    }
  });
  $("catalogue-refresh").addEventListener("click", () =>
    refresh().catch((error) => {
      $("catalogue-message").textContent = error.message;
    }),
  );
  $("catalogue-cancel").addEventListener("click", () => {
    $("catalogue-message").textContent = "Canceling operation…";
    bridge.cancelImport().catch((error) => {
      $("catalogue-message").textContent = error.message;
    });
  });
  bridge.onCatalogueProgress?.((state) => {
    if (!busy) return;
    const p = state.progress || {};
    $("catalogue-message").textContent =
      `${p.phase || "Working…"}${p.done ? ` · ${p.done.toLocaleString()} entries` : ""}`;
  });
  $("catalogue-close").addEventListener("click", () =>
    $("catalogue-dialog").close(),
  );
  $("catalogue-add").addEventListener("click", () => openImport());
  $("catalogue-resume").addEventListener("click", () =>
    openImport({ resume: true }),
  );
  $("catalogue-geography").addEventListener("click", () => {
    $("catalogue-dialog").close();
    $("postcode-setup-open").click();
  });
  $("catalogue-backup-notes").addEventListener("click", () => {
    $("catalogue-dialog").close();
    $("research-open").click();
  });
  $("catalogue-show-folder").addEventListener("click", () =>
    bridge.showArchiveFolder().catch((error) => {
      $("catalogue-message").textContent = error.message;
    }),
  );
  $("catalogue-diagnostics").addEventListener("click", async () => {
    const button = $("catalogue-diagnostics");
    button.disabled = true;
    $("diagnostics-message").textContent = "Choose where to save diagnostics…";
    try {
      const result = await bridge.exportDiagnostics();
      $("diagnostics-message").textContent = result.canceled
        ? "Export canceled."
        : result.busy
          ? "Another diagnostics export is open."
          : "Diagnostics saved. Review the JSON, then attach it to your issue.";
    } catch (error) {
      $("diagnostics-message").textContent = error.message;
    } finally {
      button.disabled = false;
    }
  });
  if (bridge.archiveMaintenance) {
    for (const [id, action] of [
      ["catalogue-backup", "backup"],
      ["catalogue-restore", "restore"],
      ["catalogue-switch", "open"],
    ]) {
      $(id).addEventListener("click", async () => {
        $("catalogue-message").textContent = "Review the archive operation…";
        try {
          const result = await bridge.archiveMaintenance(action);
          $("catalogue-message").textContent = result.canceled
            ? "Operation canceled. Your archive is unchanged."
            : "Restarting…";
        } catch (error) {
          $("catalogue-message").textContent = error.message;
        }
      });
    }
    bridge.onArchiveProgress((state) => {
      const modal = $("archive-copy-dialog");
      if (state.phase === "idle") {
        if (modal.open) modal.close();
        return;
      }
      if (!modal.open) modal.showModal();
      const progress = $("archive-copy-progress");
      if (state.total > 0) {
        progress.max = state.total;
        progress.value = state.done;
      } else progress.removeAttribute("value");
      $("archive-copy-status").textContent =
        `${state.phase === "stopping" ? "Stopping the database safely…" : state.phase === "verifying" ? "Verifying copied files…" : state.phase === "complete" ? "Copy verified." : "Copying files…"}${state.total ? ` ${size(state.done)} of ${size(state.total)}` : ""}`;
    });
    $("archive-copy-cancel").addEventListener("click", () =>
      bridge.cancelArchiveCopy(),
    );
    $("archive-copy-dialog").addEventListener("cancel", (event) => {
      event.preventDefault();
      bridge.cancelArchiveCopy();
    });
  } else
    for (const id of [
      "catalogue-backup",
      "catalogue-restore",
      "catalogue-switch",
    ])
      $(id).hidden = true;
  bridge
    .capabilities()
    .then((c) => {
      $("catalogue-open").hidden = !c.canManageCatalogue;
    })
    .catch(() => {});
})();
