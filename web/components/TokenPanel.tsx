"use client";

// 账号面板：支持短信登录的平台用手机号 + 验证码登录，手动粘贴 token 的通道保留在折叠块中；
// 只能抓包取 token 的平台（如奈雪小程序）直接展示粘贴框。
// token 只保存在浏览器侧，服务端不存储。
// 勾选"记住"后以明文存 localStorage——界面上如实标注，不暗示加密。
//
// 滑块在点击「发送验证码」时触发（网关在发短信接口强制人机校验）；
// 登录失败只需改正验证码再点登录，短信未过期不必重发。
// 短信/登录/查用户的反馈内联在本面板：全局状态条在 ActionBar，
// 离本面板较远，发送回执放那里容易被忽略。
import { useEffect, useRef, useState } from "react";
import { fetchUser, loginByPhone, requestLoginSms, type User } from "@/lib/api";
import { runCaptcha } from "@/lib/captcha";
import type { Platform } from "@/lib/platforms";
import { loginWithSmsCode, maskPhone, sendLoginSms, type SmsLoginDeps } from "@/lib/sms-login";

// 发送成功后的重发冷却：防连点透支短信每日上限
const SMS_COOLDOWN_SECONDS = 60;

interface Feedback {
  kind: "error" | "success";
  text: string;
}

interface Props {
  platform: Platform;
  token: string;
  remember: boolean;
  user: User | null;
  busy: boolean;
  onTokenChange(token: string, remember: boolean): void;
  onUserChange(user: User | null): void;
}

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

// 抓包复制的往往是整个 authorization 头值，去掉 Bearer 前缀
function normalizeToken(value: string): string {
  return value.trim().replace(/^Bearer\s+/i, "");
}

