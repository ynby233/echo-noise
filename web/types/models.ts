export interface Message {
    id: number;
    ID?: number;
    content: string;
    username?: string;
    Username?: string;
    user_id?: number;
    userId?: number;
    image_url?: string;
    avatar_url?: string;
    AvatarURL?: string;
    private: boolean;
    created_at: string;
    pinned?: boolean;
    notify?: boolean;
    like_count?: number;
}

export interface MessageToSave {
    username?: string;
    content: string;
    image_url?: string;
    private: boolean;
    notify: boolean;
}

export interface PageQuery {
    page: number;
    pageSize: number;
    authorId?: number;
    username?: string;
}

export interface PageQueryResult {
    total: number;
    items: Message[];
    page?: number;
}

// UserToLogin
export interface UserToLogin {
    username: string;
    password: string;
}

// UserToRegister
export interface UserToRegister {
    username: string;
    password: string;
    captcha: string;
    captcha_id?: string;
}

// User
export interface User {
    userid: number;
    id?: number;
    ID?: number;
    user_id?: number;
    username: string;
    Username?: string;
    is_admin: boolean;
    IsAdmin?: boolean;
    total_messages: number;
    token?: string;
    avatar_url?: string;
    AvatarURL?: string;
    description?: string;
    Description?: string;
    email?: string;
    Email?: string;
    email_verified?: boolean;
    EmailVerified?: boolean;
    github_id?: string | number;
}

export interface Tag {
    name: string;
    count: number;
}

export interface NotifyConfig {
    webhookEnabled: boolean;
    webhookURL: string;
    telegramEnabled: boolean;
    telegramToken: string;
    telegramChatID: string;
    weworkEnabled: boolean;
    weworkKey: string;
    feishuEnabled: boolean;
    feishuWebhook: string;
    feishuSecret: string;
    twitterEnabled: boolean;
    twitterApiKey: string;
    twitterApiSecret: string;
    twitterAccessToken: string;
    twitterAccessTokenSecret: string;
    customHttpEnabled: boolean;
    customHttpUrl: string;
    customHttpMethod: string;
    customHttpHeaders: string;
    customHttpBody: string;
}

export interface UserStatus {
    id?: number;
    user_id?: number;
    username: string;
    is_admin: boolean;
}

export interface Status {
    username: string;
    status: string;
    is_admin: boolean;
    sys_admin_id: number;
    users: UserStatus[];
    total_messages: number;
    total_users?: number;
    total_comments?: number;
    total_replies?: number;
    received_comments?: number;
    received_replies?: number;
    messages?: Message[];  // 添加消息列表字段
    total?: number;        // 添加总数字段
    items?: Message[];     // 添加与后端返回结构匹配的字段
}

export interface Response<T> {
    code: number;
    msg: string;
    data: T;
}
