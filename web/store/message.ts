import { defineStore } from "pinia";
import { ref, watch } from "vue";
import type { Message, MessagePageLocateResult, PageQuery, PageQueryResult } from "~/types/models";
import { useUserStore } from "~/store/user";

export const useMessageStore = defineStore("messageStore", () => {
  // 状态
  const messages = ref<Message[]>([]);
  const total = ref(0);
  const hasMore = ref(true);
  const page = ref<number>(1);
  const pageSize = ref(15);
  const toast = useToast();
  const loading = ref<boolean>(false);
  const siteConfig = ref<any>(null);  // 添加网站配置状态
  const tags = ref<any[]>([]);  // 添加标签状态
  const images = ref<any[]>([]); // 添加图片状态
  const notifyConfig = ref<any>(null); // 添加推送配置状态
  const userStore = useUserStore();
  let requestController: AbortController | null = null;
  let requestSeq = 0;
  const currentListQueryKey = ref("")

  const listQueryKey = (query: PageQuery) => JSON.stringify({
    pageSize: query.pageSize,
    authorId: query.authorId ?? null,
    username: query.username ?? "",
    date: query.date ?? "",
    keyword: query.keyword ?? "",
    tag: query.tag ?? "",
    pinScope: query.pinScope ?? "latest",
    excludeId: query.excludeId ?? null,
  })

  // 只跟踪影响读取权限的身份字段，资料刷新不应中断正在加载的列表。
  const identityKey = () => JSON.stringify([
    userStore.token,
    userStore.isLogin,
    userStore.user?.userid ?? userStore.user?.id ?? userStore.user?.user_id ?? userStore.user?.ID ?? null,
    userStore.user?.username ?? userStore.user?.Username ?? "",
    userStore.user?.is_admin ?? userStore.user?.IsAdmin ?? false,
  ]);

  // 分页和定位属于同一条导航链；任何新请求均使旧链中的未完成请求失效。
  const cancelRequests = () => {
    requestSeq += 1;
    const controller = requestController;
    requestController = null;
    loading.value = false;
    controller?.abort();
  };

  const beginRequest = (isPage: boolean) => {
    cancelRequests();
    const controller = new AbortController();
    requestController = controller;
    const seq = requestSeq;
    const identity = identityKey();
    loading.value = isPage;
    const isCurrent = () => seq === requestSeq && !controller.signal.aborted && identity === identityKey();
    return {
      signal: controller.signal,
      isCurrent,
      finish: () => {
        if (!isCurrent()) return;
        requestController = null;
        loading.value = false;
      },
    };
  };

  // 重置状态
  const reset = () => {
    cancelRequests();
    messages.value = [];
    total.value = 0;
    hasMore.value = true;
    page.value = 1;
    loading.value = false;
    currentListQueryKey.value = "";
  };

  // 在登录、退出或会话恢复的同一调用栈清除旧身份的数据与请求。
  watch(identityKey, reset, { flush: 'sync' });
 // 获取网站配置
 const getSiteConfig = async () => {
  try {
    const response = await getRequest<any>("site/config", undefined, {
      credentials: 'include'
    });
    
    if (!response || response.code !== 1) {
      toast.add({
        title: "获取网站配置失败",
        description: response?.msg || "请稍后重试",
        icon: "i-fluent-error-circle-16-filled",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    // 确保更新状态
    siteConfig.value = response.data;
    
    // 触发响应式更新
    nextTick();
    
    return response.data;
  } catch (error) {
    console.error("获取网站配置失败:", error);
    throw error;
  }
};

// 更新网站配置
const updateSiteConfig = async (key: string, value: any) => {
  try {
    const response = await putRequest<any>("site/config", { [key]: value }, {
      credentials: 'include'
    });

    if (!response || response.code !== 1) {
      toast.add({
        title: "更新配置失败",
        description: response?.msg,
        icon: "i-fluent-error-circle-16-filled",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    // 更新本地配置状态
    siteConfig.value = { ...siteConfig.value, [key]: value };
    return response.data;
  } catch (error) {
    console.error("更新配置失败:", error);
    throw error;
  }
};

// 两个分页入口共享请求与应用逻辑，旧入口保留按已加载条数判断 hasMore 的行为。
const requestMessagePage = async (query: PageQuery, options: { append: boolean; legacy?: boolean }) => {
  const requestListKey = listQueryKey(query);
  const requestQuery = { ...query };
  const request = beginRequest(true);

  try {
    const response = await postRequest<PageQueryResult>("messages/page", requestQuery, {
      credentials: 'include',
      silent: true,
      signal: request.signal
    });
    if (!request.isCurrent()) return null;
    if (!response || response.code !== 1 || !Array.isArray(response.data?.items)) {
      if (options.legacy) {
        toast.add({
          title: "获取笔记列表失败",
          description: response?.msg || "请稍后重试",
          icon: "i-fluent-error-circle-16-filled",
          color: "red",
          timeout: 2000,
        });
      }
      return null;
    }

    if (options.append && currentListQueryKey.value === requestListKey) {
      const existing = new Set(messages.value.map(item => item.id));
      const newItems = response.data.items.filter(item => {
        if (existing.has(item.id)) return false;
        existing.add(item.id);
        return true;
      });
      messages.value = [...messages.value, ...newItems];
    } else {
      messages.value = response.data.items;
    }
    total.value = response.data.total;
    page.value = requestQuery.page;
    pageSize.value = requestQuery.pageSize;
    hasMore.value = options.legacy
      ? messages.value.length < total.value
      : page.value * pageSize.value < total.value;
    currentListQueryKey.value = requestListKey;

    return response.data;
  } catch (error) {
    if (!request.isCurrent() || (error && typeof error === 'object' && 'name' in error && error.name === 'AbortError')) return null;
    console.error("获取笔记列表失败:", error);
    if (options.legacy) {
      toast.add({
        title: "获取笔记列表失败",
        description: "请稍后重试",
        icon: "i-fluent-error-circle-16-filled",
        color: "red",
        timeout: 2000,
      });
    }
    return null;
  } finally {
    request.finish();
  }
};

const getMessages = (query: PageQuery) => requestMessagePage(query, { append: query.page !== 1, legacy: true });

const loadMessagePage = (query: PageQuery, options: { append?: boolean } = {}) =>
  requestMessagePage(query, { append: !!options.append });

const locateMessagePage = async (query: PageQuery & { messageId: number }) => {
  const request = beginRequest(false);
  try {
    const response = await postRequest<MessagePageLocateResult>("messages/locate", {
      messageId: query.messageId,
      pageSize: query.pageSize,
      authorId: query.authorId,
      username: query.username,
      date: query.date,
      keyword: query.keyword,
      tag: query.tag,
      pinScope: query.pinScope,
      excludeId: query.excludeId,
    }, {
      credentials: 'include',
      silent: true,
      signal: request.signal
    });
    if (!request.isCurrent()) return null;
    if (!response || response.code !== 1) return null;
    return response.data;
  } catch (error) {
    if (!request.isCurrent() || (error && typeof error === 'object' && 'name' in error && error.name === 'AbortError')) return null;
    console.error("定位笔记分页失败:", error);
    return null;
  } finally {
    request.finish();
  }
}

  // 删除笔记
  const deleteMessage = async (id: number) => {
    try {
      const response = await deleteRequest<any>(`messages/${id}`, undefined, {
        credentials: 'include'
      });
      
      if (!response || response.code !== 1) {
        toast.add({
          title: "删除笔记失败",
          description: response?.msg,
          icon: "i-fluent-error-circle-16-filled",
          color: "red",
          timeout: 2000,
        });
        return null;
      }

      messages.value = messages.value.filter((message) => message.id !== id);
      total.value -= 1;
      
      return response;
    } catch (error) {
      console.error("删除笔记失败:", error);
      throw error;
    }
  };

  // 按ID获取单条消息
  const getMessageById = async (id: string) => {
    try {
    const response = await getRequest<any>(`messages/${id}`, undefined, {
      credentials: 'include'
    });
      if (!response || response.code !== 1) {
        toast.add({
          title: "获取消息失败",
          description: response?.msg || "请稍后重试",
          icon: "i-fluent-error-circle-16-filled",
          color: "red",
          timeout: 2000,
        });
        return null;
      }
      return response.data;
    } catch (error) {
      console.error("获取消息失败:", error);
      throw error;
    }
  };

  // 更新单条笔记
  const updateMessage = async (id: number, content: string) => {
    try {
      const response = await putRequest<any>(`messages/${id}`, { content }, {
        credentials: 'include'
      });

      if (!response || response.code !== 1) {
        toast.add({
          title: "更新笔记失败",
          description: response?.msg,
          icon: "i-fluent-error-circle-16-filled",
          color: "red",
          timeout: 2000,
        });
        return null;
      }

      const index = messages.value.findIndex(msg => msg.id === id);
      if (index !== -1) {
        messages.value[index] = response.data;
      }

      return response;
    } catch (error) {
      console.error("更新笔记失败:", error);
      throw error;
    }
  };

  const setPrivate = async (id: number, priv: boolean) => {
    try {
      const response = await putRequest<any>(`messages/${id}`, { private: priv }, {
        credentials: 'include'
      });
      if (!response || response.code !== 1) {
        toast.add({
          title: "更新私密状态失败",
          description: response?.msg,
          icon: "i-fluent-error-circle-16-filled",
          color: "red",
          timeout: 2000,
        });
        return null;
      }
      const index = messages.value.findIndex(msg => msg.id === id);
      if (index !== -1) {
        messages.value[index] = response.data;
      }
      return response;
    } catch (error) {
      console.error("更新私密状态失败:", error);
      throw error;
    }
  };

  // 切换置顶状态
  const setPin = async (id: number, pinned: boolean, pinScope: 'latest' | 'personal') => {
    try {
      const response = await putRequest<any>(`messages/${id}/pin/${pinScope === 'personal' ? 'personal' : 'global'}`, { pinned }, {
        credentials: 'include'
      });
      if (!response || response.code !== 1) {
        toast.add({
          title: "更新置顶状态失败",
          description: response?.msg,
          icon: "i-fluent-error-circle-16-filled",
          color: "red",
          timeout: 2000,
        });
        return null;
      }
      return response;
    } catch (error) {
      console.error("更新置顶状态失败:", error);
      throw error;
    }
  };
 // 获取所有标签
 const getAllTags = async () => {
  try {
    const response = await getRequest<any>("messages/tags", undefined, {
      credentials: 'include'
    });
    
    if (!response || response.code !== 1) {
      toast.add({
        title: "获取标签列表失败",
        description: response?.msg || "请稍后重试",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    tags.value = response.data;
    return response.data;
  } catch (error) {
    console.error("获取标签列表失败:", error);
    throw error;
  }
};

// 根据标签获取消息
const getMessagesByTag = async (tag: string) => {
  try {
    const response = await getRequest<any>(`messages/tags/${encodeURIComponent(tag)}`, undefined, {
      credentials: 'include'
    });
    
    if (!response || response.code !== 1) {
      toast.add({
        title: "获取标签消息失败",
        description: response?.msg || "请稍后重试",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    return response.data;
  } catch (error) {
    console.error("获取标签消息失败:", error);
    throw error;
  }
};

// 获取所有图片
const getAllImages = async () => {
  try {
    const response = await getRequest<any>("messages/images", undefined, {
      credentials: 'include'
    });
    
    if (!response || response.code !== 1) {
      toast.add({
        title: "获取图片列表失败",
        description: response?.msg || "请稍后重试",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    images.value = response.data;
    return response.data;
  } catch (error) {
    console.error("获取图片列表失败:", error);
    throw error;
  }
};
  // 获取推送配置
const getNotifyConfig = async () => {
  try {
    const response = await getRequest<any>("notify/config", undefined, {
      credentials: 'include'
    });
    
    if (!response || response.code !== 1) {
      toast.add({
        title: "获取推送配置失败",
        description: response?.msg || "请稍后重试",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    notifyConfig.value = response.data;
    return response.data;
  } catch (error) {
    console.error("获取推送配置失败:", error);
    throw error;
  }
};

// 更新推送配置
const updateNotifyConfig = async (config: any) => {
  try {
    const response = await putRequest<any>("notify/config", config, {
      credentials: 'include'
    });

    if (!response || response.code !== 1) {
      toast.add({
        title: "更新推送配置失败",
        description: response?.msg || "请稍后重试",
        color: "red",
        timeout: 2000,
      });
      return null;
    }

    notifyConfig.value = response.data;
    return response.data;
  } catch (error) {
    console.error("更新推送配置失败:", error);
    throw error;
  }
};

// 测试推送
const testNotify = async (type: string) => {
  try {
    const response = await postRequest<any>(`notify/test?type=${type}`, {}, {
      credentials: 'include'
    });

    if (!response || response.code !== 1) {
      throw new Error(response?.msg || "测试失败");
    }

    return response.data;
  } catch (error) {
    console.error("推送测试失败:", error);
    throw error;
  }
};

// 创建消息
const createMessage = async (message: Message) => {
  try {
    const response = await postRequest<any>("messages", message, {
      credentials: 'include'
    });

    if (!response || response.code !== 1) {
      throw new Error(response?.msg || "创建消息失败");
    }

    // 如果启用了推送
    if (message.notify) {
      try {
        const baseUrl = useRuntimeConfig().public.baseApi;
        const pushContent = {
          content: message.content,
          images: message.image_url 
            ? [`${baseUrl}${message.image_url}`].filter(Boolean) 
            : [],
          format: "markdown"
        };

        const notifyResponse = await postRequest<any>("notify/send", pushContent, {
          credentials: 'include'
        });

        if (!notifyResponse || notifyResponse.code !== 1) {
          console.warn("推送失败:", notifyResponse?.msg);
        }
      } catch (error) {
        console.error("消息推送失败:", error);
      }
    }

    // 添加成功提示
    toast.add({
      title: '成功',
      description: '发布成功',
      color: 'green',
      timeout: 2000
    });

    return response.data;
  } catch (error) {
    console.error("创建消息失败:", error);
    throw error;
  }
};
// 返回所有方法和状态
  return {
  messages,
  total,
  hasMore,
  page,
  pageSize,
  loading,
  currentListQueryKey,
  listQueryKey,
  siteConfig,
  reset,
  cancelRequests,
  getMessages,
  loadMessagePage,
  locateMessagePage,
  deleteMessage,
  updateMessage,
  setPrivate,
  setPin,
  getMessageById,
  getSiteConfig,
  updateSiteConfig,
  tags,
  images,
  getAllTags,
  getMessagesByTag,
  getAllImages,
  notifyConfig,
  getNotifyConfig,
  updateNotifyConfig,
  testNotify,
  createMessage,
};
});
