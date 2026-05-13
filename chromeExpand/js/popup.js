let editor;
let toastTimer = null;
const EDITOR_CONTENT_KEY = "editorContent";
const SITE_INFO_KEY = "capturedSiteInfo";

const settingsBtn = document.getElementById("settingsBtn");
const sendBtn = document.getElementById("sendBtn");
const tagBtn = document.getElementById("tagBtn");
const linkBtn = document.getElementById("linkBtn");
const imageBtn = document.getElementById("imageBtn");
const clearBtn = document.getElementById("clearBtn");
const notifyToggle = document.getElementById("notifyToggle");
const resultContainer = document.getElementById("resultContainer");
const closeResultBtn = document.getElementById("closeResultBtn");
const previewContent = document.getElementById("previewContent");
const viewLink = document.getElementById("viewLink");
const settingsModal = document.getElementById("settingsModal");
const closeSettingsBtn = document.getElementById("closeSettingsBtn");
const cancelSettingsBtn = document.getElementById("cancelSettingsBtn");
const saveSettingsBtn = document.getElementById("saveSettingsBtn");
const siteUrlInput = document.getElementById("siteUrl");
const apiTokenInput = document.getElementById("apiToken");
const resultMessage = document.getElementById("resultMessage");
const pageTitleInput = document.getElementById("pageTitle");
const pageDescriptionInput = document.getElementById("pageDescription");
const pageUrlInput = document.getElementById("pageUrl");
const siteInfoCard = document.getElementById("siteInfoCard");
const siteInfoBadge = document.getElementById("siteInfoBadge");
const insertSiteInfoBtn = document.getElementById("insertSiteInfoBtn");
const hideSiteInfoBtn = document.getElementById("hideSiteInfoBtn");
const toast = document.getElementById("toast");

document.addEventListener("DOMContentLoaded", async () => {
  await initEditor();
  setupEventListeners();
  loadSettings();
  await restoreCapturedSiteInfo();
});

async function initEditor() {
  const { [EDITOR_CONTENT_KEY]: savedContent = "" } = await chrome.storage.local.get([EDITOR_CONTENT_KEY]);

  editor = new EasyMDE({
    element: document.getElementById("editor"),
    autofocus: true,
    spellChecker: false,
    placeholder: "灵感笔记..",
    toolbar: false,
    status: false,
    minHeight: "260px",
    maxHeight: "320px",
    sideBySideFullscreen: false,
    forceSync: true,
    autoSave: false,
    previewClass: "preview-content",
    renderingConfig: {
      singleLineBreaks: true,
      codeSyntaxHighlighting: true
    },
    autoDownloadFontAwesome: false,
    promptURLs: false,
    shortcuts: { togglePreview: null, toggleSideBySide: null, drawTable: null },
    previewRender: (plainText) => renderMarkdownPreview(plainText, siteUrlInput.value)
  });

  if (savedContent) {
    editor.value(savedContent);
  }

  editor.codemirror.on("change", () => {
    chrome.storage.local.set({ [EDITOR_CONTENT_KEY]: editor.value() });
  });
}

function setupEventListeners() {
  settingsBtn.addEventListener("click", openSettings);
  closeSettingsBtn.addEventListener("click", closeSettings);
  cancelSettingsBtn.addEventListener("click", closeSettings);
  saveSettingsBtn.addEventListener("click", saveSettings);
  settingsModal.addEventListener("click", (event) => {
    if (event.target === settingsModal) {
      closeSettings();
    }
  });

  tagBtn.addEventListener("click", () => insertMarkdown("#标签 "));
  linkBtn.addEventListener("click", insertLink);
  imageBtn.addEventListener("click", insertImage);
  clearBtn.addEventListener("click", clearEditorContent);
  sendBtn.addEventListener("click", sendMessage);
  closeResultBtn.addEventListener("click", () => resultContainer.classList.add("hidden"));
  viewLink.addEventListener("click", (event) => {
    const href = viewLink.getAttribute("href");
    if (!href || href === "#") return;
    event.preventDefault();
    chrome.tabs.create({ url: href });
  });
  insertSiteInfoBtn.addEventListener("click", insertSiteInfoToEditor);
  hideSiteInfoBtn.addEventListener("click", hideSiteInfoCard);
}

async function loadSettings() {
  const { siteUrl = "", apiToken = "" } = await chrome.storage.sync.get(["siteUrl", "apiToken"]);
  siteUrlInput.value = siteUrl;
  apiTokenInput.value = apiToken;
  await syncNotifySetting(siteUrl);
}

