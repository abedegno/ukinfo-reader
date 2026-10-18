/* Private research workspace; shared browser/desktop validation. */
(function (root) {
  "use strict";
  const MAX_BYTES = 2 * 1024 * 1024;
  const fields = new Set([
    "dataset",
    "kind",
    "surname",
    "first_name",
    "first_match",
    "middle_initial",
    "organisation",
    "match",
    "postcode",
    "near",
    "radius_m",
    "building",
    "house",
    "flat",
    "street",
    "locality",
    "town",
    "county",
    "limit",
  ]);
  function fail() {
    throw new Error(
      "Invalid research workspace (version 1, maximum 2 MiB, 100 saved searches, 200 log entries and 500 note drafts).",
    );
  }
  function text(value, max = 4000) {
    if (typeof value !== "string" || value.length > max || value.includes("\0"))
      fail();
    return value;
  }
  function object(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) fail();
    return value;
  }
  function query(value) {
    object(value);
    const result = {};
    for (const [key, val] of Object.entries(value)) {
      if (!fields.has(key)) fail();
      result[key] = text(val, 300);
    }
    return result;
  }
  function date(value) {
    text(value, 40);
    if (!Number.isFinite(Date.parse(value))) fail();
    return value;
  }
  function empty() {
    return {
      format: "ukinfo-workbench",
      version: 1,
      revision: 0,
      searches: [],
      logging: false,
      log: [],
      drafts: [],
    };
  }
  function validate(value) {
    object(value);
    if (
      value.format !== "ukinfo-workbench" ||
      value.version !== 1 ||
      !Number.isSafeInteger(value.revision) ||
      value.revision < 0 ||
      typeof value.logging !== "boolean"
    )
      fail();
    function list(items, max, convert) {
      if (!Array.isArray(items) || items.length > max) fail();
      const ids = new Set();
      return items.map((item) => {
        object(item);
        const id = text(item.id, 100);
        if (!id || ids.has(id)) fail();
        ids.add(id);
        return { id, ...convert(item) };
      });
    }
    const result = {
      ...empty(),
      revision: value.revision,
      logging: value.logging,
      searches: list(value.searches, 100, (item) => ({
        name: text(item.name, 100),
        query: query(item.query),
        created: date(item.created),
      })),
      log: list(value.log, 200, (item) => {
        if (
          !Number.isSafeInteger(item.count) ||
          item.count < 0 ||
          item.count > 1000 ||
          typeof item.more !== "boolean"
        )
          fail();
        return {
          query: query(item.query),
          created: date(item.created),
          count: item.count,
          more: item.more,
          coverage: text(item.coverage, 4000),
        };
      }),
      drafts: list(value.drafts, 500, (item) => ({
        note: text(item.note),
        base: text(item.base),
        updated: date(item.updated),
      })),
    };
    if (new TextEncoder().encode(JSON.stringify(result)).length > MAX_BYTES)
      fail();
    return result;
  }
  const api = {
    MAX_BYTES,
    fields,
    query,
    empty,
    validate,
    parse: (raw) => {
      if (
        typeof raw !== "string" ||
        new TextEncoder().encode(raw).length > MAX_BYTES
      )
        fail();
      return validate(JSON.parse(raw));
    },
  };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.UKWorkbench = api;
})(typeof window !== "undefined" ? window : globalThis);
