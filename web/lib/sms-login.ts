// 手机号短信登录流程：网关在发短信接口强制人机校验（缺 ticket 时返回 4005021），
// 所以滑块在发短信前触发，ticket/randstr 随短信请求消费；登录接口不再需要 ticket。
// 因此登录失败（如验证码输错）改正后再点登录即可，短信未过期就不必重发——
// 短信每日有发送上限，要省着用。
import type { User } from "./api";

export const PHONE_PATTERN = /^1\d{10}$/;

export interface SmsLoginDeps {
  runCaptcha(): Promise<{ ticket: string; randstr: string }>;
  requestLoginSms(phone: string, ticket: string, randstr: string): Promise<void>;
  loginByPhone(phone: string, code: string): Promise<{ token: string; user: User }>;
}

// sendLoginSms 先过滑块，再带 ticket/randstr 发送短信验证码。
export async function sendLoginSms(deps: SmsLoginDeps, phone: string): Promise<void> {
  if (!PHONE_PATTERN.test(phone)) throw new Error("请输入 11 位手机号");
  const captcha = await deps.runCaptcha();
  await deps.requestLoginSms(phone, captcha.ticket, captcha.randstr);
}

// loginWithSmsCode 用短信验证码登录；人机验证已在发短信时完成，这里不再过滑块。
export async function loginWithSmsCode(
  deps: SmsLoginDeps,
  phone: string,
  code: string,
): Promise<{ token: string; user: User }> {
  if (!PHONE_PATTERN.test(phone)) throw new Error("请输入 11 位手机号");
  if (!code) throw new Error("请输入短信验证码");
  return deps.loginByPhone(phone, code);
}

// 回执里确认发送目标，138****8000
export function maskPhone(phone: string): string {
  return phone.length === 11 ? `${phone.slice(0, 3)}****${phone.slice(7)}` : phone;
}
