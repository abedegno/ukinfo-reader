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
let catalogueLoaded = false;
let searchGeography = null;
let addressCoverage = new Set();
let geographyAvailable = false;
// Older bookmarks can combine a radius with an independent postcode restriction.
// Keep that restriction in the visible filter chips without a second postcode box.
let savedPostcodeConstraint = "";
let restoredSearch = false;
function updatePostcodeMode() {
  const nearby = form.elements.postcode_scope.value === "near";
  $("nearby-controls").hidden = !nearby;
  $("geography-capability").hidden = geographyAvailable && !nearby;
  $("postcode-constraint-note").hidden = !(nearby && savedPostcodeConstraint);
  $("postcode-constraint-note").textContent = savedPostcodeConstraint
    ? `This saved search also restricts results to ${savedPostcodeConstraint}. Remove the Postcode filter from the results to widen it.`
    : "";
}
for (const input of form.elements.postcode_scope)
  input.addEventListener("change", () => {
    if (form.elements.postcode_scope.value === "in")
      savedPostcodeConstraint = "";
    updatePostcodeMode();
  });
const locationFields = ["street", "locality", "town", "county"];
const addressFields = ["house", "building", "flat", ...locationFields];
const filterLabels = {
  surname: "Surname",
  match: "Name matching",
  first_name: "Given names",
  first_match: "First-name matching",
  middle_initial: "Middle initial",
  organisation: "Business",
  postcode: "Postcode",
  house: "House",
  building: "Building",
  flat: "Flat",
  street: "Street",
  locality: "Locality",
  town: "Town / city",
  county: "County",
  near: "Near",
};
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
  $("active-filters").replaceChildren();
  $("search-suggestions").replaceChildren();
  $("search-suggestions").hidden = true;
  $("next").hidden = true;
  $("previous").hidden = true;
  $("previous-top").hidden = true;
  $("next-top").hidden = true;
  $("export").disabled = true;
  $("submit").disabled = datasets.length === 0;
  $("status").textContent =
    "Enter a name, address or postcode to search this edition.";
}
function addressNote() {
  const selection = form.elements.dataset.value;
  const selected =
    selection === "all"
      ? datasets.filter(
          (d) =>
            !d.manifest.sample ||
            !datasets.some(
              (other) =>
                other.manifest.edition === d.manifest.edition &&
                !other.manifest.sample,
            ),
        )
      : datasets.filter((d) => d.key === selection);
  const missing = selected.filter((d) => !addressCoverage.has(d.key));
  const available = selected.length > 0 && missing.length === 0;
  for (const name of locationFields) form.elements[name].disabled = !available;
  $("address-capability").textContent = available
    ? "Street, locality, town and county searches are ready for the selected editions."
    : "Address search is not ready for every selected edition. Choose an indexed edition or add its address index.";
}
async function refreshAddresses() {
  try {
    const data = await api("/api/addresses");
    addressCoverage = new Set((data.datasets || []).map((d) => d.dataset));
  } catch {
    addressCoverage = new Set();
  }
  addressNote();
}
function editionNote() {
  addressNote();
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
  $("name-refinements").hidden = kind !== "person";
  $("name-help").hidden = kind !== "person";
  $("business-field").hidden = kind !== "business";
  $("who-heading").textContent =
    kind === "person" ? "Who are you looking for?" : "Which business?";
  $("match-label").textContent =
    kind === "person" ? "Surname match" : "Business name match";
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
function details(item, occurrences = [item], options = {}) {
  viewGeneration++;
  if ($("address-details").open) $("address-details").close();
  if (options.updateURL !== false && hasRecordID(item))
    setViewURL("record", item);
  selectedRecord = item;
  $("back-address").hidden = !options.fromAddress || !addressView;
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
  if (!$("details").open) $("details").showModal();
  recordActions(item, options.metadata);
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
    const response = await fetch(
      hasRecordID(selectedRecord)
        ? `/api/record?${recordParams(selectedRecord)}&format=jsonl`
        : `/api/export?${lastQuery}`,
    );
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
$("close-details").addEventListener("click", closeView);
function render() {
  $("geography-note").hidden = !searchGeography;
  if (searchGeography)
    $("geography-note").textContent =
      `Within ${(searchGeography.radius_m / 1609.344).toFixed(1)} miles of ${searchGeography.near}${searchGeography.centre_type && searchGeography.centre_type !== "postcode" ? ` (approximate ${searchGeography.centre_type} centre from ${searchGeography.centre_postcodes} postcode points${searchGeography.centre_retired_only ? "; retired postcodes only" : ""}; radius from centre, not boundary)` : ""} · ONSPD ${searchGeography.reference.release} · approximate postcode positions. Results use catalogue order, not distance order.${searchGeography.terminated ? " The centre postcode is retired." : ""}`;
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
function hasSearchAnchor(params) {
  return [
    "surname",
    "organisation",
    "postcode",
    "near",
    ...locationFields,
  ].some((name) => params.get(name)?.trim());
}
function applyRefinement(params) {
  params.delete("cursor");
  params.delete("page");
  const url = new URL(location.href);
  url.search = params.toString();
  history.pushState(null, "", url);
  restoreURL();
}
function removeFilter(params, name) {
  params.delete(name);
  if (name === "near") params.delete("radius_m");
  if (name === "first_name") params.delete("first_match");
}
function renderFilters(params) {
  const container = $("active-filters");
  container.replaceChildren();
  for (const [name, label] of Object.entries(filterLabels)) {
    const value = params.get(name);
    if (
      !value ||
      (name === "match" && value === "exact") ||
      (name === "first_match" && !params.get("first_name"))
    )
      continue;
    const display =
      name === "first_match"
        ? {
            exact: "Exact",
            prefix: "Starts with",
            given: "Given name + following names",
          }[value] || value
        : value;
    const button = element("button", `${label}: ${display} ×`);
    button.type = "button";
    button.setAttribute("aria-label", `Remove ${label.toLowerCase()} filter`);
    button.addEventListener("click", () => {
      const revised = new URLSearchParams(params);
      removeFilter(revised, name);
      applyRefinement(revised);
    });
    container.append(button);
  }
  if (container.childElementCount) {
    const clear = element("button", "Clear filters");
    clear.type = "button";
    clear.addEventListener("click", () =>
      applyRefinement(
        new URLSearchParams({
          dataset: params.get("dataset"),
          kind: params.get("kind") || "person",
          limit: params.get("limit") || "50",
        }),
      ),
    );
    container.append(clear);
  }
}
function renderSuggestions(params) {
  const container = $("search-suggestions");
  const add = (text, update) => {
    const button = element("button", text);
    button.type = "button";
    button.addEventListener("click", () => {
      const revised = new URLSearchParams(params);
      update(revised);
      applyRefinement(revised);
    });
    container.append(button);
  };
  const name = params.get("surname") || params.get("organisation") || "";
  if (
    params.get("match") !== "prefix" &&
    !name.endsWith("*") &&
    [...name.trim()].length >= 2
  )
    add("Try name prefixes", (p) => p.set("match", "prefix"));
  for (const name of [
    "middle_initial",
    "flat",
    "house",
    "building",
    "first_name",
  ])
    if (params.has(name))
      add(`Search without ${filterLabels[name].toLowerCase()}`, (p) =>
        removeFilter(p, name),
      );
  if (params.get("dataset") !== "all" && datasets.length > 1)
    add("Search all editions", (p) => p.set("dataset", "all"));
  container.hidden = container.childElementCount === 0;
}
async function search(params, trail = [], updateURL = true) {
  if (updateURL) {
    const url = new URL(location.href);
    url.search = params.toString();
    history.pushState(null, "", url);
  }
  const id = ++generation;
  searchGeography = null;
  $("geography-note").hidden = true;
  renderFilters(params);
  $("search-suggestions").replaceChildren();
  $("search-suggestions").hidden = true;
  if (!hasSearchAnchor(params)) {
    page = [];
    next = "";
    lastQuery = null;
    pageHistory = [];
    $("submit").disabled = datasets.length === 0;
    $("items").replaceChildren();
    for (const name of ["next", "previous", "previous-top", "next-top"])
      $(name).hidden = true;
    $("export").disabled = true;
    $("status").textContent =
      "Add a surname/business name, street, locality, town, county, postcode or nearby area to narrow these filters.";
    return;
  }
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
    if (!page.length) renderSuggestions(params);
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
    ? ["surname", "first_name", "first_match", "middle_initial"]
    : ["organisation"]) {
    if (name === "first_match" && !form.elements.first_name.value.trim())
      continue;
    const value = form.elements[name].value.trim();
    if (value) params.set(name, value);
  }
  for (const name of addressFields) {
    const input = form.elements[name];
    if (!input.disabled && input.value.trim())
      params.set(name, input.value.trim());
  }
  const postcode = form.elements.postcode.value.trim();
  if (postcode) {
    if (form.elements.postcode_scope.value === "near") {
      if (!geographyAvailable) {
        $("status").textContent =
          "Nearby search needs postcode locations. Use In this postcode, or add a postcode reference in Postcode setup.";
        return;
      }
      params.set("near", postcode);
      params.set("radius_m", form.elements.radius_m.value);
      if (savedPostcodeConstraint)
        params.set("postcode", savedPostcodeConstraint);
    } else params.set("postcode", postcode);
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
  catalogueLoaded = true;
  window.dispatchEvent(new Event("ukinfo-catalogue-ready"));
}
window.addEventListener("ukinfo-catalogue-changed", async () => {
  try {
    await refreshDatasets();
    await refreshAddresses();
    reset();
    for (const id of ["details", "address-details"])
      if ($(id).open) $(id).close();
    $("status").textContent = datasets.length
      ? "Catalogue updated. Start a new search."
      : "Add an edition or try the invented demo to begin.";
  } catch (error) {
    $("status").textContent = error.message;
  }
});
const viewFields = [
  "view",
  "record_dataset",
  "record_id",
  "record_source",
  "address_scope",
  "address_cursor",
];
let viewGeneration = 0;
let recordActionGeneration = 0;
let addressView = null;
function hasRecordID(item) {
  return item && Number.isInteger(item.id) && item.id > 0 && item.dataset;
}
function recordParams(item) {
  return new URLSearchParams({
    dataset: item.dataset,
    id: String(item.id),
    source_id: item.source_id,
  });
}
function viewURL(
  mode,
  item,
  { scope = "edition", cursor = "", standalone = false } = {},
) {
  const url = new URL(
    standalone ? location.origin + location.pathname : location.href,
  );
  for (const field of viewFields) url.searchParams.delete(field);
  url.searchParams.set("view", mode);
  url.searchParams.set("record_dataset", item.dataset);
  url.searchParams.set("record_id", String(item.id));
  url.searchParams.set("record_source", item.source_id);
  if (mode === "address") {
    url.searchParams.set("address_scope", scope);
    if (cursor) url.searchParams.set("address_cursor", cursor);
  }
  return url;
}
function setViewURL(mode, item, options = {}) {
  history.pushState(null, "", viewURL(mode, item, options));
}
function closeDialogs() {
  viewGeneration++;
  recordActionGeneration++;
  for (const id of ["details", "address-details"])
    if ($(id).open) $(id).close();
}
function closeView() {
  closeDialogs();
  const url = new URL(location.href);
  for (const name of viewFields) url.searchParams.delete(name);
  history.pushState(null, "", url);
}
async function recordActions(item, metadata) {
  const generation = ++recordActionGeneration;
  $("copy-record-link").disabled = !hasRecordID(item);
  $("save-record").disabled = !hasRecordID(item) || !item.source_id;
  $("explore-address").disabled = true;
  $("record-address-note").textContent = "";
  if (!hasRecordID(item)) return;
  try {
    const data = metadata || (await api(`/api/record?${recordParams(item)}`));
    if (generation !== recordActionGeneration || !$("details").open) return;
    $("explore-address").disabled = !data.address_available;
    $("record-address-note").textContent = data.address_available
      ? "Explore co-listed people and businesses at this recorded address, within this edition or across available editions."
      : data.address_reason;
  } catch (error) {
    if (generation === recordActionGeneration)
      $("record-address-note").textContent = error.message;
  }
}
$("back-address").addEventListener("click", () =>
  openAddress(addressView.anchor, {
    scope: addressView.scope,
    cursor: addressView.cursor,
    trail: addressView.trail,
  }),
);
$("saved-link-open").addEventListener("click", () => {
  $("saved-link-status").textContent = "";
  $("saved-link-dialog").showModal();
});
$("saved-link-close").addEventListener("click", () =>
  $("saved-link-dialog").close(),
);
$("saved-link-form").addEventListener("submit", (event) => {
  event.preventDefault();
  try {
    const params = new URL($("saved-link").value.trim()).searchParams;
    const mode = params.get("view");
    const ref = {
      dataset: params.get("record_dataset"),
      id: Number(params.get("record_id")),
      source_id: params.get("record_source") || "",
    };
    if (!["record", "address"].includes(mode) || !hasRecordID(ref))
      throw new Error(
        "Paste a record or address link created by this application.",
      );
    const url = viewURL(mode, ref, {
      scope: params.get("address_scope") || "edition",
      cursor: params.get("address_cursor") || "",
    });
    $("saved-link-dialog").close();
    history.pushState(null, "", url);
    restoreURL();
  } catch (error) {
    $("saved-link-status").textContent =
      "Paste a record or address link created by this application.";
  }
});
$("copy-record-link").addEventListener("click", () =>
  copyText(viewURL("record", selectedRecord, { standalone: true }).href),
);
$("explore-address").addEventListener("click", () =>
  openAddress(selectedRecord),
);
for (const id of ["details", "address-details"])
  $(id).addEventListener("cancel", (event) => {
    event.preventDefault();
    closeView();
  });
$("close-address").addEventListener("click", closeView);
async function restoreView(params) {
  const mode = params.get("view");
  if (!["record", "address"].includes(mode)) return;
  const generation = ++viewGeneration;
  const ref = {
    dataset: params.get("record_dataset"),
    id: Number(params.get("record_id")),
    source_id: params.get("record_source") || "",
  };
  if (!hasRecordID(ref)) {
    $("status").textContent = "This record link is incomplete.";
    return;
  }
  try {
    const data = await api(`/api/record?${recordParams(ref)}`);
    if (generation !== viewGeneration) return;
    if (mode === "record")
      details(data.item, [data.item], { updateURL: false, metadata: data });
    else
      openAddress(data.item, {
        scope: params.get("address_scope") || "edition",
        cursor: params.get("address_cursor") || "",
        updateURL: false,
      });
  } catch (error) {
    if (generation === viewGeneration) $("status").textContent = error.message;
  }
}
async function openAddress(
  anchor,
  { scope = "edition", cursor = "", trail = [], updateURL = true } = {},
) {
  if (!hasRecordID(anchor)) return;
  const generation = ++viewGeneration;
  recordActionGeneration++;
  if ($("details").open) $("details").close();
  addressView = { anchor, scope, cursor, trail, next: "", items: [] };
  if (updateURL) setViewURL("address", anchor, { scope, cursor });
  $("address-heading").textContent = addressText(anchor.record);
  $("address-scope").value = scope;
  $("address-status").textContent =
    "Finding listings at this recorded address…";
  $("address-items").replaceChildren();
  for (const id of ["address-first", "address-previous", "address-next"])
    $(id).hidden = true;
  $("export-address-page").disabled = true;
  if (!$("address-details").open) $("address-details").showModal();
  $("address-details").scrollTop = 0;
  try {
    const params = recordParams(anchor);
    params.set("scope", scope);
    params.set("limit", "50");
    if (cursor) params.set("cursor", cursor);
    const data = await api(`/api/address?${params}`);
    if (generation !== viewGeneration) return;
    addressView.next = data.next_cursor;
    addressView.items = data.items;
    $("premises-notice").textContent = data.notice;
    let edition = "";
    for (const item of data.items) {
      if (edition !== item.dataset) {
        edition = item.dataset;
        $("address-items").append(element("h3", editionTitle(item)));
      }
      const card = element("article", "");
      card.append(
        element("h3", recordName(item.record) || "Unnamed listing"),
        element("p", listingType(item.record)),
        element("p", citation(item), "note"),
      );
      if (
        item.dataset === anchor.dataset &&
        item.source_id === anchor.source_id
      )
        card.append(element("p", "Selected source record", "badge"));
      const button = element("button", "View source record");
      button.type = "button";
      button.addEventListener("click", () =>
        details(item, [item], { fromAddress: true }),
      );
      card.append(button);
      $("address-items").append(card);
    }
    $("address-status").textContent = data.items.length
      ? `${data.items.length} source listing${data.items.length === 1 ? "" : "s"} on this page. ${data.next_cursor ? "More listings available." : "End of listings."}`
      : "No matching source listings. Recorded address variations may be missed.";
    $("address-first").hidden = !cursor;
    $("address-previous").hidden = trail.length === 0;
    $("address-next").hidden = !data.next_cursor;
    $("export-address-page").disabled = data.items.length === 0;
  } catch (error) {
    if (generation === viewGeneration) {
      $("address-status").textContent = error.message;
      $("address-first").hidden = !cursor;
    }
  }
}
$("address-scope").addEventListener("change", () =>
  openAddress(addressView.anchor, { scope: $("address-scope").value }),
);
$("address-next").addEventListener("click", () =>
  openAddress(addressView.anchor, {
    scope: addressView.scope,
    cursor: addressView.next,
    trail: [...addressView.trail, addressView.cursor],
  }),
);
$("address-previous").addEventListener("click", () =>
  openAddress(addressView.anchor, {
    scope: addressView.scope,
    cursor: addressView.trail.at(-1),
    trail: addressView.trail.slice(0, -1),
  }),
);
$("address-first").addEventListener("click", () =>
  openAddress(addressView.anchor, { scope: addressView.scope }),
);
$("copy-address-link").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(
      viewURL("address", addressView.anchor, {
        scope: addressView.scope,
        cursor: addressView.cursor,
        standalone: true,
      }).href,
    );
    $("address-status").textContent = "Address link copied.";
  } catch {
    $("address-status").textContent =
      "Copy unavailable. Copy the page URL from your browser.";
  }
});
$("export-address-page").addEventListener("click", async () => {
  const params = recordParams(addressView.anchor);
  params.set("scope", addressView.scope);
  params.set("format", "jsonl");
  if (addressView.cursor) params.set("cursor", addressView.cursor);
  try {
    const response = await fetch(`/api/address?${params}`);
    if (!response.ok)
      throw new Error((await response.json()).error || "Export unavailable");
    downloadBlob(await response.blob(), "ukinfo-address-page.jsonl");
  } catch (error) {
    $("address-status").textContent = error.message;
  }
});

