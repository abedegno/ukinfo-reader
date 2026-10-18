"use strict";
(() => {
  let state = null,
    generation = 0;
  async function open(item, scope, cursor = "", trail = []) {
    const id = ++generation;
    state = { item, scope, cursor, trail, next: "" };
    $("group-title").textContent = recordName(item.record);
    $("group-status").textContent = "Loading source occurrences…";
    $("group-sources").replaceChildren();
    $("group-next").hidden = $("group-previous").hidden = true;
    $("group-export").disabled = true;
    if (!$("group-dialog").open) $("group-dialog").showModal();
    try {
      const params = new URLSearchParams({
        dataset: scope,
        group_key: item.group_key,
      });
      if (cursor) params.set("cursor", cursor);
      const data = await api(`/api/group?${params}`);
      if (id !== generation) return;
      state.next = data.next_cursor;
      for (const source of data.items) {
        const card = element("article", "", "research-card");
        card.append(
          element("h3", editionTitle(source)),
          element("p", recordName(source.record)),
          element("p", addressText(source.record)),
          element("p", citation(source), "note"),
        );
        const button = element("button", "View source record");
        button.type = "button";
        button.addEventListener("click", () => {
          $("group-dialog").close();
          details(source, [source]);
        });
        card.append(button);
        $("group-sources").append(card);
      }
      $("group-status").textContent =
        `${item.occurrence_count} source occurrences in this group · ${data.items.length} on this page. ${data.next_cursor ? "More sources available." : "End of source occurrences."}`;
      $("group-next").hidden = !state.next;
      $("group-previous").hidden = !trail.length;
      $("group-export").disabled = !data.items.length;
    } catch (error) {
      if (id === generation) $("group-status").textContent = error.message;
    }
  }
  window.openSourceGroup = open;
  $("group-close").addEventListener("click", () => {
    $("group-dialog").close();
    generation++;
  });
  $("group-next").addEventListener("click", () =>
    open(state.item, state.scope, state.next, [...state.trail, state.cursor]),
  );
  $("group-previous").addEventListener("click", () =>
    open(state.item, state.scope, state.trail.at(-1), state.trail.slice(0, -1)),
  );
  $("group-export").addEventListener("click", async () => {
    const params = new URLSearchParams({
      dataset: state.scope,
      group_key: state.item.group_key,
      format: "jsonl",
    });
    if (state.cursor) params.set("cursor", state.cursor);
    try {
      const response = await fetch(`/api/group?${params}`);
      if (!response.ok)
        throw new Error(
          (await response.json()).error || "Source export unavailable",
        );
      downloadBlob(await response.blob(), "ukinfo-group-source-page.jsonl");
    } catch (error) {
      $("group-status").textContent = error.message;
    }
  });
})();
