import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { queueDecision } from "./queue-next.mjs";

test("Pi queues continue only after a structured verified handoff", () => {
  const fixtures = JSON.parse(readFileSync(new URL("./queue-outcomes.json", import.meta.url)));
  for (const fixture of fixtures) {
    assert.equal(queueDecision(fixture.stage, fixture.result, fixture.completed, fixture.limit), fixture.want, fixture.name);
  }
});
