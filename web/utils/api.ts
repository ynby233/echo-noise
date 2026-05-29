import type { Response } from "~/types/models";
import { useUserStore } from "~/store/user";
import { useToast } from "#imports";

const FIRST_LOAD_SUPPRESS_MS = 8000
let initialSuppressUntil = 0
let authGuideRedirecting = false
if (typeof window !== 'undefined') {
  const now = Date.now()
  initialSuppressUntil = now + FIRST_LOAD_SUPPRESS_MS
}
const isMobileDevice = () => (typeof window !== 'undefined') && window.matchMedia('(max-width: 1024px)').matches
const shouldSuppressToast = (options?: { silent?: boolean }) => {
  if (options && (options as any).silent) return true
  if (typeof window === 'undefined') return false
  if (isMobileDevice()) return true
  return Date.now() < initialSuppressUntil
}

const redirectToAuthGuide = () => {
    if (typeof window === 'undefined') return
    if (authGuideRedirecting) return
    if (window.location.pathname === '/auth/guide') return
    authGuideRedirecting = true
    const redirect = encodeURIComponent(window.location.pathname + window.location.search)
    window.location.href = `/auth/guide?reason=expired&redirect=${redirect}`
}

const handleAuthExpired = (msg?: string, options?: { silent?: boolean }) => {
    const userStore = useUserStore();
    const wasLoggedIn = !!userStore.isLogin || !!userStore.token
    if (!wasLoggedIn) return
    userStore.clearUserStatus();
    const toast = useToast();
    if (!shouldSuppressToast(options)) {
        toast.add({ title: '登录已过期', description: msg || '请重新登录', color: 'red', timeout: 2000 });
    }
    redirectToAuthGuide()
}

const isAuthExpiredMsg = (msg?: string) => {
    const m = (msg || '').toLowerCase()
    if (!m) return false
    return m.includes('未登录') ||
        m.includes('登录已过期') ||
        m.includes('无效的token') ||
        m.includes('无效token') ||
        m.includes('token已失效') ||
        m.includes('未提供认证信息') ||
        m.includes('认证格式错误')
}

const handleAuthExpiredFromResponse = (res: any, options?: { silent?: boolean }) => {
    const code = (res as any)?.code
    if (code === 1) return
    const msg = (res as any)?.msg
    if (isAuthExpiredMsg(msg)) {
        handleAuthExpired(msg, options)
    }
}

const extractServerMsg = (error: any, fallback = '网络异常') => {
    return error?.response?._data?.msg || error?.data?.msg || error?.response?.statusText || error?.message || fallback
}

const handleHttpStatusError = <T>(status: any, msg?: string, options?: { silent?: boolean }) => {
    const normalizedStatus = Number(status || 0)
    if (normalizedStatus === 401) {
        const authMsg = msg || '未登录或登录已过期'
        handleAuthExpired(authMsg, options)
        return { code: 0, msg: authMsg, data: null } as any as Response<T>
    }
    if (normalizedStatus === 403) {
        const forbiddenMsg = msg || '当前账号没有权限执行此操作'
        if (!shouldSuppressToast(options)) {
            useToast().add({ title: '没有权限', description: forbiddenMsg, color: 'orange', timeout: 2000 })
        }
        return { code: 0, msg: forbiddenMsg, data: null } as any as Response<T>
    }
    return null
}

export const postRequest = async <T>(url: string, body: object | FormData, options?: { credentials?: RequestCredentials; silent?: boolean; signal?: AbortSignal }) => {
    const BASE_API = useRuntimeConfig().public.baseApi || '/api';
    const userStore = useUserStore();
    const token = userStore.token || "null";

    try {
        const isFormData = body instanceof FormData;
        const headers: Record<string, string> = {
            'Authorization': `Bearer ${token}`,
            ...(!isFormData ? { 'Content-Type': 'application/json' } : {})
        };

        const response: Response<T> = await $fetch(`${BASE_API}/${url}`, {
            method: 'POST',
            headers,
            body: isFormData ? body : JSON.stringify(body),
            credentials: options?.credentials,
            timeout: 8000,
            retry: 0,
            signal: options?.signal
        });
        handleAuthExpiredFromResponse(response as any, options)
        return response;
    } catch (error) {
        const e: any = error;
        const status = e?.response?.status || e?.status;
        const serverMsg = extractServerMsg(e, '网络异常');
        const handled = handleHttpStatusError<T>(status, serverMsg, options)
        if (handled) return handled
        const toast = useToast();
        if (!shouldSuppressToast(options)) {
            toast.add({ title: '请求失败', description: serverMsg || '网络异常或服务器不可用', color: 'red', timeout: 2000 });
        }
        return { code: 0, msg: serverMsg, data: null } as any as Response<T>;
    }
};

