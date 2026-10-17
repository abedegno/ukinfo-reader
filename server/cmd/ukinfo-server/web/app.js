"use strict";
const $ = (id) => document.getElementById(id);
const form = $("search");
let kind = "person",
  next = "",
  lastQuery = null,
  page = [],
  pageHistory = [],
  generation = 0;
let datasets = [];
async function api(path) {
  const response = await fetch(path, { credentials: "same-origin" });
  const data = await response.json();
  if (!response.ok)
    throw new Error(data.error || "The catalogue is unavailable.");
  return data;
}
function element(tag, text, className) {
  const node = document.createElement(tag);
  node.textContent = text;
  if (className) node.className = className;
  return node;
}
function reset() {
  generation++;
  page = [];
  pageHistory = [];
  next = "";
  lastQuery = null;
  $("items").replaceChildren();
  $("next").hidden = true;
  $("previous").hidden = true;
  $("export").disabled = true;
  $("submit").disabled = datasets.length === 0;
  $("status").textContent = "Enter a name or postcode to search this edition.";
}
function editionNote() {
  const selected = datasets.find((d) => d.key === form.elements.dataset.value);
  $("edition-note").textContent = selected
    ? `${selected.entries.toLocaleString()} imported entries${selected.manifest.sample ? " · PARTIAL SAMPLE, not the complete edition" : ""}. ${selected.manifest.edition.includes("2004") ? "This disc has no business listings." : ""}`
    : "";
}
function showKind(value) {
  kind = value;
  reset();
  $("people").setAttribute("aria-pressed", String(kind === "person"));
  $("business").setAttribute("aria-pressed", String(kind === "business"));
  $("person-fields").hidden = kind !== "person";
  $("business-field").hidden = kind !== "business";
}
$("people").addEventListener("click", () => showKind("person"));
$("business").addEventListener("click", () => showKind("business"));
form.elements.dataset.addEventListener("change", () => {
  reset();
  editionNote();
});
function details(item) {
  $("provenance").textContent =
    `${lastQuery.get("dataset")} · source entry ${item.source_id}. Fields below retain the decoder's original names and values. Empty and null values are retained in page exports.`;
  const fields = [];
  for (const [key, value] of Object.entries(item.record)) {
    if (value === null || value === "") continue;
    fields.push(
      element("dt", key),
      element(
        "dd",
        typeof value === "object" ? JSON.stringify(value) : String(value),
      ),
    );
  }
  $("record-fields").replaceChildren(...fields);
  $("details").showModal();
}
$("close-details").addEventListener("click", () => $("details").close());
function render() {
  $("items").replaceChildren();
  for (const item of page) {
    const r = item.record;
    const card = element("article", "");
    const text = element("div", "");
    const title =
      r.entry_type === "business"
        ? r.organisation
        : [r.firstName, r.initial2, r.surname].filter(Boolean).join(" ");
    const heading = element("h3", title || "Unnamed source entry");
    heading.append(element("span", r.entry_type || "directory", "tag"));
    text.append(heading);
    const address = [
      "subbuilding",
      "building",
      "houseno",
      "street",
      "street_1",
      "street_2",
      "locality",
      "locality_1",
      "locality_2",
      "town",
      "county",
    ]
      .map((k) => r[k])
      .filter((v) => v && v !== "0");
    text.append(
      element("p", [...new Set(address)].join(", ") || "Address not recorded"),
    );
    if (r.postcode) text.append(element("p", r.postcode, "postcode"));
    if (r.phone) text.append(element("p", `Telephone · ${r.phone}`));
    if (r.source_warning)
      text.append(
        element("p", `Source warning · ${r.source_warning}`, "warning"),
      );
    if (r.postcode_precision != null && ![0, 10].includes(r.postcode_precision))
      text.append(
        element("p", "Postcode has limited source precision", "warning"),
      );
    const button = element("button", "View record ↗");
    button.type = "button";
    button.addEventListener("click", () => details(item));
    card.append(text, button);
    $("items").append(card);
  }
  $("next").hidden = !next;
  $("previous").hidden = pageHistory.length === 0;
  $("export").disabled = page.length === 0;
  $("status").textContent = page.length
    ? `Page ${pageHistory.length + 1} · ${page.length} source entries on this page.${next ? " More results available." : " End of results."}`
    : "No matching entries. Check the edition, exact spelling or postcode. A partial sample covers only its imported entries.";
}
async function search(params, trail = []) {
  const id = ++generation;
  $("submit").disabled = true;
  $("next").hidden = true;
  $("previous").hidden = true;
  $("export").disabled = true;
  $("status").textContent = "Searching the archive…";
  try {
    const data = await api(`/api/search?${params}`);
    if (id !== generation) return;
    lastQuery = new URLSearchParams(params);
    pageHistory = trail;
    page = data.items;
    next = data.next_cursor;
    render();
  } catch (error) {
    if (id !== generation) return;
    page = [];
    $("items").replaceChildren();
    $("status").textContent = error.message;
  } finally {
    if (id === generation) $("submit").disabled = false;
  }
}
form.addEventListener("submit", (event) => {
  event.preventDefault();
  const params = new URLSearchParams({
    dataset: form.elements.dataset.value,
    kind,
  });
  for (const name of kind === "person"
    ? ["surname", "first_name", "postcode"]
    : ["organisation", "postcode"]) {
    const value = form.elements[name].value.trim();
    if (value) params.set(name, value);
  }
  search(params);
});
$("next").addEventListener("click", () => {
  const params = new URLSearchParams(lastQuery);
  params.set("cursor", next);
  search(params, [...pageHistory, new URLSearchParams(lastQuery)]);
});
$("previous").addEventListener("click", () => {
  if (pageHistory.length)
    search(
      new URLSearchParams(pageHistory[pageHistory.length - 1]),
      pageHistory.slice(0, -1),
    );
});
$("export").addEventListener("click", async () => {
  $("export").disabled = true;
  try {
    // Preserve raw source JSON bytes; browser number parsing can round large integers.
    const response = await fetch(`/api/export?${lastQuery}`);
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || "Export unavailable.");
    }
    const url = URL.createObjectURL(await response.blob());
    const link = document.createElement("a");
    link.href = url;
    link.download = "ukinfo-page.jsonl";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (error) {
    $("status").textContent = error.message;
  } finally {
    $("export").disabled = page.length === 0;
  }
});

