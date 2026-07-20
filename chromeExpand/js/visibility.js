(function attachVisibilityHelpers(root) {
  const states = Object.freeze(["public", "users", "contacts", "private"]);

  const normalizeVisibility = (value) => {
    const normalized = String(value || "").trim().toLowerCase();
    return states.includes(normalized) ? normalized : "public";
  };

  const buildPublishPayload = (content, visibility, extra = {}) => {
    const normalized = normalizeVisibility(visibility);
    return {
      content,
      visibility: normalized,
      private: normalized !== "public",
      ...extra
    };
  };

  root.EchoNoiseVisibility = Object.freeze({
    states,
    normalizeVisibility,
    buildPublishPayload
  });
})(globalThis);