export const getRequest = async <T>(url: string, params?: any, options?: { credentials?: RequestCredentials; silent?: boolean; signal?: AbortSignal }) => {
    const BASE_API = useRuntimeConfig().public.baseApi || '/api';
    const userStore = useUserStore();
    const token = userStore.token || "null";

    try {
        const queryParamString = params ? "?" + Object.keys(params).map(key => key + "=" + params[key]).join("&") : "";
        
        const response: Response<T> = await $fetch(`${BASE_API}/${url}${queryParamString}`, {
            method: 'GET',
            headers: {
                'Authorization': `Bearer ${token}`,
                'Cache-Control': 'no-cache',
                'Pragma': 'no-cache'
            },
            credentials: options?.credentials,
            timeout: 8000,
            retry: 0,
            signal: options?.signal
        });
        handleAuthExpiredFromResponse(response as any, options)
        return response;
    } catch (error) {
        const e: any = error;
        const status = e?.response?.status || e?.status;
        const serverMsg = extractServerMsg(e, '网络异常');
        const handled = handleHttpStatusError<T>(status, serverMsg, options)
        if (handled) return handled
        const toast = useToast();
        if (!shouldSuppressToast(options)) {
            toast.add({ title: '请求失败', description: serverMsg || '网络异常或服务器不可用', color: 'red', timeout: 2000 });
        }
        return { code: 0, msg: serverMsg, data: null } as any as Response<T>;
    }
};

export const putRequest = async <T>(url: string, body: object, options?: { credentials?: RequestCredentials; silent?: boolean; signal?: AbortSignal }) => {
    const BASE_API = useRuntimeConfig().public.baseApi || '/api';
    const toast = useToast();
    const userStore = useUserStore();
    const token = userStore.token || "null";

    try {
        const response: Response<T> = await $fetch(`${BASE_API}/${url}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`,
            },
            body: JSON.stringify(body),
            credentials: options?.credentials,
            timeout: 8000,
            retry: 0,
            signal: options?.signal
        });

        handleAuthExpiredFromResponse(response as any, options)

        if (response.code !== 1) {
            if (!options || !(options as any).silent) {
                toast.add({
                    title: "请求失败",
                    description: response.msg,
                    icon: "i-fluent-error-circle-16-filled",
                    color: "red",
                    timeout: 2000,
                });
            }
            return null;
        }

        return response;
    } catch (error) {
        const e: any = error;
        const status = e?.response?.status || e?.status;
        const serverMsg = extractServerMsg(e, '网络异常');
        const handled = handleHttpStatusError<T>(status, serverMsg, options)
        if (handled) return handled
        const toast = useToast();
        if (!shouldSuppressToast(options)) {
            toast.add({ title: '请求失败', description: '网络异常或服务器不可用', color: 'red', timeout: 2000 });
        }
        return { code: 0, msg: '网络异常', data: null } as any as Response<T>;
    }
};

export const deleteRequest = async <T>(url: string, params?: any, options?: { credentials?: RequestCredentials; silent?: boolean; signal?: AbortSignal }) => {
    const BASE_API = useRuntimeConfig().public.baseApi || '/api';
    const userStore = useUserStore();
    const token = userStore.token || "null";

    try {
        const queryParamString = params ? "?" + Object.keys(params).map(key => key + "=" + params[key]).join("&") : "";
        
        const response: Response<T> = await $fetch(`${BASE_API}/${url}${queryParamString}`, {
            method: 'DELETE',
            headers: {
                'Authorization': `Bearer ${token}`,
            },
            credentials: options?.credentials,
            signal: options?.signal
        });
        handleAuthExpiredFromResponse(response as any, options)
        return response;
    } catch (error) {
        const e: any = error;
        const status = e?.response?.status || e?.status;
        const serverMsg = extractServerMsg(e, '网络异常');
        const handled = handleHttpStatusError<T>(status, serverMsg, options)
        if (handled) return handled
        console.error('请求失败:', error);
        throw error;
    }
};
