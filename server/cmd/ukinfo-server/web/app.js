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
let searchGeography = null;
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
  searchGeography = null;
  $("geography-note").hidden = true;
  $("items").replaceChildren();
  $("next").hidden = true;
  $("previous").hidden = true;
  $("previous-top").hidden = true;
  $("next-top").hidden = true;
  $("export").disabled = true;
  $("submit").disabled = datasets.length === 0;
  $("status").textContent = "Enter a name or postcode to search this edition.";
}
function editionNote() {
  if (form.elements.dataset.value === "all") {
    $("edition-note").textContent =
      "Search all imported editions. Samples are excluded when their complete edition is available. " +
      (datasets.some((d) => d.manifest.sample)
        ? "Some imports are PARTIAL SAMPLES."
        : "");
    return;
  }
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
function datasetFor(item) {
  return datasets.find(
    (d) => d.key === (item.dataset || lastQuery?.get("dataset")),
  );
}
function editionTitle(item) {
  const dataset = datasetFor(item);
  return (
    (dataset?.manifest.title || item.dataset || "Unknown edition") +
    (dataset?.manifest.sample ? " · partial sample" : "")
  );
}
function listingType(r) {
  return (
    {
      electoral: "Electoral listing",
      telephone: "Telephone listing",
      business: "Business",
      directory: "Directory listing",
    }[r.entry_type] || "Directory listing"
  );
}
function recordName(r) {
  return r.entry_type === "business"
    ? r.organisation
    : [r.firstName, r.initial2, r.surname].filter(Boolean).join(" ");
}
function addressParts(r) {
  const street = [r.houseno, r.street || r.street_1]
    .filter((v) => v && v !== "0")
    .join(" ");
  return [
    ...new Set(
      [
        r.subbuilding,
        r.building,
        street,
        r.street_2,
        r.locality || r.locality_1,
        r.locality_2,
        r.town,
        r.county,
      ].filter((v) => v && v !== "0"),
    ),
  ];
}
function addressText(r) {
  return [...addressParts(r), r.postcode].filter(Boolean).join("\n");
}
const warningNames = {
  synthetic_demo_not_a_real_person:
    "Invented demonstration record; this is not a real person.",
  partial_postcode_source:
    "Only part of the postcode was recorded; the address may also be incomplete.",
  building_reference_out_of_range:
    "The source building reference could not be resolved. The building name is unavailable.",
  building_reference_inside_string:
    "The source building reference is ambiguous. The building name is unavailable.",
  ambiguous_industry_reference:
    "The source industry description has multiple possible matches and is not displayed.",
  invalid_industry_reference:
    "The source industry reference could not be resolved.",
  industry_reference_wrap_recovered:
    "The industry description was recovered from the source's shortened reference.",
  invalid_dictionary_text: "Some source text contains undecodable characters.",
  unknown_telephone_name:
    "The telephone listing does not contain a resolvable first name.",
  invalid_surname_reference:
    "The source surname reference could not be resolved.",
  nonstandard_postcode: "The source postcode has a nonstandard format.",
  place_navigation_mismatch:
    "The source place references disagree; inspect the recorded address carefully.",
  unterminated_place_text: "The source place name may be incomplete.",
};
function warnings(r) {
  const notes = (r.source_warning || "")
    .split(";")
    .filter(Boolean)
    .map(
      (code) =>
        warningNames[code] ||
        "The source contains an unresolved data issue. See original source fields for the recorded warning.",
    );
  if (r.postcode_precision != null && ![0, 10].includes(r.postcode_precision))
    notes.push(
      "The source postcode has limited precision; a full postcode cannot be confirmed.",
    );
  return [...new Set(notes)];
}
let selectedRecord = null;
function citation(item) {
  return `${recordName(item.record) || "Unnamed listing"}. ${editionTitle(item)}. Source entry ${item.source_id}. UK-Info Archive. Cover-disc date; underlying listing date unverified.`;
}
function locationQuality(value) {
  return (
    {
      1: "Representative address near the postcode mean",
      2: "Representative position determined visually",
      3: "Approximate position, within 50 metres",
      4: "Mean position of the postcode's addresses",
      5: "Estimated from surrounding postcodes",
      8: "Last known position of a retired postcode",
    }[value] || "Coordinate quality not recorded"
  );
}
function distanceText(value) {
  return `Approximately ${(value / 1609.344).toFixed(2)} miles`;
}
function details(item, occurrences = [item]) {
  selectedRecord = item;
  $("record-title").textContent = recordName(item.record) || "Record details";
  $("provenance").textContent =
    editionTitle(item) +
    " · Cover-disc edition; underlying listing date unverified.";
  $("record-feedback").textContent = "";
  const fields = [];
  const r = item.record;
  function section(title, pairs) {
    const content = pairs.filter(
      ([, v]) => v !== null && v !== undefined && v !== "",
    );
    if (!content.length) return;
    fields.push(element("h3", title));
    const dl = element("dl", "");
    for (const [label, value] of content)
      dl.append(element("dt", label), element("dd", String(value)));
    fields.push(dl);
  }
  section(r.entry_type === "business" ? "Business" : "Person", [
    ["Name", recordName(r) || "Not recorded"],
    ["Listing type", listingType(r)],
    ["Telephone", r.phone],
    ["Fax", r.fax],
    ["Industry", r.industry],
    ["Industry classification (SIC)", r.sic_code],
  ]);
  section("Address", [
    ["Recorded address", addressText(r) || "Not recorded"],
    [
      "Place type",
      {
        T: "Town",
        TC: "Town with county",
        C: "County",
        V: "Locality",
        v: "Locality",
        VC: "Locality",
      }[r.place_type],
    ],
  ]);
  if (item.distance_m != null && searchGeography)
    section("Location estimate", [
      ["Distance from centre", distanceText(item.distance_m)],
      ["Coordinate source", `ONSPD ${searchGeography.reference.release}`],
      ["Position quality", locationQuality(item.location_quality)],
      ["Postcode retired", item.location_terminated || ""],
    ]);
  fields.push(element("h3", "Sources"));
  for (const occurrence of occurrences) {
    fields.push(element("p", citation(occurrence), "note"));
    if (occurrences.length > 1) {
      const button = element(
        "button",
        `View source: ${editionTitle(occurrence)}`,
      );
      button.type = "button";
      button.addEventListener("click", () => details(occurrence));
      fields.push(button);
    }
  }
  if (occurrences.length > 1) {
    const comparison = element("details", "");
    comparison.append(element("summary", "Compare source fields"));
    const wrap = element("div", "", "comparison");
    const table = element("table", "");
    const head = element("tr", "");
    head.append(element("th", "Field"));
    for (const source of occurrences)
      head.append(element("th", editionTitle(source)));
    table.append(head);
    for (const [label, value] of [
      ["Name", recordName],
      ["Address", addressText],
      ["Telephone", (r) => r.phone],
      ["Industry", (r) => r.industry],
      ["Listing type", listingType],
    ]) {
      const row = element("tr", "");
      row.append(element("th", label));
      for (const source of occurrences)
        row.append(element("td", value(source.record) || "Not recorded"));
      table.append(row);
    }
    wrap.append(table);
    comparison.append(wrap);
    fields.push(comparison);
  }
  const notes = warnings(r);
  if (notes.length) {
    fields.push(element("h3", "Data quality"));
    for (const note of notes) fields.push(element("p", note, "warning"));
  }
  const raw = element("details", "");
  raw.append(element("summary", "Show original source fields"));
  const dl = element("dl", "");
  dl.append(element("dt", "Source entry"), element("dd", item.source_id));
  for (const [key, value] of Object.entries(r))
    dl.append(
      element("dt", key),
      element(
        "dd",
        value === null
          ? "Not recorded (null)"
          : value === ""
            ? "Not recorded (empty)"
            : typeof value === "object"
              ? JSON.stringify(value)
              : String(value),
      ),
    );
  raw.append(dl);
  fields.push(raw);
  $("record-fields").replaceChildren(...fields);
  $("details").showModal();
}
async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    $("record-feedback").textContent = "Copied.";
  } catch {
    $("record-feedback").textContent =
      "Copy unavailable. Select and copy the text in this record.";
  }
}
$("copy-address").addEventListener("click", () =>
  copyText(addressText(selectedRecord.record)),
);
$("copy-citation").addEventListener("click", () =>
  copyText(citation(selectedRecord)),
);
function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = element("a", "");
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
$("export-record").addEventListener("click", async () => {
  // Fetch original JSON bytes: browser JSON parsing can round source integers.
  try {
    const response = await fetch(`/api/export?${lastQuery}`);
    if (!response.ok) throw new Error("Export unavailable");
    const lines = (await response.text()).trim().split("\n");
    const line = lines.find((line) => {
      const v = JSON.parse(line);
      return (
        v.source_id === selectedRecord.source_id &&
        v.dataset === selectedRecord.dataset
      );
    });
    if (!line) throw new Error("Record no longer available in this page");
    downloadBlob(
      new Blob([line + "\n"], { type: "application/x-ndjson" }),
      "ukinfo-record.jsonl",
    );
  } catch (e) {
    $("record-feedback").textContent = e.message;
  }
});
$("close-details").addEventListener("click", () => $("details").close());
function render() {
  $("geography-note").hidden = !searchGeography;
  if (searchGeography)
    $("geography-note").textContent =
      `Within ${(searchGeography.radius_m / 1609.344).toFixed(1)} miles of ${searchGeography.near} · ONSPD ${searchGeography.reference.release} · approximate postcode positions. Results use catalogue order, not distance order.${searchGeography.terminated ? " The centre postcode is retired." : ""}`;
  $("items").replaceChildren();
  const groups = new Map();
  for (const item of page) {
    const r = item.record;
    const key =
      $("group-results").checked &&
      recordName(r) &&
      (r.entry_type === "business" || (r.firstName && r.surname)) &&
      (r.street || r.street_1) &&
      ((r.houseno && r.houseno !== "0") || r.building) &&
      /^[A-Z]{1,2}\d[A-Z\d]?\s?\d[A-Z]{2}$/i.test(r.postcode || "") &&
      !r.source_warning &&
      (r.postcode_precision == null || [0, 10].includes(r.postcode_precision))
        ? JSON.stringify(
            [
              r.entry_type || "directory",
              recordName(r),
              addressText(r),
              r.phone || "",
            ].map((v) =>
              v.normalize("NFKC").toLowerCase().replace(/\s+/g, " ").trim(),
            ),
          )
        : `${item.dataset}:${item.source_id}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(item);
  }
  $("group-note").hidden = !$("group-results").checked;
  for (const occurrences of groups.values()) {
    const item = occurrences[0];
    const r = item.record;
    const card = element("article", "");
    const text = element("div", "");
    const title =
      r.entry_type === "business"
        ? r.organisation
        : [r.firstName, r.initial2, r.surname].filter(Boolean).join(" ");
    const heading = element("h3", title || "Unnamed source entry");
    heading.append(element("span", listingType(r), "tag"));
    text.append(heading);
    const address = addressParts(r);
    text.append(element("p", address.join(", ") || "Address not recorded"));
    const sources = [...new Set(occurrences.map(editionTitle))];
    text.append(element("p", sources.join(" · "), "source-badges"));
    if (occurrences.length > 1)
      text.append(
        element(
          "p",
          `${occurrences.length} identical listings on this page`,
          "note",
        ),
      );
    if (r.postcode) text.append(element("p", r.postcode, "postcode"));
    if (r.phone) text.append(element("p", `Telephone · ${r.phone}`));
    if (item.distance_m != null)
      text.append(
        element(
          "p",
          distanceText(item.distance_m) + " from the centre postcode",
          "distance",
        ),
      );
    for (const note of warnings(r)) text.append(element("p", note, "warning"));
    const button = element("button", "View record ↗");
    button.type = "button";
    button.addEventListener("click", () => details(item, occurrences));
    card.append(text, button);
    $("items").append(card);
  }
  $("next").hidden = !next;
  $("previous").hidden = pageHistory.length === 0;
  $("previous-top").hidden = $("previous").hidden;
  $("next-top").hidden = $("next").hidden;
  $("export").disabled = page.length === 0;
  $("status").textContent = page.length
    ? `Page ${Number(lastQuery.get("page") || 1)} · Results ${(Number(lastQuery.get("page") || 1) - 1) * Number(lastQuery.get("limit") || 50) + 1}–${(Number(lastQuery.get("page") || 1) - 1) * Number(lastQuery.get("limit") || 50) + page.length} · ${page.length} source entries on this page.${next ? " More results available." : " End of results."}`
    : "No matching entries. Try Starts with, a postcode district, or All editions. A partial sample covers only its imported entries.";
}
async function search(params, trail = [], updateURL = true) {
  if (updateURL) {
    const url = new URL(location.href);
    url.search = params.toString();
    history.pushState(null, "", url);
  }
  const id = ++generation;
  $("submit").disabled = true;
  $("next").hidden = true;
  $("previous").hidden = true;
  $("previous-top").hidden = true;
  $("next-top").hidden = true;
  $("export").disabled = true;
  $("status").textContent = "Searching the archive…";
  try {
    const data = await api(`/api/search?${params}`);
    if (id !== generation) return;
    lastQuery = new URLSearchParams(params);
    pageHistory = trail;
    page = data.items;
    searchGeography = data.geography || null;
    next = data.next_cursor;
    render();
    if (updateURL) $("results-title").scrollIntoView({ block: "start" });
  } catch (error) {
    if (id !== generation) return;
    page = [];
    next = "";
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
    match: form.elements.match.value,
    limit: $("page-size").value,
  });
  for (const name of kind === "person"
    ? ["surname", "first_name", "postcode"]
    : ["organisation", "postcode"]) {
    const value = form.elements[name].value.trim();
    if (value) params.set(name, value);
  }
  if (form.elements.near.value.trim()) {
    params.set("near", form.elements.near.value.trim());
    params.set("radius_m", form.elements.radius_m.value);
  }
  search(params);
});
$("next").addEventListener("click", () => {
  const params = new URLSearchParams(lastQuery);
  params.set("cursor", next);
  params.set("page", String(Number(lastQuery.get("page") || 1) + 1));
  search(params, [...pageHistory, new URLSearchParams(lastQuery)]);
});
$("previous").addEventListener("click", () => {
  if (pageHistory.length)
    search(
      new URLSearchParams(pageHistory[pageHistory.length - 1]),
      pageHistory.slice(0, -1),
    );
});
$("previous-top").addEventListener("click", () => $("previous").click());
$("next-top").addEventListener("click", () => $("next").click());
$("export").addEventListener("click", async () => {
  $("export").disabled = true;
  try {
    // Preserve raw source JSON bytes; browser number parsing can round large integers.
    const response = await fetch(`/api/export?${lastQuery}`);
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || "Export unavailable.");
    }
    if ($("export-format").value === "csv") {
      const rows = (await response.text())
        .trim()
        .split("\n")
        .filter(Boolean)
        .map(JSON.parse);
      const cell = (value) => {
        let text = String(value ?? "");
        if (/^(?:\s*[=+@\-]|[\t\r\n])/.test(text)) text = "'" + text;
        return '"' + text.replaceAll('"', '""') + '"';
      };
      const values = [
        [
          "Name",
          "Listing type",
          "Address",
          "Postcode",
          "Telephone",
          "Edition",
          "Source entry",
          "Citation",
          "Approximate distance (miles)",
          "Coordinate source",
          "Position quality",
        ],
        ...rows.map((item) => [
          recordName(item.record),
          listingType(item.record),
          addressParts(item.record).join(", "),
          item.record.postcode,
          item.record.phone,
          editionTitle(item),
          item.source_id,
          citation(item),
          item.distance_m == null
            ? ""
            : (item.distance_m / 1609.344).toFixed(3),
          item.geography ? `ONSPD ${item.geography.reference.release}` : "",
          item.location_quality ? locationQuality(item.location_quality) : "",
        ]),
      ];
      downloadBlob(
        new Blob(
          [
            "\ufeff" +
              values.map((row) => row.map(cell).join(",")).join("\r\n") +
              "\r\n",
          ],
          { type: "text/csv;charset=utf-8" },
        ),
        "ukinfo-page.csv",
      );
      return;
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
    Object.assign(element("option", "All editions"), { value: "all" }),
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
    `${new Set(datasets.map((d) => d.manifest.edition)).size} editions · ${datasets.filter((d) => d.manifest.sample).length} samples`;
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
    try {
      const geo = await api("/api/geography");
      $("near").disabled = !geo.available;
      $("radius").disabled = !geo.available;
      $("geography-capability").textContent = geo.available
        ? `ONSPD ${geo.reference.release} · Great Britain · current and retired postcodes`
        : "Postcode locations have not been added to this catalogue yet.";
      if (geo.available) {
        $("geography-attribution").hidden = false;
        const year = geo.reference.release.slice(0, 4);
        $("geography-attribution").textContent =
          `Contains OS data © Crown copyright and database right ${year}. Contains Royal Mail data © Royal Mail copyright and database right ${year}. Source: Office for National Statistics licensed under the Open Government Licence v3.0.`;
      }
    } catch {
      $("geography-capability").textContent =
        "Postcode locations are currently unavailable.";
    }
    restoreURL();
  } catch (error) {
    $("coverage").textContent = "Connection unavailable";
    $("status").textContent = error.message;
    $("submit").disabled = true;
  }
})();

$("group-results").addEventListener("change", () => {
  if (lastQuery) render();
});
$("page-size").addEventListener("change", () => {
  if (lastQuery) form.requestSubmit();
});
function restoreURL() {
  const params = new URLSearchParams(location.search);
  if (
    !params.has("surname") &&
    !params.has("organisation") &&
    !params.has("postcode") &&
    !params.has("near")
  ) {
    reset();
    return;
  }
  form.reset();
  $("nearby-controls").open = params.has("near");
  showKind(params.get("kind") === "business" ? "business" : "person");
  for (const name of [
    "dataset",
    "surname",
    "first_name",
    "organisation",
    "postcode",
    "match",
    "near",
    "radius_m",
  ])
    if (params.has(name)) form.elements[name].value = params.get(name);
  $("page-size").value = params.get("limit") || "50";
  editionNote();
  search(params, [], false);
}
window.addEventListener("popstate", restoreURL);

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
