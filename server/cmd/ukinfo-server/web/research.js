"use strict";
(() => {
  const model = window.UKResearch,
    key = "ukinfo.research.v1";
  let data = model.empty(),
    selected = "",
    pending = null,
    incoming = null,
    nameMode = "new",
    shown = 50,
    mutationQueue = Promise.resolve();
  const checked = new Set(),
    drafts = new Map();
  const bridge = window.ukinfoDesktop;
  const native = Boolean(bridge?.researchRead && bridge?.researchWrite);
  const persistedDrafts = new Map(),
    draftBases = new Map(),
    pendingDrafts = new Set();
  let draftTimer,
    draftQueue = Promise.resolve(),
    draftReadError = null;
  const draftsReady = window.researchWorkspace
    .read()
    .then((value) => {
      for (const draft of value.drafts) {
        persistedDrafts.set(draft.id, draft);
        drafts.set(draft.id, draft.note);
        draftBases.set(draft.id, draft.base);
      }
    })
    .catch((error) => {
      draftReadError = error;
    });
  async function flushDrafts() {
    clearTimeout(draftTimer);
    const run = async () => {
      await draftsReady;
      if (draftReadError) throw draftReadError;
      const ids = [...pendingDrafts];
      if (!ids.length) return;
      pendingDrafts.clear();
      const snapshot = new Map(
        ids.map((id) => [
          id,
          drafts.has(id)
            ? {
                id,
                note: drafts.get(id),
                base: draftBases.get(id) || "",
                updated: new Date().toISOString(),
              }
            : null,
        ]),
      );
      try {
        await window.researchWorkspace.update((value) => {
          for (const id of ids) {
            const current =
              value.drafts.find((draft) => draft.id === id) || null;
            if (
              JSON.stringify(current) !==
              JSON.stringify(persistedDrafts.get(id) || null)
            )
              throw new Error(
                "A note draft changed in another window. Your edits remain here; copy them before reloading. The other window's draft has been preserved.",
              );
          }
          value.drafts = value.drafts.filter(
            (draft) => !snapshot.has(draft.id),
          );
          for (const draft of snapshot.values())
            if (draft) value.drafts.push(draft);
        });
        for (const [id, value] of snapshot)
          value ? persistedDrafts.set(id, value) : persistedDrafts.delete(id);
      } catch (error) {
        for (const id of ids) pendingDrafts.add(id);
        throw error;
      }
    };
    const operation = draftQueue.then(run);
    draftQueue = operation.catch(() => {});
    return operation;
  }
  function draftChanged(id, base) {
    if (!draftBases.has(id)) draftBases.set(id, base);
    pendingDrafts.add(id);
    clearTimeout(draftTimer);
    draftTimer = setTimeout(() => {
      flushDrafts()
        .then(() =>
          draftStatus(
            drafts.size
              ? "Drafts saved for recovery on this device. Use Save note to commit them."
              : "",
          ),
        )
        .catch((error) =>
          draftStatus(`Draft recovery unavailable: ${error.message}`),
        );
    }, 300);
  }
  window.flushResearchDrafts = flushDrafts;
  window.discardResearchDrafts = async () => {
    await draftsReady;
    for (const id of drafts.keys()) {
      drafts.delete(id);
      pendingDrafts.add(id);
    }
    await flushDrafts();
  };
  const byId = (id) => data.lists.find((list) => list.id === id);
  const current = () => byId(selected);
  const uuid = () => crypto.randomUUID();
  function storageRead() {
    return native
      ? bridge.researchRead()
      : Promise.resolve().then(() => {
          const raw = localStorage.getItem(key);
          return raw === null ? model.empty() : model.parse(raw);
        });
  }
  async function load() {
    await draftsReady;
    if (draftReadError) throw draftReadError;
    data = model.validate(await storageRead());
    if (!byId(selected)) selected = data.lists[0]?.id || "";
  }
  function mutate(change) {
    const run = async () => {
      const apply = async () => {
        const stored = model.validate(await storageRead());
        if (stored.revision !== data.revision)
          throw new Error(
            "Research lists changed in another window. Reload lists before saving; your unsaved notes are still here.",
          );
        const next = structuredClone(data);
        change(next);
        next.revision++;
        const validated = model.validate(next);
        if (native) await bridge.researchWrite(validated, data.revision);
        else {
          const original = localStorage.getItem(key);
          if (
            original &&
            JSON.parse(original).version === 1 &&
            localStorage.getItem(key + ".v1-backup") === null
          )
            localStorage.setItem(key + ".v1-backup", original);
          localStorage.setItem(key, JSON.stringify(validated));
        }
        data = validated;
      };
      if (!native) {
        if (!navigator.locks)
          throw new Error(
            "Saving research lists needs HTTPS or localhost in a browser with Web Locks support. JSON export is still available.",
          );
        await navigator.locks.request(key, apply);
      } else await apply();
    };
    const pending = mutationQueue.then(run);
    mutationQueue = pending.catch(() => {});
    return pending;
  }

  const status = (message) => ($("research-status").textContent = message);
  const draftStatus = (message) =>
    ($("research-draft-status").textContent = message);
  async function action(change, message) {
    try {
      await mutate(change);
      render();
      status(message);
      return true;
    } catch (error) {
      status(
        error.message ||
          "Local storage is unavailable or full. Export a backup before clearing browser data.",
      );
      return false;
    }
  }
  const importSignature = (list, entry) =>
    JSON.stringify([
      list.name,
      model.key(entry),
      entry.note,
      entry.status || "",
      entry.tags || [],
      entry.summary,
    ]);
  function importPreview() {
    if (!incoming) return;
    const signatures = new Set(),
      refs = new Set();
    for (const list of data.lists)
      for (const entry of list.entries) {
        signatures.add(importSignature(list, entry));
        refs.add(JSON.stringify([list.name, model.key(entry)]));
      }
    let identical = 0,
      versions = 0;
    for (const list of incoming.lists)
      for (const entry of list.entries) {
        if (signatures.has(importSignature(list, entry))) identical++;
        else if (refs.has(JSON.stringify([list.name, model.key(entry)])))
          versions++;
      }
    $("research-import-preview").textContent =
      `${incoming.lists.length} lists · ${incoming.lists.reduce((count, list) => count + list.entries.length, 0)} entries · ${identical} exact duplicates · ${versions} source references with different saved details. ${$("research-skip-duplicates").checked ? "Exact duplicates will be skipped." : "Duplicates will be added as separate copies."} Existing lists and notes are kept; differing versions are added separately.`;
  }
  $("research-skip-duplicates").addEventListener("change", importPreview);
  function selection() {
    return (current()?.entries || []).filter((entry) => checked.has(entry.id));
  }
  function updateSelection() {
    const n = selection().length;
    $("research-compare").disabled = n < 1 || n > 4;
    $("research-map").disabled = n < 1 || n > 100;
  }
  function render() {
    const select = $("research-list");
    select.replaceChildren();
    for (const list of data.lists) {
      const option = element("option", `${list.name} (${list.entries.length})`);
      option.value = list.id;
      select.append(option);
    }
    if (!byId(selected)) selected = data.lists[0]?.id || "";
    select.value = selected;
    select.disabled = !data.lists.length;
    for (const id of ["research-rename", "research-delete", "research-csv"])
      $(id).disabled = !current();
    const container = $("research-entries");
    container.replaceChildren();
    const term = $("research-filter").value.trim().toLocaleLowerCase();
    const entries = (current()?.entries || []).filter((entry) =>
      [
        ...Object.values(entry.summary),
        entry.status,
        ...(entry.tags || []),
        drafts.get(entry.id) ?? entry.note,
      ]
        .join(" ")
        .toLocaleLowerCase()
        .includes(term),
    );
    if (!entries.length)
      container.append(
        element(
          "p",
          current()?.entries.length
            ? "No saved entries match this filter."
            : "No entries yet. Open a source record or address and save it to a list.",
          "note",
        ),
      );
    for (const entry of entries.slice(0, shown)) {
      const card = element("article", "", "research-card"),
        body = element("div", "", "research-entry");
      const label = element("label", "", "check"),
        checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = checked.has(entry.id);
      checkbox.setAttribute(
        "aria-label",
        `Select ${entry.summary.title || "saved address"}`,
      );
      checkbox.addEventListener("change", () => {
        checkbox.checked ? checked.add(entry.id) : checked.delete(entry.id);
        updateSelection();
      });
      label.append(
        checkbox,
        document.createTextNode(
          `${entry.mode === "address" ? "Address: " : ""}${entry.summary.title || "Unnamed listing"}`,
        ),
      );
      body.append(
        label,
        element("p", entry.summary.address, "address-heading"),
        element("p", entry.summary.edition, "badge"),
        element("p", entry.summary.citation, "note"),
      );
      if (entry.summary.warnings)
        body.append(element("p", entry.summary.warnings, "warning"));
      const labels = element("details", ""),
        labelSummary = element(
          "summary",
          entry.status || entry.tags?.length
            ? `Research labels: ${(entry.status || "").replaceAll("-", " ") || "no status"}${entry.tags?.length ? " · " + entry.tags.join(", ") : ""}`
            : "Add research status or tags",
        );
      const statusLabel = element("label", "Research status"),
        statusSelect = element("select", "");
      statusSelect.setAttribute(
        "aria-label",
        `Research status for ${entry.summary.title}`,
      );
      for (const [value, title] of [
        ["", "Not set"],
        ["candidate", "Candidate"],
        ["follow-up", "Follow up"],
        ["ruled-out", "Ruled out"],
        ["reviewed", "Reviewed"],
      ]) {
        const option = element("option", title);
        option.value = value;
        statusSelect.append(option);
      }
      statusSelect.value = entry.status || "";
      statusLabel.append(statusSelect);
      const tagLabel = element("label", "Tags (comma separated)"),
        tagsInput = element("input", "");
      tagsInput.value = (entry.tags || []).join(", ");
      tagsInput.maxLength = 400;
      tagsInput.setAttribute("aria-label", `Tags for ${entry.summary.title}`);
      tagLabel.append(tagsInput);
      const saveLabels = element("button", "Save labels");
      saveLabels.type = "button";
      saveLabels.addEventListener("click", () => {
        const savedStatus = statusSelect.value,
          tags = [
            ...new Set(
              tagsInput.value
                .split(",")
                .map((value) => value.trim())
                .filter(Boolean),
            ),
          ];
        if (tags.length > 10 || tags.some((tag) => tag.length > 40)) {
          status("Use at most 10 tags of 40 characters each.");
          return;
        }
        action((next) => {
          const target = next.lists
            .flatMap((list) => list.entries)
            .find((item) => item.id === entry.id);
          if (!target) throw new Error("Entry no longer exists.");
          target.status = savedStatus;
          target.tags = tags;
          target.updated = new Date().toISOString();
        }, "Research labels saved locally.");
      });
      labels.append(
        labelSummary,
        element(
          "p",
          "These are your research judgements. They do not change the source listing or establish a person's identity.",
          "note",
        ),
        statusLabel,
        tagLabel,
        saveLabels,
      );
      body.append(labels);
      const noteLabel = element("label", "Note"),
        textarea = document.createElement("textarea");
      textarea.rows = 3;
      textarea.maxLength = 4000;
      textarea.value = drafts.get(entry.id) ?? entry.note;
      textarea.setAttribute("aria-label", `Note for ${entry.summary.title}`);
      noteLabel.append(textarea);
      if (
        drafts.has(entry.id) &&
        draftBases.get(entry.id) !== undefined &&
        draftBases.get(entry.id) !== entry.note
      ) {
        const comparison = element("details", ""),
          summary = element(
            "summary",
            "Saved note changed since this draft began — compare before saving",
          );
        comparison.append(
          summary,
          element("p", `Current saved note: ${entry.note || "(empty)"}`),
          element(
            "p",
            `Draft started from: ${draftBases.get(entry.id) || "(empty)"}`,
          ),
        );
        body.append(comparison);
      }
      body.append(noteLabel);
      const actions = element("div", "", "record-actions");
      const noteButton = element(
        "button",
        drafts.has(entry.id) ? "Save note" : "Note saved",
      );
      noteButton.type = "button";
      noteButton.disabled = !drafts.has(entry.id);
      textarea.addEventListener("input", () => {
        if (textarea.value === entry.note) drafts.delete(entry.id);
        else drafts.set(entry.id, textarea.value);
        draftChanged(entry.id, entry.note);
        noteButton.disabled = !drafts.has(entry.id);
        noteButton.textContent = noteButton.disabled
          ? "Note saved"
          : "Save note";
      });
      noteButton.addEventListener("click", async () => {
        const value = textarea.value;
        if (
          await action((next) => {
            const item = next.lists
              .flatMap((l) => l.entries)
              .find((e) => e.id === entry.id);
            if (!item) throw new Error("Entry no longer exists.");
            item.note = value;
            item.updated = new Date().toISOString();
          }, "Note saved locally.")
        ) {
          if (drafts.get(entry.id) === value) {
            drafts.delete(entry.id);
            draftChanged(entry.id, value);
          }
          render();
        }
      });
      const discard = element("button", "Discard draft");
      discard.type = "button";
      discard.disabled = !drafts.has(entry.id);
      textarea.addEventListener("input", () => {
        discard.disabled = !drafts.has(entry.id);
      });
      discard.addEventListener("click", async () => {
        drafts.delete(entry.id);
        draftChanged(entry.id, entry.note);
        try {
          await flushDrafts();
          render();
          status("Draft discarded; saved note retained.");
        } catch (error) {
          status(error.message);
        }
      });
      const open = element("button", "Open source");
      open.type = "button";
      open.addEventListener("click", async () => {
        try {
          status("Opening source…");
          const metadata = await api(`/api/record?${recordParams(entry.ref)}`);
          if (entry.mode === "address" && !metadata.address_available)
            throw new Error(metadata.address_reason);
          $("research-dialog").close();
          if (entry.mode === "address")
            openAddress(metadata.item, { scope: entry.scope });
          else details(metadata.item, [metadata.item], { metadata });
        } catch (error) {
          status(error.message);
        }
      });
      const remove = element("button", "Remove entry");
      remove.type = "button";
      let confirm = false;
      remove.addEventListener("click", async () => {
        if (!confirm) {
          confirm = true;
          remove.textContent = "Confirm removal and delete note";
          return;
        }
        if (
          await action((next) => {
            const list = next.lists.find((l) =>
              l.entries.some((e) => e.id === entry.id),
            );
            if (!list) throw new Error("Entry no longer exists.");
            list.entries = list.entries.filter((e) => e.id !== entry.id);
          }, "Entry removed.")
        ) {
          drafts.delete(entry.id);
          draftChanged(entry.id, entry.note);
          checked.delete(entry.id);
          render();
        }
      });
      actions.append(open, noteButton, discard, remove);
      body.append(actions);
      card.append(body);
      container.append(card);
    }
    $("research-more").hidden = entries.length <= shown;
    updateSelection();
  }
  $("research-storage-note").textContent = native
    ? "Stored in this app's data directory, independent of its local server address. Export JSON backups to move or restore your lists."
    : "Stored in this browser for this site only. Clearing site data, private browsing or changing sites can lose these lists. Export JSON backups to keep them.";
  $("research-open").addEventListener("click", async () => {
    try {
      await load();
      shown = 50;
      render();
      status(
        drafts.size
          ? "Recovered note drafts are shown below. Drafts are stored on this device; use Save note to commit them."
          : `${data.lists.length} research lists stored locally.`,
      );
    } catch (error) {
      status(error.message);
    }
    $("research-dialog").showModal();
  });
  $("research-close").addEventListener("click", () =>
    $("research-dialog").close(),
  );
  $("research-list").addEventListener("change", () => {
    selected = $("research-list").value;
    checked.clear();
    shown = 50;
    $("research-delete-confirm").hidden = true;
    $("research-name-form").hidden = true;
    render();
  });
  $("research-filter").addEventListener("input", () => {
    shown = 50;
    render();
  });
  $("research-more").addEventListener("click", () => {
    shown += 50;
    render();
  });
  $("research-reload").addEventListener("click", async () => {
    try {
      await load();
      render();
      status(
        "Lists reloaded. Unsaved drafts in this window have been retained.",
      );
    } catch (error) {
      status(error.message);
    }
  });
  for (const [id, mode] of [
    ["research-new", "new"],
    ["research-rename", "rename"],
  ])
    $(id).addEventListener("click", () => {
      nameMode = mode;
      $("research-name-form").hidden = false;
      $("research-name").value = mode === "rename" ? current().name : "";
      $("research-name").focus();
    });
  $("research-name-cancel").addEventListener(
    "click",
    () => ($("research-name-form").hidden = true),
  );
  $("research-name-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const name = $("research-name").value.trim();
    if (!name) return;
    const id = uuid();
    const targetList = selected,
      mode = nameMode;
    if (
      await action((next) => {
        if (mode === "new") next.lists.push({ id, name, entries: [] });
        else next.lists.find((l) => l.id === targetList).name = name;
      }, "List saved.")
    ) {
      if (mode === "new") selected = id;
      $("research-name-form").hidden = true;
      render();
    }
  });
  $("research-delete").addEventListener(
    "click",
    () => ($("research-delete-confirm").hidden = false),
  );
  $("research-delete-no").addEventListener(
    "click",
    () => ($("research-delete-confirm").hidden = true),
  );
  $("research-delete-yes").addEventListener("click", async () => {
    const id = selected;
    const ids = current()?.entries.map((e) => e.id) || [];
    if (
      await action(
        (next) => (next.lists = next.lists.filter((l) => l.id !== id)),
        "List deleted.",
      )
    ) {
      for (const id of ids) {
        drafts.delete(id);
        draftChanged(id, "");
      }
      checked.clear();
      $("research-delete-confirm").hidden = true;
    }
  });
  $("research-json").addEventListener("click", async () => {
    try {
      const stored = model.validate(await storageRead());
      downloadBlob(
        new Blob([JSON.stringify(stored)], {
          type: "application/json",
        }),
        "ukinfo-research.json",
      );
      status("Exported saved lists. Unsaved note drafts are not included.");
    } catch (error) {
      status(error.message);
    }
  });
  $("research-csv").addEventListener("click", () => {
    if (current())
      downloadBlob(
        new Blob([model.csv(current())], { type: "text/csv;charset=utf-8" }),
        "ukinfo-research-list.csv",
      );
  });
  $("research-file").addEventListener("change", async () => {
    incoming = null;
    $("research-import").disabled = true;
    try {
      const file = $("research-file").files[0];
      if (!file) return;
      if (file.size > model.MAX_BYTES)
        throw new Error("Research JSON must be at most 2 MiB.");
      incoming = model.parse(await file.text());
      importPreview();
      $("research-import").disabled = !incoming.lists.length;
    } catch (error) {
      $("research-import-preview").textContent = error.message;
    }
  });
  $("research-import").addEventListener("click", async () => {
    if (!incoming) return;
    if (
      await action((next) => {
        const signatures = new Set(
          next.lists.flatMap((list) =>
            list.entries.map((entry) => importSignature(list, entry)),
          ),
        );
        const skip = $("research-skip-duplicates").checked;
        for (const list of incoming.lists) {
          const copy = structuredClone(list);
          copy.entries = copy.entries.filter(
            (entry) => !skip || !signatures.has(importSignature(list, entry)),
          );
          if (skip && list.entries.length && !copy.entries.length) continue;
          if (
            skip &&
            !list.entries.length &&
            next.lists.some(
              (existing) =>
                existing.name === list.name && !existing.entries.length,
            )
          )
            continue;
          for (const entry of copy.entries)
            signatures.add(importSignature(list, entry));
          copy.id = uuid();
          for (const entry of copy.entries) entry.id = uuid();
          next.lists.push(copy);
        }
      }, "Imported lists added; existing notes preserved.")
    ) {
      incoming = null;
      $("research-file").value = "";
      $("research-import").disabled = true;
      $("research-import-preview").textContent = "Import complete.";
    }
  });
  $("research-compare").addEventListener("click", () => {
    const entries = selection();
    if (entries.length < 1 || entries.length > 4) return;
    const table = element("table", ""),
      head = element("tr", "");
    head.append(element("th", "Field"));
    for (const entry of entries)
      head.append(element("th", entry.summary.title));
    table.append(head);
    for (const [label, field] of [
      ["Edition", "edition"],
      ["Name", "title"],
      ["Address", "address"],
      ["Listing type", "listing"],
      ["Telephone", "phone"],
      ["Citation", "citation"],
      ["Saved note", "note"],
    ]) {
      const row = element("tr", "");
      row.append(element("th", label));
      for (const entry of entries)
        row.append(
          element(
            "td",
            (field === "note" ? entry.note : entry.summary[field]) ||
              "Not recorded",
          ),
        );
      table.append(row);
    }
    $("research-comparison-content").replaceChildren(table);
    $("research-comparison").showModal();
  });
  $("research-comparison-close").addEventListener("click", () =>
    $("research-comparison").close(),
  );
  $("research-map").addEventListener("click", () => {
    const entries = selection();
    if (entries.length && entries.length <= 100)
      window.openArchiveMap(
        entries.map((e) => ({
          label: e.summary.title,
          edition: e.summary.edition,
          postcode: e.summary.postcode,
        })),
      );
  });
  async function save(mode, item, scope = "edition") {
    if (!hasRecordID(item) || !item.source_id) return;
    try {
      await load();
      const r = item.record;
      pending = {
        id: uuid(),
        mode,
        scope,
        ref: {
          dataset: item.dataset,
          id: item.id,
          source_id: item.source_id,
          ...(item.archive_id ? { archive_id: item.archive_id } : {}),
        },
        summary: {
          title: recordName(r) || "Unnamed listing",
          address: addressText(r),
          postcode: String(r.postcode || ""),
          edition: editionTitle(item),
          listing: listingType(r),
          phone: String(r.phone || ""),
          citation: citation(item),
          warnings: warnings(r).join("; "),
        },
        note: "",
        created: new Date().toISOString(),
        updated: new Date().toISOString(),
      };
      $("research-save-summary").textContent =
        `${mode === "address" ? "Address anchored by " : ""}${pending.summary.title} · ${pending.summary.edition}`;
      const select = $("research-save-list");
      select.replaceChildren();
      for (const list of data.lists) {
        const option = element("option", list.name);
        option.value = list.id;
        select.append(option);
      }
      const option = element("option", "Create a new list…");
      option.value = "";
      select.append(option);
      select.value = byId(selected) ? selected : data.lists[0]?.id || "";
      $("research-new-name-label").hidden = Boolean(select.value);
      $("research-new-name").value = "";
      $("research-save-note").value = "";
      $("research-save-status").textContent = "";
      $("research-save").showModal();
    } catch (error) {
      $(mode === "address" ? "address-status" : "record-feedback").textContent =
        error.message;
    }
  }
  $("save-record").addEventListener("click", () =>
    save("record", selectedRecord),
  );
  $("save-address").addEventListener(
    "click",
    () => addressView && save("address", addressView.anchor, addressView.scope),
  );
  $("research-save-list").addEventListener(
    "change",
    () =>
      ($("research-new-name-label").hidden = Boolean(
        $("research-save-list").value,
      )),
  );
  $("research-save-close").addEventListener("click", () =>
    $("research-save").close(),
  );
  $("research-save-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!pending) return;
    let id = $("research-save-list").value;
    const name = $("research-new-name").value.trim();
    if (!id && !name) {
      $("research-save-status").textContent = "Enter a name for the new list.";
      return;
    }
    const entry = { ...pending, note: $("research-save-note").value };
    try {
      await mutate((next) => {
        if (!id) {
          id = uuid();
          next.lists.push({ id, name, entries: [] });
        }
        const list = next.lists.find((l) => l.id === id);
        if (!list)
          throw new Error("List no longer exists; reopen Save to list.");
        if (list.entries.some((e) => model.key(e) === model.key(entry)))
          throw new Error(
            "This source reference is already in that list. Open Research lists to edit its note.",
          );
        list.entries.push(entry);
      });
      selected = id;
      $("research-save").close();
      $(
        pending.mode === "address" ? "address-status" : "record-feedback",
      ).textContent = "Saved to research list.";
    } catch (error) {
      $("research-save-status").textContent =
        error.message ||
        "Unable to save locally; storage may be full or disabled.";
    }
  });
  window.addEventListener("storage", (event) => {
    if (event.key === key && $("research-dialog").open)
      status(
        "Research lists changed in another tab. Reload lists before saving; unsaved drafts are retained.",
      );
  });
  window.hasUnsavedResearch = () => drafts.size > 0;
  window.addEventListener("beforeunload", (event) => {
    if (drafts.size && !native) {
      event.preventDefault();
      event.returnValue = "";
    }
  });
})();
