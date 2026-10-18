/* Shared browser/desktop research-file validation. No directory records bundled. */
(function (root) {
  "use strict";
  const MAX_BYTES = 2 * 1024 * 1024;
  const fail = () => {
    throw new Error(
      "Invalid research file. Use a UK-Info research JSON export (version 1, at most 2 MiB, 50 lists and 2,000 entries).",
    );
  };
  function text(value, max, required = false) {
    if (
      typeof value !== "string" ||
      value.length > max ||
      value.includes("\0") ||
      (required && !value.trim())
    )
      fail();
    return value;
  }
  function object(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) fail();
    return value;
  }
  function date(value) {
    text(value, 40, true);
    if (!Number.isFinite(Date.parse(value))) fail();
    return value;
  }
  function empty() {
    return { format: "ukinfo-research", version: 1, revision: 0, lists: [] };
  }
  function validate(input) {
    object(input);
    if (
      input.format !== "ukinfo-research" ||
      input.version !== 1 ||
      !Number.isSafeInteger(input.revision) ||
      input.revision < 0 ||
      !Array.isArray(input.lists) ||
      input.lists.length > 50
    )
      fail();
    const ids = new Set();
    const unique = (value) => {
      text(value, 100, true);
      if (ids.has(value)) fail();
      ids.add(value);
      return value;
    };
    let count = 0;
    const lists = input.lists.map((list) => {
      object(list);
      if (!Array.isArray(list.entries) || (count += list.entries.length) > 2000)
        fail();
      return {
        id: unique(list.id),
        name: text(list.name, 100, true),
        entries: list.entries.map((item) => {
          object(item);
          object(item.ref);
          object(item.summary);
          if (
            !Number.isInteger(item.ref.id) ||
            item.ref.id < 1 ||
            item.ref.id > 2147483647 ||
            !["record", "address"].includes(item.mode) ||
            !["edition", "all"].includes(item.scope)
          )
            fail();
          const summary = {};
          for (const [field, max] of Object.entries({
            title: 512,
            address: 2048,
            postcode: 32,
            edition: 512,
            listing: 128,
            phone: 128,
            citation: 2048,
            warnings: 2048,
          }))
            summary[field] = text(item.summary[field], max);
          return {
            id: unique(item.id),
            mode: item.mode,
            scope: item.scope,
            ref: {
              dataset: text(item.ref.dataset, 256, true),
              id: item.ref.id,
              source_id: text(item.ref.source_id, 512, true),
            },
            summary,
            note: text(item.note, 4000),
            created: date(item.created),
            updated: date(item.updated),
          };
        }),
      };
    });
    const result = {
      format: input.format,
      version: 1,
      revision: input.revision,
      lists,
    };
    if (new TextEncoder().encode(JSON.stringify(result)).length > MAX_BYTES)
      fail();
    return result;
  }
  function parse(raw) {
    if (
      typeof raw !== "string" ||
      new TextEncoder().encode(raw).length > MAX_BYTES
    )
      fail();
    try {
      return validate(JSON.parse(raw));
    } catch {
      fail();
    }
  }
  function key(item) {
    return JSON.stringify([
      item.mode,
      item.scope,
      item.ref.dataset,
      item.ref.id,
      item.ref.source_id,
    ]);
  }
  function csv(list) {
    const cell = (value) => {
      let s = String(value ?? "");
      if (/^(?:\s*[=+@\-]|[\t\r\n])/.test(s)) s = "'" + s;
      return '"' + s.replaceAll('"', '""') + '"';
    };
    const rows = [
      [
        "List",
        "View",
        "Scope",
        "Name",
        "Address",
        "Postcode",
        "Edition",
        "Listing type",
        "Telephone",
        "Source entry",
        "Citation",
        "Note",
        "Saved",
        "Updated",
      ],
      ...list.entries.map((e) => [
        list.name,
        e.mode,
        e.scope,
        e.summary.title,
        e.summary.address,
        e.summary.postcode,
        e.summary.edition,
        e.summary.listing,
        e.summary.phone,
        e.ref.source_id,
        e.summary.citation,
        e.note,
        e.created,
        e.updated,
      ]),
    ];
    return (
      "\ufeff" +
      rows.map((row) => row.map(cell).join(",")).join("\r\n") +
      "\r\n"
    );
  }
  const api = { MAX_BYTES, empty, validate, parse, key, csv };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.UKResearch = api;
})(globalThis);
