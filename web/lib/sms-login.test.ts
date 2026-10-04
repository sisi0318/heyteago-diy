import { describe, expect, it, vi } from "vitest";
import { loginWithSmsCode, maskPhone, sendLoginSms, type SmsLoginDeps } from "./sms-login";

function makeDeps(overrides?: Partial<SmsLoginDeps>) {
  return {
    runCaptcha: vi.fn(async () => ({ ticket: "cap-1", randstr: "rand-1" })),
    requestLoginSms: vi.fn(async () => {}),
    loginByPhone: vi.fn(async () => ({ token: "tok", user: { id: "1", name: "n" } })),
    ...overrides,
  };
}

describe("sendLoginSms", () => {
  // 网关在发短信接口强制人机校验：先过滑块，ticket/randstr 随短信请求发出
  it("先过滑块再发短信，ticket 与 randstr 透传", async () => {
    const deps = makeDeps();
    await sendLoginSms(deps, "13800138000");
    expect(deps.runCaptcha).toHaveBeenCalledOnce();
    expect(deps.requestLoginSms).toHaveBeenCalledWith("13800138000", "cap-1", "rand-1");
  });

  it("手机号不合法时不弹滑块也不发请求", async () => {
    const deps = makeDeps();
    await expect(sendLoginSms(deps, "123")).rejects.toThrow("11 位手机号");
    expect(deps.runCaptcha).not.toHaveBeenCalled();
    expect(deps.requestLoginSms).not.toHaveBeenCalled();
  });

  it("取消滑块时不发短信", async () => {
    const deps = makeDeps({
      runCaptcha: vi.fn(async () => {
        throw new Error("已取消人机验证");
      }),
    });
    await expect(sendLoginSms(deps, "13800138000")).rejects.toThrow("已取消人机验证");
    expect(deps.requestLoginSms).not.toHaveBeenCalled();
  });
});

describe("loginWithSmsCode", () => {
  it("用短信验证码登录，不再过滑块", async () => {
    const deps = makeDeps();
    const out = await loginWithSmsCode(deps, "13800138000", "123456");
    expect(out.token).toBe("tok");
    expect(deps.loginByPhone).toHaveBeenCalledWith("13800138000", "123456");
    expect(deps.runCaptcha).not.toHaveBeenCalled();
  });

  // 验证码输错后改正重试：不需要重新滑块，也不需要重发短信
  it("失败后直接重试，不重发短信", async () => {
    const deps = makeDeps({
      loginByPhone: vi
        .fn()
        .mockRejectedValueOnce(new Error("验证码错误"))
        .mockResolvedValue({ token: "tok", user: { id: "1", name: "n" } }),
    });
    await expect(loginWithSmsCode(deps, "13800138000", "000000")).rejects.toThrow("验证码错误");
    const out = await loginWithSmsCode(deps, "13800138000", "123456");
    expect(out.token).toBe("tok");
    expect(deps.requestLoginSms).not.toHaveBeenCalled();
    expect(deps.runCaptcha).not.toHaveBeenCalled();
  });

  it("缺验证码时不发请求", async () => {
    const deps = makeDeps();
    await expect(loginWithSmsCode(deps, "13800138000", "")).rejects.toThrow("请输入短信验证码");
    expect(deps.loginByPhone).not.toHaveBeenCalled();
  });
});

describe("maskPhone", () => {
  it("11 位号码打码", () => {
    expect(maskPhone("13800138000")).toBe("138****8000");
  });
  it("非 11 位原样返回", () => {
    expect(maskPhone("123")).toBe("123");
  });
});