async function refreshDatasets(preferred) {
  datasets = await api("/api/datasets");
  form.elements.dataset.replaceChildren(
    ...datasets.map((d) => {
      const option = element(
        "option",
        `${d.manifest.title}${d.manifest.sample ? " [SAMPLE]" : ""}`,
      );
      option.value = d.key;
      return option;
    }),
  );
  if (preferred && datasets.some((d) => d.key === preferred))
    form.elements.dataset.value = preferred;
  $("coverage").textContent =
    `${datasets.length} imported edition${datasets.length === 1 ? "" : "s"}`;
  $("submit").disabled = datasets.length === 0;
  if (!datasets.length)
    $("status").textContent = window.ukinfoDesktop
      ? "Add an edition or try the invented demo to begin."
      : "No editions imported yet. Follow the catalogue import guide to add your local disc data.";
  editionNote();
}
(async () => {
  try {
    await refreshDatasets();
  } catch (error) {
    $("coverage").textContent = "Connection unavailable";
    $("status").textContent = error.message;
    $("submit").disabled = true;
  }
})();

if (window.ukinfoDesktop) {
  const bridge = window.ukinfoDesktop;
  let selected = null,
    running = false;
  const phaseNames = {
    "checking source": "Checking source files…",
    loading: "Importing entries",
    "checking identities": "Checking entry identities",
    "building indexes": "Preparing search",
    publishing: "Finishing import",
    complete: "Ready to search",
    canceled: "Import canceled",
    failed: "Import failed",
  };
  const busy = (value) => {
    running = value;
    for (const id of [
      "import-choose",
      "import-demo",
      "import-scope",
      "import-start",
    ])
      $(id).disabled = value;
    $("import-cancel").disabled = !value;
  };
  const choose = async (demo) => {
    try {
      busy(true);
      $("import-message").textContent = demo
        ? "Preparing invented entries…"
        : "Checking the selected edition…";
      const source = await (demo ? bridge.demoSource() : bridge.chooseSource());
      if (!source) {
        $("import-message").textContent = "";
        return;
      }
      selected = source;
      $("import-selected").hidden = false;
      $("import-title").textContent = source.title;
      $("import-description").textContent =
        `${source.entries.toLocaleString()} entries · ${source.label}`;
      $("import-scope").value = demo ? "full" : "10000";
      $("import-scope").disabled = demo;
      $("import-message").textContent = demo
        ? "This demo contains invented people and a business. Search for Example or Aster Archive Supplies."
        : "Source fingerprints match a supported edition.";
    } catch (error) {
      $("import-message").textContent = error.message;
    } finally {
      busy(false);
      if (selected?.edition === "synthetic-demo")
        $("import-scope").disabled = true;
    }
  };
  $("import-open").addEventListener("click", () =>
    $("import-dialog").showModal(),
  );
  $("import-close").addEventListener("click", () => $("import-dialog").close());
  $("import-choose").addEventListener("click", () => choose(false));
  $("import-demo").addEventListener("click", () => choose(true));
  bridge.onProgress((state) => {
    const p = state.progress || {};
    if (!running) return;
    $("import-stage").textContent =
      `${phaseNames[p.phase] || p.phase || "Importing…"}${p.total ? ` · ${p.done.toLocaleString()} / ${p.total.toLocaleString()}` : ""}`;
    if (p.total)
      $("import-bar").value = Math.min(100, (p.done / p.total) * 100);
    else $("import-bar").removeAttribute("value");
    $("import-open").textContent = p.total
      ? `Importing ${Math.floor((p.done / p.total) * 100)}%`
      : "Importing…";
  });
  $("import-start").addEventListener("click", async () => {
    if (!selected || running) return;
    const limit =
      $("import-scope").value === "full"
        ? null
        : Number($("import-scope").value);
    const key = selected.edition + (limit ? `:sample:${limit}` : "");
    if (datasets.some((d) => d.key === key)) {
      $("import-message").textContent =
        "That edition is already imported. Choose it in the search panel.";
      return;
    }
    busy(true);
    $("import-progress").hidden = false;
    $("import-bar").removeAttribute("value");
    $("import-stage").textContent = "Checking source files…";
    $("import-message").textContent =
      "You can close this panel and keep searching imported editions.";
    try {
      await bridge.importSource(selected.selection, limit);
      await refreshDatasets(key);
      reset();
      $("import-stage").textContent = "Ready to search";
      $("import-bar").value = 100;
      $("import-message").textContent = "Edition imported successfully.";
    } catch (error) {
      // Cancellation can arrive after commit. Reconcile with the catalogue
      // rather than claiming that a completed import was rolled back.
      try {
        await refreshDatasets(key);
      } catch {
        /* Preserve the original error. */
      }
      if (datasets.some((d) => d.key === key)) {
        $("import-message").textContent =
          "The edition finished importing and is available to search.";
        $("import-stage").textContent = "Ready to search";
        $("import-bar").value = 100;
      } else {
        $("import-message").textContent = error.message;
        $("import-stage").textContent = "Import stopped";
      }
    } finally {
      busy(false);
      $("import-open").textContent = "Add edition +";
      if (selected?.edition === "synthetic-demo")
        $("import-scope").disabled = true;
    }
  });
  $("import-cancel").addEventListener("click", async () => {
    $("import-cancel").disabled = true;
    $("import-stage").textContent = "Canceling import…";
    try {
      await bridge.cancelImport();
    } catch (error) {
      $("import-message").textContent = error.message;
    }
  });
  bridge
    .capabilities()
    .then((c) => {
      $("import-open").hidden = !c.canImport;
    })
    .catch(() => {});
}