export function TokenPanel({ platform, token, remember, user, busy, onTokenChange, onUserChange }: Props) {
  const { login } = platform;
  const [loadingUser, setLoadingUser] = useState(false);
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);
  const [sending, setSending] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [loggingIn, setLoggingIn] = useState(false);
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const codeInputRef = useRef<HTMLInputElement | null>(null);

  // 短信登录按当前平台的验证码 appId 与接口路由组装
  const smsDeps: SmsLoginDeps | null = login && {
    runCaptcha: () => runCaptcha(login.captchaAppId),
    requestLoginSms: (mobile, ticket, randstr) => requestLoginSms(platform.id, mobile, ticket, randstr),
    loginByPhone: (mobile, smsCode) => loginByPhone(platform.id, mobile, smsCode),
  };

  // 冷却倒计时逐秒递减；换号不清零——冷却约束的是发送频率，不是某个号码
  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setTimeout(() => setCooldown(cooldown - 1), 1000);
    return () => clearTimeout(timer);
  }, [cooldown]);

  const queryUser = async () => {
    setLoadingUser(true);
    setFeedback(null);
    try {
      onUserChange(await fetchUser(platform.id, token || undefined));
      setFeedback({ kind: "success", text: "用户信息查询成功" });
    } catch (err) {
      onUserChange(null);
      setFeedback({ kind: "error", text: errorText(err, "查询用户失败") });
    } finally {
      setLoadingUser(false);
    }
  };

  const sendSms = async () => {
    if (!smsDeps) return;
    setSending(true);
    setFeedback(null);
    try {
      await sendLoginSms(smsDeps, phone);
      setSent(true);
      // 重发后旧验证码大概率已失效，清空避免误提交
      setCode("");
      setCooldown(SMS_COOLDOWN_SECONDS);
      setFeedback({ kind: "success", text: `验证码已发送至 ${maskPhone(phone)}，请查收` });
      codeInputRef.current?.focus();
    } catch (err) {
      setFeedback({ kind: "error", text: errorText(err, "验证码发送失败") });
    } finally {
      setSending(false);
    }
  };

  const handleLogin = async () => {
    if (!smsDeps || loggingIn) return;
    setLoggingIn(true);
    setFeedback(null);
    try {
      const result = await loginWithSmsCode(smsDeps, phone, code.trim());
      onTokenChange(result.token, remember);
      onUserChange(result.user);
      setCode("");
    } catch (err) {
      // 保留手机号与验证码：改正后再点登录即可，短信未过期不必重发
      setFeedback({ kind: "error", text: errorText(err, "登录失败") });
    } finally {
      setLoggingIn(false);
    }
  };

  const sendLabel = sending
    ? "发送中…"
    : cooldown > 0
      ? `${cooldown}s 后可重发`
      : sent
        ? "重新获取"
        : "发送验证码";

  const feedbackLine = feedback && (
    <p className={`mt-2 text-xs ${feedback.kind === "error" ? "text-red-600" : "text-emerald-600"}`}>
      {feedback.text}
    </p>
  );

  const tokenInput = (
    <textarea
      className="mt-2 w-full rounded-lg border border-neutral-300 p-2 font-mono text-xs focus:border-neutral-500 focus:outline-none"
      rows={3}
      placeholder={platform.tokenPlaceholder}
      value={token}
      onChange={(e) => onTokenChange(normalizeToken(e.target.value), remember)}
    />
  );

  return (
    <section className="rounded-xl border border-neutral-200 bg-white p-4 shadow-sm">
      <h2 className="mb-3 text-sm font-semibold text-neutral-800">{login ? "账号登录" : "账号 token"}</h2>
      {login ? (
        <>
          <div className="flex gap-2">
            <input
              type="tel"
              inputMode="numeric"
              maxLength={11}
              className="w-full min-w-0 flex-1 rounded-lg border border-neutral-300 p-2 text-xs focus:border-neutral-500 focus:outline-none"
              placeholder="手机号"
              value={phone}
              onChange={(e) => {
                setPhone(e.target.value.trim());
                // 换号后原号码的发送回执不再适用
                setSent(false);
                setFeedback(null);
              }}
            />
            <button
              type="button"
              onClick={sendSms}
              disabled={sending || loggingIn || busy || cooldown > 0}
              className="shrink-0 rounded-lg border border-neutral-300 px-3 py-1.5 text-xs text-neutral-700 hover:bg-neutral-100 disabled:opacity-50"
            >
              {sendLabel}
            </button>
          </div>
          <div className="mt-2 flex gap-2">
            <input
              ref={codeInputRef}
              inputMode="numeric"
              maxLength={6}
              className="w-full min-w-0 flex-1 rounded-lg border border-neutral-300 p-2 text-xs focus:border-neutral-500 focus:outline-none"
              placeholder="短信验证码"
              value={code}
              onChange={(e) => setCode(e.target.value.trim())}
              onKeyDown={(e) => {
                if (e.key === "Enter" && phone && code && !loggingIn) void handleLogin();
              }}
            />
            <button
              type="button"
              onClick={handleLogin}
              disabled={!phone || !code || loggingIn || sending || busy}
              className="shrink-0 rounded-lg bg-neutral-900 px-3 py-1.5 text-xs text-white hover:bg-neutral-700 disabled:opacity-50"
            >
              {loggingIn ? "登录中…" : "登录"}
            </button>
          </div>
          {feedbackLine}
          <p className="mt-2 text-xs text-neutral-400">{login.hint}</p>
          <details className="mt-3">
            <summary className="cursor-pointer select-none text-xs text-neutral-500 hover:text-neutral-700">
              手动粘贴 token（抓包获取）
            </summary>
            {tokenInput}
          </details>
        </>
      ) : (
        <>
          {tokenInput}
          {feedbackLine}
        </>
      )}
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={queryUser}
          disabled={loadingUser || busy}
          className="rounded-lg bg-neutral-900 px-3 py-1.5 text-xs text-white hover:bg-neutral-700 disabled:opacity-50"
        >
          {loadingUser ? "查询中…" : "查询用户"}
        </button>
        <label className="flex items-center gap-1.5 text-xs text-neutral-600">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => onTokenChange(token, e.target.checked)}
          />
          记住 token（明文保存在本机浏览器）
        </label>
      </div>
      <div className="mt-3">
        {user ? (
          <div className="flex items-center gap-2.5 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2.5">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-emerald-500 text-xs font-bold text-white">
              ✓
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-emerald-900">
                已登录：{user.name || `${platform.name}用户`}
              </p>
              <p className="text-xs text-emerald-700">
                ID {user.id} · {platform.draft ? "可以存草稿或上传" : "可以上传"}
              </p>
            </div>
          </div>
        ) : (
          <p className="text-xs text-neutral-600">
            {login
              ? "未登录 —— 使用手机号登录，或粘贴 token 后点击「查询用户」"
              : "未登录 —— 粘贴 token 后点击「查询用户」"}
          </p>
        )}
      </div>
    </section>
  );
}
