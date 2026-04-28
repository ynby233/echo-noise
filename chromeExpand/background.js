const SITE_INFO_KEY = "capturedSiteInfo";
const CAPTURE_MENU_ID = "capture-site-info";

chrome.runtime.onInstalled.addListener(async () => {
  console.log("说说笔记扩展已安装");

  const result = await chrome.storage.sync.get(["siteUrl", "apiToken"]);
  if (!result.siteUrl) {
    await chrome.storage.sync.set({ siteUrl: "" });
  }
  if (!result.apiToken) {
    await chrome.storage.sync.set({ apiToken: "" });
  }

  chrome.contextMenus.create({
    id: CAPTURE_MENU_ID,
    title: "识别当前网站信息并写入说说笔记",
    contexts: ["page"]
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId !== CAPTURE_MENU_ID || !tab?.id) return;

  try {
    const [{ result }] = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => {
        const descriptionMeta =
          document.querySelector('meta[name="description"]')?.getAttribute("content") ||
          document.querySelector('meta[property="og:description"]')?.getAttribute("content") ||
          "";

        return {
          title: document.title || "",
          description: descriptionMeta || "",
          url: window.location.href || ""
        };
      }
    });

    await chrome.storage.local.set({ [SITE_INFO_KEY]: result });
  } catch (error) {
    console.error("识别网页信息失败", error);
    await chrome.storage.local.set({
      [SITE_INFO_KEY]: {
        title: tab.title || "",
        description: "",
        url: tab.url || ""
      }
    });
  }

  try {
    await chrome.action.openPopup();
  } catch (error) {
    console.warn("自动弹出失败，改为打开 popup 页面", error);
    await chrome.windows.create({
      url: chrome.runtime.getURL("popup.html"),
      type: "popup",
      width: 560,
      height: 760
    });
  }
});