function openSettings() {
  settingsModal.classList.remove("hidden");
}

function closeSettings() {
  settingsModal.classList.add("hidden");
}

async function syncNotifySetting(siteUrl) {
  if (!notifyToggle) return;

  const base = normalizeSiteUrl(siteUrl);
  if (!base) {
    notifyToggle.checked = true;
    return;
  }

  try {
    const response = await fetch(joinSiteUrl(base, "/api/frontend/config"), {
      method: "GET",
      headers: { "Accept": "application/json" }
    });
    const data = await response.json();
    const enabled = data?.data?.frontendSettings?.notifyEnabled;
    notifyToggle.checked = enabled === true || enabled === "true";
  } catch (error) {
    console.warn("读取站内推送设置失败，使用默认开启", error);
    notifyToggle.checked = true;
  }
}

function normalizeSiteUrl(siteUrl) {
  const value = String(siteUrl || "").trim();
  if (!value) return "";

  try {
    const url = new URL(value);
    return `${url.origin}${url.pathname}`.replace(/\/+$/, "");
  } catch {
    return value.replace(/\/+$/, "");
  }
}

function joinSiteUrl(siteUrl, path) {
  const base = normalizeSiteUrl(siteUrl);
  const suffix = String(path || "");
  if (!base) return suffix;
  if (!suffix) return base;
  return `${base}/${suffix.replace(/^\/+/, "")}`;
}

function resolvePreviewUrl(rawUrl, siteUrl) {
  const value = String(rawUrl || "").trim();
  if (!value) return "";

  if (
    value.startsWith("data:") ||
    value.startsWith("mailto:") ||
    value.startsWith("tel:")
  ) {
    return value;
  }

  const base = normalizeSiteUrl(siteUrl);
  try {
    return base ? new URL(value, `${base}/`).toString() : new URL(value).toString();
  } catch {
    return value;
  }
}

function renderMarkdownPreview(content, siteUrl) {
  const html = marked.parse(content || "");
  const template = document.createElement("template");
  template.innerHTML = html;

  template.content.querySelectorAll("img").forEach((img) => {
    const src = img.getAttribute("src");
    const resolvedSrc = resolvePreviewUrl(src, siteUrl);
    if (resolvedSrc) {
      img.setAttribute("src", resolvedSrc);
    }
  });

  template.content.querySelectorAll("a").forEach((link) => {
    const href = link.getAttribute("href");
    const resolvedHref = resolvePreviewUrl(href, siteUrl);
    if (resolvedHref) {
      link.setAttribute("href", resolvedHref);
    }
    link.setAttribute("target", "_blank");
    link.setAttribute("rel", "noopener noreferrer");
  });

  return template.innerHTML;
}

function buildMessageViewUrl(siteUrl, messageId) {
  const base = normalizeSiteUrl(siteUrl);
  if (!base || !messageId) return "";
  return `${base}/#/messages/${encodeURIComponent(String(messageId))}`;
}

async function saveSettings() {
  const siteUrl = siteUrlInput.value.trim();
  const apiToken = apiTokenInput.value.trim();

  if (!siteUrl) {
    showToast("请输入站点地址", "error");
    siteUrlInput.focus();
    return;
  }

  try {
    const normalizedSiteUrl = normalizeSiteUrl(siteUrl);
    if (!normalizedSiteUrl) {
      throw new Error("invalid site url");
    }
    await chrome.storage.sync.set({ siteUrl: normalizedSiteUrl, apiToken });
    await syncNotifySetting(normalizedSiteUrl);
    showToast("设置已保存", "success");
    closeSettings();
  } catch (error) {
    showToast("站点地址格式不正确", "error");
    console.error(error);
  }
}

function insertMarkdown(text) {
  const cm = editor.codemirror;
  cm.replaceSelection(text);
  cm.focus();
}

function insertLink() {
  const url = window.prompt("请输入链接地址:", "https://");
  if (!url) return;
  const text = window.prompt("请输入链接文本:", "链接");
  if (!text) return;
  insertMarkdown(`[${text}](${url})`);
}

function insertImage() {
  const url = window.prompt("请输入图片地址:", "https://");
  if (!url) return;
  const alt = window.prompt("请输入图片描述:", "") || "";
  insertMarkdown(`![${alt}](${url})`);
}

