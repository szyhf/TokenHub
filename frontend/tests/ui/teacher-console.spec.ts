import type { AdminResource, Provider } from "../../features/admin/core/types";
import { teacherTest, expect, capture } from "./harness";

teacherTest("teacher-console provider ownership scope", async ({ page, api }, testInfo) => {
  const providers: Provider[] = [
    { id: "prv_ui_teacher", name: "UI Teacher Channel", type: "mock", base_url: "https://teacher-upstream.example.test/v1", priority: 1, status: "active", healthy: true, owner_team_id: "team_ui" },
  ];
  api.respond("GET", "/api/admin/providers", { data: providers });
  api.respond("GET", "/api/admin/provider-catalog", { data: [{ id: "local", name: "Mock Catalog", display_name: "Mock Provider", type: "mock", models_count: 0, source: "plugin" }] });
  api.respond("GET", "/api/admin/provider-catalog/local", { data: { id: "local", name: "Local Cluster", type: "mock", models_count: 0, models: [], source: "plugin" } });
  api.respond("GET", "/api/admin/resources/teams", { data: [{ id: "team_ui", kind: "teams", name: "UI Teacher Team", status: "active" } satisfies AdminResource] });
  for (const path of ["provider-resources", "routing-rules", "audit/events", "providers/monitoring"]) {
    api.respond("GET", `/api/admin/${path}`, { data: [] });
  }
  api.define("GET", "/api/admin/audit/requests", () => ({ json: {
    data: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 0 },
    summary: { all: 0, ok: 0, error: 0, average_latency_ms: 0 },
  } }), query => {
    expect(Object.fromEntries(query)).toEqual({ page: "1", page_size: "20", status: "all", q: "" });
  });

  await page.goto("/providers");
  const row = page.getByRole("row").filter({ hasText: "UI Teacher Channel" });
  await expect(row.getByText("团队渠道 · UI Teacher Team", { exact: true })).toBeVisible();
  await expect(page.locator(".app-shell")).toBeVisible();
  await capture(page, testInfo, page.locator(".provider-channel-list"), "teacher-providers-ownership", "老师渠道列表显示团队归属");

  // The teacher workspace must not touch admin-only plugin endpoints; the
  // strict harness fails the scenario on any undeclared request.
  expect(api.calls.filter(call => call.path.startsWith("/api/admin/plugins") || call.path === "/api/admin/plugin-marketplace")).toEqual([]);

  await page.goto("/routes");
  await expect(page.locator(".app-shell")).toBeVisible();
  await capture(page, testInfo, page.locator(".app-shell"), "teacher-routes-workspace", "老师路由策略工作台", "viewport");
});

teacherTest("teacher-console billing shows tenant statement only", async ({ page, api }, testInfo) => {
  api.respond("GET", "/api/admin/users", { data: [] });
  await page.goto("/billing");
  const statement = page.locator("section.section").filter({ has: page.getByRole("heading", { name: "费用对账单", exact: true }) });
  await expect(statement).toBeVisible();
  // The single-side teacher statement hides the side switcher entirely.
  await expect(statement.getByLabel("对账单类型")).toHaveCount(0);
  await expect(statement.getByLabel("客户项目（可多选）")).toBeVisible();
  await expect(statement.getByRole("button", { name: "预览对账单", exact: true })).toBeVisible();
  await capture(page, testInfo, statement, "teacher-billing-tenant-statement", "老师租户侧费用对账单");
});
