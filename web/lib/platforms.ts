// 平台注册表：各奶茶平台的画布规格、上传上限与能力差异。
// id 与 Go 后端路由 /api/{platform}/... 的平台标识一致。
import { HEYTEA_CAPTCHA_APP_ID } from "@/lib/captcha";

export type PlatformId = "heytea" | "nayuki";

export interface FaqItem {
  q: string;
  a: string[];
}

export interface Platform {
  id: PlatformId;
  name: string;
  // 上传走的官方通道，展示在页头
  channel: string;
  // 杯贴画布像素尺寸
  width: number;
  height: number;
  // 导出产物的体积上限（字节）
  maxBytes: number;
  // 杯贴底色：近白/透明区域与橡皮擦孔洞都合成到该色
  background: string;
  // 官方链路只上传 PNG：不退 JPEG
  pngOnly: boolean;
  // 是否支持存草稿
  draft: boolean;
  // 短信登录（腾讯验证码 appId 与提示）；null 表示只能手动粘贴 token
  login: { captchaAppId: string; hint: string } | null;
  tokenPlaceholder: string;
  // 确认直接上传时的提示
  uploadNotice: string;
  faq: FaqItem[];
}

export const PLATFORMS: Record<PlatformId, Platform> = {
  heytea: {
    id: "heytea",
    name: "喜茶",
    channel: "App 通道",
    width: 596,
    height: 832,
    maxBytes: 200 * 1024,
    background: "#EEEEEE",
    pngOnly: false,
    draft: true,
    login: {
      captchaAppId: HEYTEA_CAPTCHA_APP_ID,
      hint: "短信每日有发送上限，请勿频繁获取；登录会使手机上的喜茶 GO App 下线（单端会话）",
    },
    tokenPlaceholder: "粘贴 App 通道 token（抓包获取，见下方常见问题）",
    uploadNotice: "直接上传会立即发布到你的喜茶账号，日常更推荐「存为草稿」。",
    faq: [
      {
        q: "1. 怎么获取 token？",
        a: [
          "本工具不做登录（App 登录走原生反滥用通道，无法在本机复现）。用手机抓包提取：手机 WiFi 代理指向本机 mitmproxy（默认 :8898），在喜茶 App 退出登录后重新短信登录，抓包请求头里的 Authorization 即为 token，有效期约 15 天。",
        ],
      },
      {
        q: "2. 上传失败，提示“文件格式不允许上传”/“文件大小不符合要求”。",
        a: ["工具会自动压缩到 200KB 内，但不排除失败可能，可更换 PNG 文件或更小的图片后重试。"],
      },
      {
        q: "3. 上传成功后小程序不显示",
        a: ["确认上传工具使用的账号和喜茶登录账号保持一致，随后刷新喜茶小程序。"],
      },
      {
        q: "4. 其他上传失败",
        a: [
          "确认今日内上传未超过 10 张，并检查喜茶小程序是否能正常打开上传界面并制作喜贴。",
          "部分浏览器不支持现代 Web API，可更换 Chrome 或 Safari 后重试上传。",
        ],
      },
    ],
  },
  nayuki: {
    id: "nayuki",
    name: "奈雪的茶",
    channel: "小程序通道",
    // 小程序画布 308×377，按 3 倍像素比导出，与官方上传的图片尺寸一致
    width: 924,
    height: 1131,
    // 官方链路不限体积（OSS policy 上限 1GB），取 1MB 兼顾画质与上传速度
    maxBytes: 1024 * 1024,
    background: "#FFFFFF",
    pngOnly: true,
    draft: false,
    login: null,
    tokenPlaceholder: "粘贴小程序请求头 authorization 的值（抓包获取，见下方常见问题）",
    uploadNotice: "上传会立即提交为你的奈雪杯贴作品，作品需经平台审核。",
    faq: [
      {
        q: "1. 怎么获取 token？",
        a: [
          "小程序登录依赖微信授权，无法在本机复现。用手机抓包奈雪点单小程序（域名 tm-api.pin-dao.cn），任一请求头里的 authorization 即为 token，带不带 Bearer 前缀都可以，有效期约 120 天。",
        ],
      },
      {
        q: "2. 上传后在哪里看？",
        a: ["作品会提交到小程序当前的杯贴活动并进入审核，请在奈雪点单小程序的杯贴页面查看。"],
      },
    ],
  },
};

export const PLATFORM_LIST: Platform[] = [PLATFORMS.heytea, PLATFORMS.nayuki];

export function isPlatformId(value: string): value is PlatformId {
  return Object.prototype.hasOwnProperty.call(PLATFORMS, value);
}

// 体积上限的展示文案：200KB / 1MB
export function formatLimit(bytes: number): string {
  return bytes >= 1024 * 1024 ? `${bytes / (1024 * 1024)}MB` : `${Math.round(bytes / 1024)}KB`;
}
