import { test as base, expect, type BrowserContext, type Locator, type Page, type TestInfo } from "@playwright/test";
import { languageStorageKey, sessionStorageKey } from "../../features/admin/core/types";
import configuration from "./config.cjs";
import { fixedTime, shellResponses, user } from "./fixtures/shell";
import { MockAPI } from "./network";
const { apiOrigin } = configuration;

// bootConsole is the single factory behind every harness variant: it
// installs the shell fixtures, fixes time and motion, seeds the requested
// console session (null boots the signed-out surface), and fails the
// scenario on unexpected page errors.
async function bootConsole(
  context: BrowserContext,
  page: Page,
  runScenario: (api: MockAPI) => Promise<void>,
  sessionUser: typeof user | null,
) {
  const api = new MockAPI();
  for (const [key, json] of shellResponses()) {
    const [method, pathname] = key.split(" ");
    api.respond(method, pathname, json);
  }
  await api.install(context);
  await page.clock.setFixedTime(new Date(fixedTime));
  await page.emulateMedia({ reducedMotion: "reduce" });
  const session = sessionUser
    ? { baseURL: apiOrigin, token: "ui-fixture-session", user: sessionUser, expiresAt: "2099-01-01T00:00:00Z" }
    : null;
  await context.addInitScript(({ sessionKey, languageKey, session }) => {
    if (session) {
      window.sessionStorage.setItem(sessionKey, JSON.stringify(session));
    }
    window.localStorage.setItem(languageKey, "zh-CN");
  }, { sessionKey: sessionStorageKey, languageKey: languageStorageKey, session });
  const pageErrors: string[] = [];
  context.on("page", opened => opened.on("pageerror", error => pageErrors.push(error.message)));
  page.on("pageerror", error => pageErrors.push(error.message));
  try {
    await runScenario(api);
  } finally {
    // Keep interception installed until every page and its requests are closed.
    await context.close();
    api.assertClean();
    expect(pageErrors, "Unexpected application errors").toEqual([]);
  }
}

type ConsoleFixture = [
  ({ context, page }: { context: BrowserContext; page: Page }, runScenario: (api: MockAPI) => Promise<void>) => Promise<void>,
  { auto: true },
];

function consoleFixture(sessionUser: typeof user | null): ConsoleFixture {
  return [async ({ context, page }, runScenario) => {
    await bootConsole(context, page, runScenario, sessionUser);
  }, { auto: true }];
}

export const test = base.extend<{ api: MockAPI }>({ api: consoleFixture(user) });

// teacherTest seeds a team-leader session so scenarios can exercise the
// teacher workspace without touching the shared admin fixture.
export const teacherUser = { ...user, id: "usr_ui_teacher", username: "ui-teacher", name: "UI Review Teacher", email: "ui-teacher@example.test", role: "team_leader", team_id: "team_ui" };
export const teacherTest = base.extend<{ api: MockAPI }>({ api: consoleFixture(teacherUser) });

// publicTest boots without a seeded console session so scenarios can cover
// the sign-in and registration surfaces.
export const publicTest = base.extend<{ api: MockAPI }>({ api: consoleFixture(null) });

export { expect };

export function section(page: Page, title: string): Locator {
  return page.locator("section.section").filter({ has: page.getByRole("heading", { name: title, exact: true }) });
}
export async function openBilling(page: Page) {
  await page.goto("/billing");
  await expect(page.locator(".app-shell")).toBeVisible();
  await expect(section(page, "费用对账单").getByRole("option", { name: "UI Review Project (prj_ui)", exact: true })).toBeAttached();
}
export async function capture(page: Page, testInfo: TestInfo, subject: Locator, id: string, title: string, mode: "section" | "viewport" = "section") {
  testInfo.annotations.push({ type: "ui-capture", description: JSON.stringify({ id, title }) });
  if (process.env.TOKENHUB_UI_CAPTURE !== "1") return;
  await page.evaluate(() => document.fonts.ready);
  const image = testInfo.outputPath(`${id}.png`);
  if (mode === "viewport") {
    await page.screenshot({ path: image, fullPage: false, animations: "disabled", caret: "hide" });
  } else {
    const bounds = await subject.boundingBox();
    if (!bounds || bounds.height > page.viewportSize()!.height) {
      throw new Error("Capture long scrollable sections as named viewport segments instead of a clipped element image.");
    }
    await subject.scrollIntoViewIfNeeded();
    await subject.screenshot({ path: image, animations: "disabled", caret: "hide" });
  }
  await testInfo.attach(id, { path: image, contentType: "image/png" });
}
