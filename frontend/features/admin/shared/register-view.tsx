"use client";

import { Eye, EyeOff } from "lucide-react";
import { useState, type FormEvent } from "react";
import { tx } from "../i18n/runtime";

type RegisterOutcome =
  | { ok: true }
  | { ok: false; message: string };

async function submitRegistration(baseURL: string, payload: Record<string, string>): Promise<RegisterOutcome> {
  try {
    const resp = await fetch(`${baseURL.replace(/\/$/, "")}/api/admin/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (resp.ok) {
      return { ok: true };
    }
    const body = await resp.json().catch(() => null);
    const message = body?.error?.message;
    return { ok: false, message: typeof message === "string" && message ? message : tx("注册请求失败") };
  } catch {
    return { ok: false, message: tx("注册请求失败") };
  }
}

export function RegisterView({
  baseURL,
  busy,
  onBack,
  onRegistered,
}: {
  baseURL: string;
  busy: boolean;
  onBack: () => void;
  onRegistered: (username: string) => void;
}) {
  const [username, setUsername] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting || busy) {
      return;
    }
    if (password !== confirmPassword) {
      setError(tx("两次输入的密码不一致"));
      return;
    }
    setSubmitting(true);
    setError("");
    const outcome = await submitRegistration(baseURL, {
      username: username.trim(),
      name: name.trim(),
      email: email.trim(),
      password,
      invite_code: inviteCode.trim(),
    });
    setSubmitting(false);
    if (outcome.ok) {
      onRegistered(username.trim());
      return;
    }
    setError(outcome.message);
  }

  return (
    <form aria-label={tx("注册老师账号")} className="login-form" onSubmit={submit}>
      <h2>{tx("注册老师账号")}</h2>
      <label>
        <span>{tx("邀请码")}</span>
        <input
          aria-label={tx("邀请码")}
          autoComplete="off"
          onChange={(event) => setInviteCode(event.target.value)}
          required
          type="text"
          value={inviteCode}
        />
      </label>
      <label>
        <span>{tx("用户名")}</span>
        <input
          aria-label={tx("用户名")}
          autoComplete="username"
          onChange={(event) => setUsername(event.target.value)}
          required
          type="text"
          value={username}
        />
      </label>
      <label>
        <span>{tx("姓名（可选）")}</span>
        <input
          aria-label={tx("姓名（可选）")}
          autoComplete="name"
          onChange={(event) => setName(event.target.value)}
          type="text"
          value={name}
        />
      </label>
      <label>
        <span>{tx("邮箱")}</span>
        <input
          aria-label={tx("邮箱")}
          autoComplete="email"
          onChange={(event) => setEmail(event.target.value)}
          required
          type="email"
          value={email}
        />
      </label>
      <label>
        <span>{tx("设置密码")}</span>
        <span className="password-field">
          <input
            aria-label={tx("设置密码")}
            autoComplete="new-password"
            minLength={10}
            onChange={(event) => setPassword(event.target.value)}
            required
            type={passwordVisible ? "text" : "password"}
            value={password}
          />
          <button
            aria-label={passwordVisible ? tx("隐藏密码") : tx("显示密码")}
            className="password-toggle"
            onClick={() => setPasswordVisible((value) => !value)}
            type="button"
          >
            {passwordVisible ? <EyeOff size={16} /> : <Eye size={16} />}
          </button>
        </span>
      </label>
      <label>
        <span>{tx("确认密码")}</span>
        <input
          aria-label={tx("确认密码")}
          autoComplete="new-password"
          minLength={10}
          onChange={(event) => setConfirmPassword(event.target.value)}
          required
          type={passwordVisible ? "text" : "password"}
          value={confirmPassword}
        />
      </label>
      {error ? <div className="login-error">{error}</div> : null}
      <button className="button login-submit" disabled={submitting || busy} type="submit">
        {submitting ? tx("注册中") : tx("创建账号")}
      </button>
      <div className="login-helper-row">
        <span />
        <button onClick={onBack} type="button">{tx("返回登录")}</button>
      </div>
    </form>
  );
}
