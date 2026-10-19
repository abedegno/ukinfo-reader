"use strict";
((root, factory) => {
  if (typeof module === "object" && module.exports) module.exports = factory();
  else root.UKInfoHTTP = factory();
})(globalThis, () => {
  class HTTPError extends Error {
    constructor(message, { status = 0, code = "network_error", field, requestId, retryAfter = 0 } = {}) {
      super(message);
      this.name = "HTTPError";
      Object.assign(this, { status, code, field, requestId, retryAfter });
      this.retryable = [0, 408, 429, 502, 503, 504].includes(status) && code !== "catalogue_integrity";
    }
  }
  function retryDelay(value, now = Date.now()) {
    if (!value) return 0;
    const seconds = /^\d+$/.test(value) ? Number(value) : (Date.parse(value) - now) / 1000;
    return Number.isFinite(seconds) ? Math.max(0, Math.min(3600, Math.ceil(seconds))) : 0;
  }
  async function request(path, { signal, format = "json" } = {}) {
    let response;
    try {
      response = await fetch(path, { credentials: "same-origin", signal });
    } catch (error) {
      if (signal?.aborted || error.name === "AbortError") throw error;
      throw new HTTPError("Unable to reach the archive. Check your connection and try again.");
    }
    if (!response.ok) {
      let data = {};
      if ((response.headers.get("content-type") || "").includes("application/json")) {
        try { data = await response.json(); } catch (error) { if (signal?.aborted) throw error; }
      }
      const message = typeof data.error === "string" ? data.error :
        response.status === 429 ? "The archive is busy. Please wait before trying again." :
        response.status === 403 ? "The request was blocked by the service. Try again or contact the operator." :
        "The archive is temporarily unavailable. Please try again shortly.";
      throw new HTTPError(message, {
        status: response.status,
        code: typeof data.code === "string" ? data.code : "http_error",
        field: typeof data.field === "string" ? data.field : undefined,
        requestId: response.headers.get("X-Request-ID") || data.request_id,
        retryAfter: retryDelay(response.headers.get("Retry-After")),
      });
    }
    try {
      return format === "response" ? response : format === "text" ? await response.text() : await response.json();
    } catch (error) {
      if (signal?.aborted || error.name === "AbortError") throw error;
      throw new HTTPError("The service returned an unreadable response. Please try again.", {
        status: 502, code: "invalid_response", requestId: response.headers.get("X-Request-ID"),
      });
    }
  }
  return { HTTPError, retryDelay, request };
});