async function clearEditorContent() {
  const current = editor.value().trim();
  if (!current) {
    showToast("当前没有可清空内容", "error");
    return;
  }

  const confirmed = window.confirm("确定一键清空当前内容吗？");
  if (!confirmed) return;

  editor.value("");
  resultContainer.classList.add("hidden");
  await chrome.storage.local.remove([EDITOR_CONTENT_KEY]);
  showToast("内容已清空", "success");
}

async function sendMessage() {
  const content = editor.value();
  if (!content.trim()) {
    showResult({ success: false, message: "请输入内容后再发送" });
    return;
  }

  const { siteUrl = "", apiToken = "" } = await chrome.storage.sync.get(["siteUrl", "apiToken"]);
  if (!siteUrl) {
    showToast("请先在设置中配置站点地址", "error");
    openSettings();
    return;
  }

  setSending(true);

  try {
    const shouldNotify = Boolean(notifyToggle?.checked);
    const payload = { content, private: false, notify: shouldNotify, username: "" };
    let response = await fetch(joinSiteUrl(siteUrl, "/api/token/messages"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: apiToken ? `Bearer ${apiToken}` : ""
      },
      body: JSON.stringify(payload)
    });

    if (!response.ok && !apiToken) {
      response = await fetch(joinSiteUrl(siteUrl, "/api/messages"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content, private: false, notify: shouldNotify }),
        credentials: "include"
      });
    }

    const data = await response.json();
    if (data?.code === 1) {
      const messageId = data?.data?.id || null;
      showResult({
        success: true,
        message: "发送成功",
        content,
        siteUrl,
        messageId
      });
      editor.value("");
      await chrome.storage.local.remove([EDITOR_CONTENT_KEY]);
      showToast("内容已发送", "success");
      return;
    }

    showResult({
      success: false,
      message: `发送失败: ${data?.msg || "未知错误"}`
    });
  } catch (error) {
    showResult({
      success: false,
      message: `发送失败: ${error.message || "网络异常"}`
    });
  } finally {
    setSending(false);
  }
}

function showResult({ success, message, content = "", siteUrl = "", messageId = null }) {
  resultMessage.textContent = message;
  resultContainer.classList.remove("hidden", "success", "error");
  resultContainer.classList.add(success ? "success" : "error");

  if (success && content) {
    previewContent.innerHTML = renderMarkdownPreview(content, siteUrl);
    previewContent.classList.remove("hidden");
  } else {
    previewContent.innerHTML = "";
    previewContent.classList.add("hidden");
  }

  if (success && siteUrl && messageId) {
    viewLink.href = buildMessageViewUrl(siteUrl, messageId);
    viewLink.classList.remove("hidden");
  } else {
    viewLink.classList.add("hidden");
  }
}

function setSending(isSending) {
  sendBtn.disabled = isSending;
  sendBtn.textContent = isSending ? "发送中..." : "发送";
}

function showToast(message, type = "success") {
  toast.textContent = message;
  toast.classList.remove("hidden", "success", "error");
  toast.classList.add(type);
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    toast.classList.add("hidden");
  }, 2200);
}

function applySiteInfo(info, markRecognized = true) {
  const title = (info?.title || "").trim();
  const description = (info?.description || "").trim();
  const url = (info?.url || "").trim();

  pageTitleInput.value = title;
  pageDescriptionInput.value = description;
  pageUrlInput.value = url;

  const hasData = Boolean(title || description || url);
  siteInfoCard.classList.toggle("hidden", !hasData);
  siteInfoBadge.textContent = hasData ? "已识别" : "未识别";
  siteInfoBadge.classList.toggle("success", hasData && markRecognized);
}

function insertSiteInfoToEditor() {
  const title = pageTitleInput.value.trim();
  const description = pageDescriptionInput.value.trim();
  const url = pageUrlInput.value.trim();
  if (!title && !description && !url) {
    showToast("暂无可插入的网站信息", "error");
    return;
  }

  const lines = [];
  if (title) lines.push(title);
  if (description) lines.push(description);
  if (url) lines.push(url);

  insertMarkdown(`${lines.join("\n\n")}\n\n`);
  showToast("网站信息已插入正文", "success");
}

function hideSiteInfoCard() {
  siteInfoCard.classList.add("hidden");
}

async function restoreCapturedSiteInfo() {
  const localData = await chrome.storage.local.get([SITE_INFO_KEY]);
  if (localData?.[SITE_INFO_KEY]) {
    applySiteInfo(localData[SITE_INFO_KEY], true);
    await chrome.storage.local.remove([SITE_INFO_KEY]);
    showToast("已自动识别当前网页信息", "success");
    return;
  }
}
