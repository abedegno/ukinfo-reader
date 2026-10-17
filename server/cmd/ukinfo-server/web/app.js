"use strict";
const $ = (id) => document.getElementById(id);
const form = $("search");
let kind = "person",
  next = "",
  lastQuery = null,
  page = [],
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
  next = "";
  lastQuery = null;
  $("items").replaceChildren();
  $("next").hidden = true;
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
  $("export").disabled = page.length === 0;
  $("status").textContent = page.length
    ? `${page.length} source entries on this page.${next ? " More results available." : " End of results."}`
    : "No matching entries. Check the edition, exact spelling or postcode. A partial sample covers only its imported entries.";
}
async function search(params) {
  const id = ++generation;
  $("submit").disabled = true;
  $("next").hidden = true;
  $("export").disabled = true;
  $("status").textContent = "Searching the archive…";
  try {
    const data = await api(`/api/search?${params}`);
    if (id !== generation) return;
    lastQuery = new URLSearchParams(params);
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
  search(params);
});
$("export").addEventListener("click", () => {
  // Export just the bounded page, losslessly, with provenance on each entry.
  const text =
    page
      .map((item) =>
        JSON.stringify({
          dataset: lastQuery.get("dataset"),
          source_id: item.source_id,
          record: item.record,
        }),
      )
      .join("\n") + "\n";
  const url = URL.createObjectURL(
    new Blob([text], { type: "application/x-ndjson" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = "ukinfo-page.jsonl";
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
});
(async () => {
  try {
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
    $("coverage").textContent =
      `${datasets.length} imported edition${datasets.length === 1 ? "" : "s"}`;
    $("submit").disabled = datasets.length === 0;
    if (!datasets.length)
      $("status").textContent =
        "No editions imported yet. Follow the catalogue import guide to add your local disc data.";
    editionNote();
  } catch (error) {
    $("coverage").textContent = "Connection unavailable";
    $("status").textContent = error.message;
    $("submit").disabled = true;
  }
})();
