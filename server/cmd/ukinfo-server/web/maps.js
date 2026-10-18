"use strict";
(() => {
  let leaflet,
    map,
    tiles,
    config,
    generation = 0;
  const compact = (value) =>
    String(value || "")
      .replace(/\s/g, "")
      .toUpperCase();
  const full = (value) =>
    /^(?:[A-Z]{1,2}[0-9][A-Z0-9]?[0-9][A-Z]{2}|GIR0AA)$/.test(value);
  const pointFor = (item) => ({
    label: recordName(item.record) || "Unnamed listing",
    edition: editionTitle(item),
    postcode: item.record.postcode,
  });
  async function library() {
    if (window.L) return window.L;
    if (!leaflet)
      leaflet = new Promise((resolve, reject) => {
        const css = document.createElement("link");
        css.rel = "stylesheet";
        css.href = "/leaflet.css";
        document.head.append(css);
        const script = document.createElement("script");
        script.src = "/leaflet.js";
        script.onload = () => resolve(window.L);
        script.onerror = () => {
          leaflet = null;
          script.remove();
          reject(new Error("The local map library could not load."));
        };
        document.head.append(script);
      });
    return leaflet;
  }
  function background() {
    if (tiles && map) {
      map.removeLayer(tiles);
      tiles = null;
    }
    if (
      !map ||
      !config ||
      !$("map-online").checked ||
      config.tiles_url === "none"
    )
      return;
    const attribution = element("span", config.attribution),
      link = element("a", "OpenStreetMap licence");
    link.href = "https://www.openstreetmap.org/copyright";
    link.target = "_blank";
    link.rel = "noopener";
    attribution.append(document.createTextNode(" · "), link);
    tiles = window.L.tileLayer(config.tiles_url, {
      maxZoom: 17,
      keepBuffer: 0,
      updateWhenIdle: true,
      referrerPolicy: "origin",
      attribution: attribution.innerHTML,
    });
    tiles.on("tileerror", () => {
      $("map-tiles-note").textContent =
        "Street-map tiles are unavailable. Approximate positions and the location list remain usable; try again when online.";
    });
    tiles.addTo(map);
  }
  function cleanup() {
    generation++;
    if (map) {
      map.remove();
      map = null;
    }
    tiles = null;
    $("map-canvas").replaceChildren();
  }
  $("map-close").addEventListener("click", () => $("map-dialog").close());
  $("map-dialog").addEventListener("close", cleanup);
  $("map-online").addEventListener("change", background);
  window.openArchiveMap = async (entries, centre = null) => {
    if (entries.length > 100) {
      $("status").textContent = "Choose at most 100 saved entries to map.";
      return;
    }
    cleanup();
    const id = generation;
    $("map-status").textContent =
      "Resolving displayed postcodes in the local ONSPD reference…";
    $("map-locations").replaceChildren();
    $("map-attribution").textContent = "";
    $("map-online").checked = false;
    $("map-online").disabled = true;
    $("map-tiles-note").textContent =
      "Background tiles are optional. Loading them sends the viewed map area and your IP address to the tile provider; names and notes stay local.";
    if (!$("map-dialog").open) $("map-dialog").showModal();
    try {
      const groups = new Map();
      let incomplete = 0;
      for (const item of entries) {
        const code = compact(item.postcode);
        if (!full(code)) {
          incomplete++;
          continue;
        }
        if (!groups.has(code)) groups.set(code, []);
        groups.get(code).push(item);
      }
      let data = { points: [], reference: centre?.reference };
      if (groups.size) {
        const params = new URLSearchParams();
        for (const code of groups.keys()) params.append("postcode", code);
        data = await api(`/api/map?${params}`);
      }
      if (id !== generation) return;
      const centreValid =
        centre &&
        Number.isFinite(centre.latitude) &&
        Number.isFinite(centre.longitude);
      if (!data.points.length && !centreValid) {
        $("map-status").textContent =
          `No usable Great Britain postcode positions. ${incomplete} listings have incomplete or invalid displayed postcodes; other locations may be absent from this reference. Raw postcodes are not substituted.`;
        return;
      }
      const L = await library();
      config = await api("/api/map-config");
      if (id !== generation) return;
      $("map-online").disabled = config.tiles_url === "none";
      if (config.tiles_url === "none")
        $("map-tiles-note").textContent =
          "Online map backgrounds are disabled for this installation. Local postcode positions remain available.";
      map = L.map("map-canvas", {
        minZoom: 4,
        maxZoom: 17,
        scrollWheelZoom: false,
        attributionControl: true,
      });
      map.attributionControl.setPrefix(false);
      const initial = data.points[0] || centre;
      map.setView([initial.latitude, initial.longitude], 12);
      L.control.scale({ imperial: true, metric: true }).addTo(map);
      const bounds = L.latLngBounds([]);
      let located = 0;
      for (const point of data.points) {
        const items = groups.get(point.postcode) || [];
        located += items.length;
        const position = [point.latitude, point.longitude];
        bounds.extend(position);
        const content = element("div", "");
        content.append(
          element(
            "strong",
            `${point.postcode} · approximate postcode position`,
          ),
          element(
            "p",
            `${locationQuality(point.quality)}${point.terminated ? " · retired " + point.terminated : ""}`,
          ),
        );
        for (const item of items)
          content.append(element("p", `${item.label} · ${item.edition}`));
        const marker = L.circleMarker(position, {
          radius: 8,
          color: items.some((item) => item.edition !== items[0]?.edition)
            ? "#4266a3"
            : "#286b50",
          fillOpacity: 0.7,
        })
          .addTo(map)
          .bindPopup(content);
        const row = element("div", "", "map-location"),
          button = element(
            "button",
            `${point.postcode} · ${items.length} source listing${items.length === 1 ? "" : "s"}`,
          );
        button.type = "button";
        button.addEventListener("click", () => {
          map.setView(position, Math.max(map.getZoom(), 12));
          marker.openPopup();
        });
        row.append(
          button,
          element(
            "p",
            items.map((item) => `${item.label} — ${item.edition}`).join("; "),
            "note",
          ),
          element(
            "p",
            `${point.latitude.toFixed(5)}, ${point.longitude.toFixed(5)} · ${locationQuality(point.quality)}${point.terminated ? " · retired " + point.terminated : ""}`,
            "note",
          ),
        );
        $("map-locations").append(row);
      }
      if (centreValid) {
        const position = [centre.latitude, centre.longitude];
        const label =
          centre.centre_type && centre.centre_type !== "postcode"
            ? `Approximate ${centre.centre_type} centre (${centre.centre_postcodes} postcode points${centre.centre_retired_only ? ", retired only" : ""})`
            : "Approximate centre postcode";
        const circle = L.circle(position, {
          radius: centre.radius_m,
          color: "#a46d30",
          weight: 2,
          fillOpacity: 0.05,
          dashArray: "6 5",
        }).addTo(map);
        circle.bindPopup(
          element(
            "p",
            `${centre.near} · ${label}. Radius from centre, not an area boundary.`,
          ),
        );
        bounds.extend(circle.getBounds());
        $("map-locations").prepend(
          element(
            "p",
            `${centre.near} · ${label} · ${(centre.radius_m / 1609.344).toFixed(1)} mile radius from centre, not a district/sector boundary.`,
            "note",
          ),
        );
      }
      map.fitBounds(bounds, { padding: [24, 24], maxZoom: 13 });
      map.invalidateSize();
      $("map-status").textContent =
        `${located} of ${entries.length} source listings located at ${data.points.length} approximate postcode points. ${entries.length - located} listings not plotted. Markers may contain multiple editions; this is not the whole search or an exact house map.`;
      const year = data.reference?.release?.slice(0, 4) || "";
      $("map-attribution").textContent =
        `ONSPD ${data.reference?.release || ""}. Contains OS data © Crown copyright and database right ${year}. Contains Royal Mail data © Royal Mail copyright and database right ${year}. Source: Office for National Statistics, Open Government Licence v3.0. Green: one edition; blue: multiple editions. Modern street-map background is optional.`;
    } catch (error) {
      if (id === generation) $("map-status").textContent = error.message;
    }
  };
  $("map-record").addEventListener("click", () => {
    if (selectedRecord) window.openArchiveMap([pointFor(selectedRecord)]);
  });
  $("map-page").addEventListener("click", () =>
    window.openArchiveMap(page.map(pointFor), searchGeography),
  );
  // Keep the page action in step with successful searches and resets without adding
  // external geography or tile calls to the ordinary search path.
  new MutationObserver(() => {
    $("map-page").disabled = page.length === 0;
  }).observe($("items"), { childList: true });
})();
