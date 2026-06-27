#!/usr/bin/env node
// play.mjs drives one (target=js/wasm, compat=chrome|firefox) cell
// for `gdnext ci play-cell`. Invoked with --url --browser --report
// --screenshot --timeout (ms). Captures the wasm play-bot's
// `GDNEXT_PLAY_RESULT:<base64>` console line, writes the decoded
// JSON to --report, and takes a page.screenshot() to --screenshot.
//
// Exits 0 on success; 1 if the report line never arrives within the
// timeout, the report can't be decoded, or the browser fails to
// launch. The driver's downstream report-reading logic produces the
// human-readable failure message.

import { promises as fs } from "node:fs";
import process from "node:process";
import playwright from "playwright";

function parseArgs(argv) {
	const out = { headed: false };
	for (let i = 2; i < argv.length; i++) {
		const k = argv[i];
		if (!k?.startsWith("--")) throw new Error(`bad arg ${k}`);
		if (k === "--headed") {
			out.headed = true;
			continue;
		}
		const v = argv[i + 1];
		out[k.slice(2)] = v;
		i++;
	}
	for (const k of ["url", "browser", "report", "screenshot", "timeout"]) {
		if (!(k in out)) throw new Error(`missing --${k}`);
	}
	return out;
}

const args = parseArgs(process.argv);
const timeoutMs = Number(args.timeout);

// Playwright defaults chromium.launch() to the headless-shell build
// since v1.40. We want the full chromium so a regular page renders
// the wasm bundle the same way a desktop chrome would; pass the
// channel explicitly. Firefox has only one build, no channel needed.
const launchers = {
	chrome: { launcher: playwright.chromium, options: { channel: "chromium" } },
	firefox: { launcher: playwright.firefox, options: {} },
};
const entry = launchers[args.browser];
if (!entry) {
	console.error(`play.mjs: unknown --browser=${args.browser} (want chrome|firefox)`);
	process.exit(2);
}

const browser = await entry.launcher.launch({
	headless: !args.headed,
	...entry.options,
});
const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
const page = await context.newPage();

// Wire console + crash capture BEFORE navigate so we don't miss an early
// report (or an early null-function abort).
let reportB64 = null;
let firstPageError = null;
const reportPrefix = "GDNEXT_PLAY_RESULT:";
const reportPromise = new Promise((resolve) => {
	page.on("console", (msg) => {
		const text = msg.text();
		if (text.startsWith(reportPrefix)) {
			reportB64 = text.slice(reportPrefix.length);
			resolve();
		} else {
			// Mirror the page's own console output to the CI log so
			// engine warnings/errors are visible alongside native runs.
			console.log(`[page:${msg.type()}]`, text);
		}
	});
});
// Surface page-level runtime errors (uncaught exceptions, wasm aborts)
// so the report wait can fail fast instead of timing out — but give
// the page a short grace window first so we still wait briefly for a
// trailing report write the page may already have queued.
const crashGraceMs = 2000;
const crashPromise = new Promise((_, reject) => {
	page.on("pageerror", (err) => {
		const msg = err.message || String(err);
		console.error("[page:error]", msg);
		if (!firstPageError) firstPageError = msg;
		setTimeout(() => {
			if (!reportB64) reject(new Error(`page crashed before report: ${msg}`));
		}, crashGraceMs);
	});
});

await page.goto(args.url, { waitUntil: "load", timeout: timeoutMs });

const timeoutPromise = new Promise((_, reject) =>
	setTimeout(() => reject(new Error(`timeout: no GDNEXT_PLAY_RESULT within ${timeoutMs}ms`)), timeoutMs)
);

let exitCode = 0;
try {
	await Promise.race([reportPromise, crashPromise, timeoutPromise]);
	const json = Buffer.from(reportB64, "base64").toString("utf8");
	await fs.writeFile(args.report, json);
	console.log(`==> wrote ${args.report} (${json.length} bytes)`);
} catch (err) {
	console.error(`play.mjs: ${err.message}`);
	exitCode = 1;
}

// Screenshot always: an allow-fail cell still uploads the pre-crash
// frame for the workflow summary. Errors here are non-fatal so a
// dead page doesn't suppress whatever the run did manage to produce.
try {
	await page.screenshot({ path: args.screenshot, fullPage: false });
	console.log(`==> wrote ${args.screenshot}`);
} catch (err) {
	console.error(`play.mjs: screenshot failed: ${err.message}`);
}

await browser.close();
process.exit(exitCode);
