// skl-owned: skl.pi/v1
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

export function queueDecision(stage, result, completed, limit) {
  const allowed = stage === "implement" ? ["awaiting_review", "needs_human"]
    : stage === "watchdog" ? ["ready_for_merge", "rework", "needs_human"] : [];
  const item = result?.item;
  if (!Number.isInteger(completed) || completed < 1 || !Number.isInteger(limit) || completed >= limit
    || !allowed.includes(result?.status) || !Number.isInteger(item?.Number) || item.Number <= 0
    || item.Claimed !== false || item.State !== result.status || item.Problem
    || (result.status !== "needs_human" && (!item.Submission?.Number || !item.Submission.Head))) return "stop";
  return "continue";
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  let decision = "stop";
  try {
    const [stage, completed, limit, file] = process.argv.slice(2);
    if (process.argv.length === 6) decision = queueDecision(stage, JSON.parse(readFileSync(file, "utf8")), Number(completed), Number(limit));
  } catch { /* Ambiguous worker results stop the queue. */ }
  process.stdout.write(`${decision}\n`);
}