async function refreshGeography() {
  const geo = await api("/api/geography");
  geographyAvailable = geo.available === true;
  $("postcode-near").disabled = !geographyAvailable;
  $("radius").disabled = !geo.available;
  $("geography-capability").textContent = geo.available
    ? `ONSPD ${geo.reference.release} · Great Britain · current and retired postcodes`
    : "Nearby search needs postcode locations. Postcode matching is still available.";
  updatePostcodeMode();
  if (geo.available) {
    $("geography-attribution").hidden = false;
    const year = geo.reference.release.slice(0, 4);
    $("geography-attribution").textContent =
      `Contains OS data © Crown copyright and database right ${year}. Contains Royal Mail data © Royal Mail copyright and database right ${year}. Source: Office for National Statistics licensed under the Open Government Licence v3.0.`;
  }
  return geo;
}
window.addEventListener("ukinfo-geography-changed", () =>
  refreshGeography().catch(() => {}),
);

(async () => {
  try {
    await refreshDatasets();
    await refreshAddresses();
    try {
      await refreshGeography();
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
  const viewParams = new URLSearchParams(location.search);
  const params = new URLSearchParams(viewParams);
  for (const name of viewFields) params.delete(name);
  closeDialogs();
  restoreView(viewParams);
  // Initial capability requests may finish after someone has started typing.
  // A blank initial URL must not erase their draft; history navigation still resets.
  if (!params.size && !restoredSearch) {
    restoredSearch = true;
    updatePostcodeMode();
    editionNote();
    return;
  }
  restoredSearch = true;
  form.reset();
  savedPostcodeConstraint = params.has("near")
    ? params.get("postcode") || ""
    : "";
  form.elements.postcode_scope.value = params.has("near") ? "near" : "in";
  form.elements.postcode.value =
    params.get("near") || params.get("postcode") || "";
  updatePostcodeMode();
  $("name-options").open =
    params.get("match") === "prefix" ||
    params.has("middle_initial") ||
    (params.has("first_name") && params.get("first_match") !== "given");
  $("address-controls").open = [
    "house",
    "building",
    "flat",
    "locality",
    "county",
  ].some((name) => params.has(name));
  const chosen = params.get("dataset");
  if (
    chosen &&
    [...form.elements.dataset.options].some((option) => option.value === chosen)
  )
    form.elements.dataset.value = chosen;
  if (
    !params.has("surname") &&
    !params.has("first_name") &&
    !params.has("middle_initial") &&
    !params.has("organisation") &&
    !params.has("postcode") &&
    !params.has("near") &&
    !addressFields.some((name) => params.has(name))
  ) {
    if (params.size)
      showKind(params.get("kind") === "business" ? "business" : "person");
    else reset();
    editionNote();
    return;
  }

  showKind(params.get("kind") === "business" ? "business" : "person");
  for (const name of [
    "dataset",
    "surname",
    "first_name",
    "first_match",
    "middle_initial",
    "organisation",
    "match",
    "radius_m",
    ...addressFields,
  ])
    if (params.has(name)) form.elements[name].value = params.get(name);
  if (params.has("first_name") && !params.has("first_match"))
    form.elements.first_match.value =
      params.get("match") === "prefix" ? "prefix" : "exact";
  $("page-size").value = params.get("limit") || "50";
  editionNote();
  search(params, [], false);
}
window.addEventListener("popstate", restoreURL);

if (window.ukinfoDesktop) {
  const bridge = window.ukinfoDesktop;
  let setupDismissed = false,
    canImport = false;
  function importStep(step) {
    [...$("import-steps").children].forEach((node, index) => {
      node.classList.toggle("done", index < step - 1);
      if (index === step - 1) node.setAttribute("aria-current", "step");
      else node.removeAttribute("aria-current");
    });
  }
  function welcome() {
    if (catalogueLoaded && datasets.length) setupDismissed = true;
    if (!canImport || !catalogueLoaded || datasets.length || setupDismissed)
      return;
    $("import-heading").textContent = "Set up your archive";
    $("import-welcome").hidden = false;
    if (!$("import-dialog").open) $("import-dialog").showModal();
  }
  window.addEventListener("ukinfo-catalogue-ready", welcome);
  $("import-dialog").addEventListener("close", () => {
    setupDismissed = true;
  });
  $("import-search").addEventListener("click", () => {
    $("import-dialog").close();
    if (selected?.edition === "synthetic-demo")
      form.elements.surname.value = "Example";
    form.elements.surname.focus();
  });
  let selected = null,
    running = false,
    desiredEdition = null;
  const phaseNames = {
    "checking source": "Checking source files…",
    "reading disc directory": "Finding the supported database…",
    "extracting disc files": "Preparing temporary disc files…",
    "verifying disc files": "Verifying source fingerprints…",
    loading: "Importing entries",
    "checking identities": "Checking entry identities",
    "building indexes": "Preparing search",
    "indexing addresses": "Indexing addresses…",
    "grouping address postings": "Preparing address search…",
    publishing: "Finishing import",
    complete: "Ready to search",
    canceled: "Import canceled",
    failed: "Import failed",
  };
  const busy = (value) => {
    running = value;
    for (const id of [
      "import-choose",
      "import-media",
      "import-resume",
      "import-demo",
      "import-scope",
      "import-start",
    ])
      $(id).disabled = value;
    $("import-cancel").disabled = !value;
  };
  const choose = async (demo, media = false, resume = false) => {
    try {
      busy(true);
      $("import-progress").hidden = !media;
      $("import-bar").removeAttribute("value");
      $("import-stage").textContent = "Preparing disc files…";
      $("import-message").textContent = demo
        ? "Preparing invented entries…"
        : "Checking the selected edition…";
      const source = await (resume
        ? bridge.resumeSetup()
        : demo
          ? bridge.demoSource()
          : media
            ? bridge.chooseMedia()
            : bridge.chooseSource());
      if (!source) {
        $("import-message").textContent = "";
        return;
      }
      if (desiredEdition && source.edition !== desiredEdition.edition)
        throw new Error(
          `Choose the source for ${desiredEdition.title}. This disc is a different edition.`,
        );
      selected = source;
      importStep(2);
      $("import-search").hidden = true;
      $("import-selected").hidden = false;
      $("import-title").textContent = source.title;
      $("import-description").textContent =
        `${source.entries.toLocaleString()} entries · ${source.label}`;
      $("import-scope").value =
        source.edition === "synthetic-demo" ||
        desiredEdition?.full ||
        (resume && source.limit === null)
          ? "full"
          : "10000";
      $("import-scope").disabled = demo;
      $("import-message").textContent = demo
        ? "This demo contains invented people and a business. Search for Example or Aster Archive Supplies."
        : "Source fingerprints match a supported edition.";
    } catch (error) {
      selected = null;
      $("import-selected").hidden = true;
      $("import-message").textContent = error.message;
    } finally {
      busy(false);
      $("import-open").textContent = "Add edition +";
      $("import-progress").hidden = true;
      if (selected?.edition === "synthetic-demo")
        $("import-scope").disabled = true;
    }
  };
  window.addEventListener("ukinfo-import-request", (event) => {
    if (running) {
      $("import-dialog").showModal();
      return;
    }
    desiredEdition = event.detail?.full ? event.detail : null;
    $("import-heading").textContent = desiredEdition
      ? "Add the complete edition"
      : "Add an edition";
    $("import-message").textContent = desiredEdition
      ? `Choose ${desiredEdition.title}. Your sample stays available until you choose to remove it.`
      : "";
    $("import-dialog").showModal();
    if (event.detail?.resume || event.detail?.previous)
      choose(false, true, true);
  });
  $("import-open").addEventListener("click", () => {
    if (!running) desiredEdition = null;
    $("import-dialog").showModal();
  });
  $("import-close").addEventListener("click", () => $("import-dialog").close());
  $("import-choose").addEventListener("click", () => choose(false));
  $("import-resume").addEventListener("click", () => choose(false, true, true));
  $("import-media").addEventListener("click", () => choose(false, true));
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
    importStep(3);
    $("import-search").hidden = true;
    $("import-progress").hidden = false;
    $("import-bar").removeAttribute("value");
    $("import-stage").textContent = "Checking source files…";
    $("import-message").textContent =
      "You can close this panel and keep searching imported editions.";
    try {
      await bridge.importSource(selected.selection, limit);
      await refreshDatasets(key);
      await refreshAddresses();
      reset();
      $("import-stage").textContent = "Ready to search";
      $("import-bar").value = 100;
      $("import-message").textContent = "Edition imported successfully.";
      importStep(4);
      $("import-search").hidden = false;
    } catch (error) {
      // Cancellation can arrive after commit. Reconcile with the catalogue
      // rather than claiming that a completed import was rolled back.
      try {
        await refreshDatasets(key);
        await refreshAddresses();
      } catch {
        /* Preserve the original error. */
      }
      if (datasets.some((d) => d.key === key)) {
        $("import-message").textContent =
          "The edition finished importing and is available to search.";
        $("import-stage").textContent = "Ready to search";
        importStep(4);
        $("import-search").hidden = false;
        $("import-bar").value = 100;
      } else {
        $("import-message").textContent = error.message;
        $("import-stage").textContent = "Import stopped";
        importStep(2);
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
      canImport = c.canImport;
      welcome();
      if (bridge.catalogue)
        bridge
          .catalogue()
          .then((report) => {
            const previous = report.setup;
            const key = previous
              ? previous.edition +
                (previous.limit ? `:sample:${previous.limit}` : "")
              : "";
            $("import-resume").hidden =
              !previous ||
              previous.status === "complete" ||
              report.datasets.some((d) => d.key === key);
          })
          .catch(() => {});
    })
    .catch(() => {});
}
