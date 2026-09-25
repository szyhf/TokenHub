import { publicTest, expect, capture } from "./harness";

publicTest("registration invite-code teacher signup", async ({ page, api }, testInfo) => {
  api.respond("GET", "/api/admin/auth/registration-status", { allowed: true });
  api.respond("GET", "/api/admin/auth/identity-providers", { data: [] });
  api.define("POST", "/api/admin/auth/register", () => ({ status: 201, json: { user: { id: "usr_ui_registered", username: "ui-teacher", role: "team_leader", status: "active" } } }), query => {
    expect(Object.fromEntries(query)).toEqual({});
  });

  await page.goto("/");
  const registerEntry = page.getByRole("button", { name: "注册老师账号", exact: true });
  await expect(registerEntry).toBeVisible();
  await registerEntry.click();

  const form = page.locator(".login-form");
  await expect(form.getByLabel("邀请码")).toBeVisible();
  await form.getByLabel("邀请码").fill("classroom-invite");
  await form.getByLabel("用户名").fill("ui-teacher");
  await form.getByLabel("姓名（可选）").fill("UI Teacher");
  await form.getByLabel("邮箱").fill("ui-teacher@example.test");
  await form.getByLabel("设置密码").fill("teacher123456");
  await form.getByLabel("确认密码").fill("teacher123456");
  await capture(page, testInfo, form, "registration-form", "老师注册表单");

  await form.getByRole("button", { name: "创建账号", exact: true }).click();
  await expect(page.getByText("注册成功，请登录")).toBeVisible();
  await expect(page.locator(".login-card").getByLabel("账号 / 邮箱", { exact: false })).toHaveValue("ui-teacher");

  const registerCalls = api.calls.filter(call => call.path === "/api/admin/auth/register");
  expect(registerCalls).toHaveLength(1);
});

publicTest("registration hidden when disabled", async ({ page, api }) => {
  api.respond("GET", "/api/admin/auth/registration-status", { allowed: false });
  api.respond("GET", "/api/admin/auth/identity-providers", { data: [] });

  await page.goto("/");
  await expect(page.locator(".login-card")).toBeVisible();
  await expect(page.getByRole("button", { name: "注册老师账号", exact: true })).toHaveCount(0);
});
