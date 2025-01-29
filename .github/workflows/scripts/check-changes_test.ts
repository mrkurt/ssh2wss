import { assertEquals } from "https://deno.land/std@0.220.1/assert/mod.ts";
import { assertExists } from "https://deno.land/std@0.220.1/assert/mod.ts";

// Import functions to test
const scriptPath = new URL("./check-changes", import.meta.url);
const scriptModule = await import(scriptPath.href);

Deno.test("check-changes script exists", async () => {
  const info = await Deno.stat(scriptPath);
  assertExists(info);
  assertEquals(info.isFile, true);
});

Deno.test("check-changes is executable", async () => {
  const info = await Deno.stat(scriptPath);
  assertExists(info);
  // Check if file has execute permission (mode & 0o111)
  assertEquals(info.mode! & 0o111, 0o111);
});

Deno.test("release notes template exists", async () => {
  const templatePath = new URL("./release-notes-template.md", import.meta.url);
  const info = await Deno.stat(templatePath);
  assertExists(info);
  assertEquals(info.isFile, true);
});

// Add more test cases as needed 