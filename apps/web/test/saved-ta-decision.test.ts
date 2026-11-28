import assert from "node:assert/strict";
import test from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { SavedTaDecision } from "../components/live/saved-ta-decision.js";

test("a reloaded violation still shows the confirmed deduction and reason", () => {
  const html = renderToStaticMarkup(
    createElement(SavedTaDecision, {
      decision: { verdict: "violation", reason: "The selected line calls a forbidden library." },
      deduction: 3,
    }),
  );
  assert.match(html, /Saved TA decision: violation\. Confirmed deduction: 3\./);
  assert.match(html, /The selected line calls a forbidden library\./);
  assert.doesNotMatch(html, /<input|checked|No deduction chosen/);
});

test("a saved pass never applies the configured deduction", () => {
  const html = renderToStaticMarkup(
    createElement(SavedTaDecision, {
      decision: { verdict: "pass", reason: "The call resolves to the student's own function." },
      deduction: 4,
    }),
  );
  assert.match(html, /Saved TA decision: pass\. Confirmed deduction: 0\./);
  assert.doesNotMatch(html, /Confirmed deduction: 4/);
});

test("an unreviewed finding has no saved decision summary", () => {
  assert.equal(
    renderToStaticMarkup(createElement(SavedTaDecision, { decision: undefined, deduction: 3 })),
    "",
  );
});
