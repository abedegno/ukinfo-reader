"use strict";
(() => {
  const model = window.UKWorkbench,
    key = "ukinfo.workbench.v1",
    bridge = window.ukinfoDesktop;
  const native = Boolean(bridge?.workbenchRead && bridge?.workbenchWrite);
  let queue = Promise.resolve(),
    latest = null,
    incoming = null;
  const read = async () =>
    model.validate(
      native
        ? await bridge.workbenchRead()
        : localStorage.getItem(key) === null
          ? model.empty()
          : model.parse(localStorage.getItem(key)),
    );
  function update(change) {
    const run = async () => {
      // Read inside the lock; unrelated changes are merged, not overwritten.
      const apply = async () => {
        const value = await read(),
          revision = value.revision;
        change(value);
        value.revision++;
        const valid = model.validate(value);
        if (native) await bridge.workbenchWrite(valid, revision);
        else localStorage.setItem(key, JSON.stringify(valid));
        return valid;
      };
      if (native) return apply();
      if (!navigator.locks)
        throw new Error(
          "Saving this workspace needs HTTPS or localhost with Web Locks support.",
        );
      return navigator.locks.request(key, apply);
    };
    const pending = queue.then(run);
    queue = pending.catch(() => {});
    return pending;
  }
  window.researchWorkspace = { read, update };
  const status = (message) => ($("workbench-status").textContent = message);
  const recipe = (params) =>
    model.query(
      Object.fromEntries(
        [...params].filter(([name]) => model.fields.has(name)),
      ),
    );
  const describe = (query) =>
    Object.entries(query)
      .filter(([name]) => !["limit", "dataset", "kind"].includes(name))
      .map(([name, value]) => `${name.replaceAll("_", " ")}: ${value}`)
      .join(" · ");
  function rerun(query) {
    $("workbench-dialog").close();
    applyRefinement(new URLSearchParams(query));
  }
  async function render() {
    const value = await read();
    $("workbench-logging").checked = value.logging;
    for (const [name, items] of [
      ["searches", value.searches],
      ["log", [...value.log].reverse()],
    ]) {
      const container = $("workbench-" + name);
      container.replaceChildren();
      if (!items.length)
        container.append(
          element(
            "p",
            name === "searches"
              ? "No saved searches yet. Run a search, then save its criteria here."
              : "No search history. History is optional and only records searches made while enabled.",
            "note",
          ),
        );
      for (const item of items) {
        const card = element("article", "", "research-card"),
          body = element("div", "", "research-entry");
        body.append(
          element("h3", item.name || new Date(item.created).toLocaleString()),
          element("p", describe(item.query)),
          element(
            "p",
            `Scope: ${item.query.dataset || "not specified"} · ${item.query.kind || "person"}`,
            "note",
          ),
        );
        if (name === "log")
          body.append(
            element(
              "p",
              `${item.count}${item.more ? "+" : ""} source entries on the first page. ${item.coverage}`,
              "note",
            ),
          );
        const actions = element("div", "", "record-actions"),
          run = element("button", "Run search"),
          remove = element("button", "Remove");
        run.type = remove.type = "button";
        run.addEventListener("click", () => rerun(item.query));
        remove.addEventListener("click", () =>
          perform((value) => {
            value[name] = value[name].filter((entry) => entry.id !== item.id);
          }, "Removed locally."),
        );
        actions.append(run, remove);
        body.append(actions);
        card.append(body);
        container.append(card);
      }
    }
    latest = lastQuery ? recipe(lastQuery) : null;
    $("workbench-save").disabled = !latest;
  }
  async function perform(change, message) {
    try {
      await update(change);
      await render();
      status(message);
    } catch (error) {
      status(error.message);
    }
  }
  $("workbench-open").addEventListener("click", async () => {
    $("workbench-dialog").showModal();
    try {
      await render();
      status(
        "Saved searches rerun against the currently available archive; results can change when editions are added or removed.",
      );
    } catch (error) {
      status(error.message);
    }
  });
  $("workbench-close").addEventListener("click", () =>
    $("workbench-dialog").close(),
  );
  $("workbench-save-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!latest) return;
    const name = $("workbench-name").value.trim();
    if (!name) return;
    const query = structuredClone(latest);
    perform((value) => {
      value.searches.push({
        id: crypto.randomUUID(),
        name,
        query,
        created: new Date().toISOString(),
      });
    }, "Search criteria saved locally.");
  });
  $("workbench-logging").addEventListener("change", (event) => {
    const enabled = event.target.checked;
    perform(
      (value) => {
        value.logging = enabled;
      },
      enabled
        ? "Local history enabled. Earlier searches are not recorded."
        : "History paused. Existing entries remain until cleared.",
    );
  });
  $("workbench-clear-log").addEventListener("click", () =>
    perform((value) => {
      value.log = [];
    }, "Search history cleared."),
  );
  $("workbench-export").addEventListener("click", async () => {
    try {
      const value = await read();
      downloadBlob(
        new Blob([JSON.stringify(value)], { type: "application/json" }),
        "ukinfo-private-workspace.json",
      );
      status(
        "Private workspace exported, including note drafts. Keep this file private; never attach it to issues.",
      );
    } catch (error) {
      status(error.message);
    }
  });
  $("workbench-file").addEventListener("change", async (event) => {
    incoming = null;
    $("workbench-import").disabled = true;
    try {
      const file = event.target.files[0];
      if (!file) return;
      if (file.size > model.MAX_BYTES)
        throw new Error("Workspace JSON must be at most 2 MiB.");
      incoming = model.parse(await file.text());
      $("workbench-preview").textContent =
        `${incoming.searches.length} saved searches will be added. Existing searches are kept. History, settings and drafts from this file will not be imported.`;
      $("workbench-import").disabled = !incoming.searches.length;
    } catch (error) {
      $("workbench-preview").textContent = error.message;
    }
  });
  $("workbench-import").addEventListener("click", async () => {
    if (!incoming) return;
    const searches = structuredClone(incoming.searches);
    await perform((value) => {
      for (const item of searches) {
        if (
          !value.searches.some(
            (existing) =>
              existing.name === item.name &&
              JSON.stringify(existing.query) === JSON.stringify(item.query),
          )
        )
          value.searches.push({ ...item, id: crypto.randomUUID() });
      }
    }, "Saved searches imported; identical saved criteria skipped.");
  });
  window.addEventListener("ukinfo-search-complete", async (event) => {
    const { params, count, more, coverage } = event.detail;
    const query = recipe(new URLSearchParams(params));
    latest = query;
    $("workbench-save").disabled = false;
    if (new URLSearchParams(params).has("cursor")) return;
    try {
      const stored = await read();
      if (!stored.logging) return;
      await update((value) => {
        if (!value.logging) return;
        value.log.push({
          id: crypto.randomUUID(),
          query,
          created: new Date().toISOString(),
          count,
          more: Boolean(more),
          coverage,
        });
        value.log = value.log.slice(-200);
      });
    } catch (error) {
      $("workspace-feedback").textContent =
        `Search history could not be saved: ${error.message}`;
    }
  });
})();
