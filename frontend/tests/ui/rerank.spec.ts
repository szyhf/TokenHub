import type { Provider } from "../../features/admin/core/types";
import { test, expect, capture } from "./harness";

test("providers rerank-settings", async ({ page, api }, testInfo) => {
  const provider: Provider = { id: "prv_ui_rerank", name: "UI Rerank", type: "mock", base_url: "http://inference.example.test/v2", priority: 1, status: "active", healthy: true, options: { embedding_protocol: "tei" } };
  api.respond("GET", "/api/admin/providers", { data: [provider] });
  api.respond("GET", "/api/admin/provider-catalog", { data: [{ id: "local", name: "Mock Catalog", display_name: "Mock Provider", type: "mock", models_count: 0, source: "plugin" }] });
  api.respond("GET", "/api/admin/provider-catalog/local", { data: { id: "local", name: "Local Cluster", type: "mock", models_count: 0, models: [], source: "plugin" } });
  api.respond("GET", "/api/admin/provider-adapters", { data: [{ type: "mock", capabilities: ["rerank"], plugin_id: "tokenhub.provider.mock" }] });
  api.respond("GET", "/api/admin/plugins", { data: [{ id: "tokenhub.provider.mock", name: "Mock Plugin", version: "1.0.0", source: "local_file", kinds: ["provider"], placements: ["gateway_chain"], capabilities: [{ kind: "provider_type", name: "mock" }] }] });
  api.respond("GET", "/api/admin/plugin-marketplace", { data: { available: true, plugins: [] } });
  api.respond("GET", "/api/admin/plugin-background-jobs", { data: [], runs: [] });
  for (const path of ["provider-resources", "routing-rules", "audit/events", "providers/monitoring", "plugin-ui-manifest", "plugin-actions"]) api.respond("GET", `/api/admin/${path}`, { data: [] });
  api.define("GET", "/api/admin/audit/requests", () => ({ json: { data: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 0 }, summary: { all: 0, ok: 0, error: 0, average_latency_ms: 0 } } }), query => { expect(Object.fromEntries(query)).toEqual({ page: "1", page_size: "20", status: "all", q: "" }); });
  api.define("PATCH", "/api/admin/providers/prv_ui_rerank", input => {
    const payload=input.body as {options:Record<string,string>};
    expect(payload.options).toMatchObject({embedding_protocol:"tei",rerank_protocol:"cohere",rerank_path:"/rerank"});
    return {json:{...provider,options:payload.options}};
  });
  await page.goto("/providers");
  await page.getByRole("row").filter({hasText:"UI Rerank"}).getByRole("button",{name:"编辑",exact:true}).click();
  const editor=page.locator(".provider-modal");
  await editor.getByRole("tab",{name:"高级",exact:true}).click();
  await editor.getByText("文本重排配置",{exact:true}).click();
  await editor.getByRole("combobox",{name:"重排协议",exact:true}).selectOption("cohere");
  await editor.getByLabel("重排接口路径",{exact:true}).fill("/rerank");
  await capture(page,testInfo,editor,"rerank-provider-settings","重排渠道协议配置","viewport");
  await editor.getByRole("button",{name:"保存",exact:true}).click();
  await expect(editor).not.toBeVisible();
});
