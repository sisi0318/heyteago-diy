// 后端 API 客户端。浏览器经 Next 同源代理访问 Go 服务（见 next.config.ts rewrites），
// 路由按平台分组：/api/{platform}/...
import type { PlatformId } from "@/lib/platforms";

export interface User {
  id: string;
  name: string;
}

export interface UploadResult {
  message: string;
  data?: unknown;
}

class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: number,
  ) {
    super(message);
  }
}

async function parseError(resp: Response): Promise<ApiError> {
  let message = `请求失败（HTTP ${resp.status}）`;
  let code: number | undefined;
  try {
    const body = await resp.json();
    if (body?.message) message = body.message;
    if (typeof body?.code === "number") code = body.code;
  } catch {
    // 非 JSON 错误体，保留默认 message
  }
  return new ApiError(message, resp.status, code);
}

export async function fetchUser(platform: PlatformId, token?: string): Promise<User> {
  const resp = await fetch(`/api/${platform}/user`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  });
  if (!resp.ok) throw await parseError(resp);
  const body = await resp.json();
  return body.user as User;
}

// requestLoginSms 发送验证码：网关在该接口强制人机校验，需带上腾讯验证码
// 的 ticket/randstr（在发短信这步消费，登录环节不再需要）。
export async function requestLoginSms(
  platform: PlatformId,
  phone: string,
  ticket: string,
  randstr: string,
): Promise<void> {
  const resp = await fetch(`/api/${platform}/login/sms`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ phone, ticket, randstr }),
  });
  if (!resp.ok) throw await parseError(resp);
}

export async function loginByPhone(
  platform: PlatformId,
  phone: string,
  code: string,
): Promise<{ token: string; user: User }> {
  const resp = await fetch(`/api/${platform}/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ phone, code }),
  });
  if (!resp.ok) throw await parseError(resp);
  return resp.json();
}

export async function uploadSticker(
  platform: PlatformId,
  blob: Blob,
  opts: { token: string; userId: string; width?: number; height?: number },
): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", blob, fileNameFor(blob));
  form.append("token", opts.token);
  form.append("userId", opts.userId);
  if (opts.width) form.append("width", String(opts.width));
  if (opts.height) form.append("height", String(opts.height));

  const resp = await fetch(`/api/${platform}/upload`, { method: "POST", body: form });
  if (!resp.ok) throw await parseError(resp);
  return resp.json();
}

export async function saveDraft(platform: PlatformId, blob: Blob, token: string): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", blob, fileNameFor(blob));
  form.append("token", token);

  const resp = await fetch(`/api/${platform}/draft/save`, { method: "POST", body: form });
  if (!resp.ok) throw await parseError(resp);
  return resp.json();
}

function fileNameFor(blob: Blob): string {
  return blob.type === "image/jpeg" ? "cup.jpg" : "cup.png";
}
