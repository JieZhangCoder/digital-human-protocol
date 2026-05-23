#!/usr/bin/env node

/**
 * extract-bundled-skills.mjs
 *
 * Cold-start tool: scan every `packages/digital-humans/*​/spec.yaml` for entries
 * declared with `requires.skills[].bundled: true`, group them by skill id, and
 * promote byte-identical copies into standalone skill packages under
 * `packages/skills/<author>/<skill-id>/`. Hosts whose copies disagree with the
 * majority are flagged as conflicts and left untouched.
 *
 * Intended to run ONCE during the DHP v2 migration. See DHP-V2-PLAN.md §7.
 *
 * Modes:
 *   --dry-run (default)  analyze, write extraction-report.md, no mutations
 *   --apply              perform extraction with rollback-per-skill on failure
 *   --author=<name>      default author for promoted skills (default openkursar)
 *   --help
 */

import { createHash } from "crypto";
import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "fs";
import { dirname, join, relative, resolve, sep } from "path";
import { fileURLToPath } from "url";
import { parse as parseYaml, stringify as stringifyYaml } from "yaml";

// ---------------------------------------------------------------------------
// CLI parsing
// ---------------------------------------------------------------------------

const HELP_TEXT = `extract-bundled-skills.mjs — promote bundled skills to standalone packages

Usage:
  node scripts/extract-bundled-skills.mjs [--dry-run | --apply] [--author=<name>]
  node scripts/extract-bundled-skills.mjs --help

Options:
  --dry-run        Analyze only, write extraction-report.md (default)
  --apply          Perform extraction. Requires a fresh --dry-run since the
                   last source change.
  --author=<name>  Default author scope for promoted skills (default: openkursar)
  --help           Show this message
`;

function parseArgs(argv) {
  const args = { dryRun: true, apply: false, author: "openkursar", help: false };
  for (const raw of argv) {
    if (raw === "--help" || raw === "-h") args.help = true;
    else if (raw === "--dry-run") {
      args.dryRun = true;
      args.apply = false;
    } else if (raw === "--apply") {
      args.dryRun = false;
      args.apply = true;
    } else if (raw.startsWith("--author=")) {
      const v = raw.slice("--author=".length).trim();
      if (v) args.author = v;
    } else {
      throw new Error(`Unknown argument: ${raw}`);
    }
  }
  return args;
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(__dirname, "..");
const BUNDLES_DIR = join(REPO_ROOT, "packages", "digital-humans");
const SKILLS_DIR = join(REPO_ROOT, "packages", "skills");
const REPORT_PATH = join(REPO_ROOT, "extraction-report.md");
const LOCK_PATH = join(REPO_ROOT, ".extraction-lock.json");

function toPosix(p) {
  return p.split(sep).join("/");
}

function relRepo(p) {
  return toPosix(relative(REPO_ROOT, p));
}

function log(verb, target) {
  process.stdout.write(`[extract] ${verb} ${target}\n`);
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

/**
 * Discover every bundled skill declaration across all digital-human bundles.
 * Returns a flat list of { hostDir, hostName, specPath, spec, skillEntry, declaredFiles }.
 */
function discoverBundledSkills() {
  const hosts = [];
  if (!existsSync(BUNDLES_DIR)) {
    return hosts;
  }
  const entries = readdirSync(BUNDLES_DIR, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const hostDir = join(BUNDLES_DIR, entry.name);
    const specPath = join(hostDir, "spec.yaml");
    if (!existsSync(specPath)) continue;
    const raw = readFileSync(specPath, "utf8");
    let spec;
    try {
      spec = parseYaml(raw);
    } catch (err) {
      throw new Error(`Failed to parse ${relRepo(specPath)}: ${String(err)}`);
    }
    const skills = spec && spec.requires && spec.requires.skills;
    if (!Array.isArray(skills)) continue;
    for (const sk of skills) {
      if (!sk || typeof sk !== "object" || sk.bundled !== true) continue;
      if (typeof sk.id !== "string" || sk.id.length === 0) continue;
      const declaredFiles = Array.isArray(sk.files)
        ? sk.files.filter((f) => typeof f === "string")
        : [];
      hosts.push({
        hostDir,
        hostName: entry.name,
        specPath,
        spec,
        skillEntry: sk,
        skillId: sk.id,
        declaredFiles,
      });
    }
  }
  return hosts;
}

// ---------------------------------------------------------------------------
// File reading / hashing
// ---------------------------------------------------------------------------

function sha256(buf) {
  return createHash("sha256").update(buf).digest("hex");
}

/**
 * For a given host record, read every file the bundled skill ships.
 * We trust the spec's `files` list as the authoritative manifest; missing
 * files become "[missing]" markers so they show up in conflict reporting.
 */
function readSkillFiles(host) {
  const skillRoot = join(host.hostDir, "skills", host.skillId);
  const files = {};
  for (const relPath of host.declaredFiles) {
    const full = join(skillRoot, relPath);
    if (!existsSync(full) || !statSync(full).isFile()) {
      files[relPath] = { missing: true, hash: null, content: null };
      continue;
    }
    const content = readFileSync(full);
    files[relPath] = { missing: false, hash: sha256(content), content };
  }
  return { skillRoot, files };
}

// ---------------------------------------------------------------------------
// Grouping + content state analysis
// ---------------------------------------------------------------------------

/**
 * Group host records by raw skill id. For each group, compute:
 *   - allFilePaths: union of every declared file path across hosts
 *   - hostHashes: { [hostName]: { [filePath]: hash | null } }
 *   - canonicalHosts: set of hosts whose entire file map matches the majority
 *   - conflictHosts: hosts that differ from the majority
 *   - state: 'identical' | 'partial' | 'conflict'
 *   - canonicalFingerprint: aggregate hash of canonical file set
 *   - canonicalSource: the host whose copy is taken as canonical
 */
function analyzeGroups(hosts) {
  const groups = new Map();
  for (const h of hosts) {
    if (!groups.has(h.skillId)) groups.set(h.skillId, []);
    groups.get(h.skillId).push(h);
  }

  const results = [];
  for (const [skillId, members] of groups) {
    // Read content for each host
    const readPerHost = new Map();
    for (const m of members) {
      readPerHost.set(m.hostName, readSkillFiles(m));
    }

    // Union of file paths
    const allFilePaths = new Set();
    for (const r of readPerHost.values()) {
      for (const p of Object.keys(r.files)) allFilePaths.add(p);
    }

    // Aggregate fingerprint per host = hash over (filePath -> fileHash) sorted
    const hostFingerprints = new Map();
    for (const m of members) {
      const r = readPerHost.get(m.hostName);
      const parts = [];
      for (const p of [...allFilePaths].sort()) {
        const f = r.files[p];
        parts.push(`${p}\0${f ? f.hash || "MISSING" : "MISSING"}`);
      }
      hostFingerprints.set(m.hostName, sha256(Buffer.from(parts.join("\n"))));
    }

    // Tally fingerprints to find the majority
    const tally = new Map();
    for (const fp of hostFingerprints.values()) {
      tally.set(fp, (tally.get(fp) || 0) + 1);
    }
    let majorityFp = null;
    let majorityCount = 0;
    for (const [fp, count] of tally) {
      if (count > majorityCount) {
        majorityFp = fp;
        majorityCount = count;
      }
    }

    const canonicalHosts = members.filter(
      (m) => hostFingerprints.get(m.hostName) === majorityFp
    );
    const conflictHosts = members.filter(
      (m) => hostFingerprints.get(m.hostName) !== majorityFp
    );

    let state;
    if (members.length === 1) {
      state = "single";
    } else if (tally.size === 1) {
      state = "identical";
    } else if (majorityCount >= 2) {
      state = "partial";
    } else {
      // Every host has a unique fingerprint
      state = "conflict";
    }

    const canonicalSource = canonicalHosts[0];
    const canonicalRead = readPerHost.get(canonicalSource.hostName);

    results.push({
      skillId,
      members,
      readPerHost,
      hostFingerprints,
      canonicalHosts,
      conflictHosts,
      canonicalSource,
      canonicalRead,
      state,
      allFilePaths: [...allFilePaths].sort(),
    });
  }
  results.sort((a, b) => a.skillId.localeCompare(b.skillId));
  return results;
}

// ---------------------------------------------------------------------------
// Promotion / spec rewrite
// ---------------------------------------------------------------------------

function promotedSkillDir(author, skillId) {
  return join(SKILLS_DIR, author, skillId);
}

function buildSkillSpec(author, skillId, canonicalHosts) {
  const hostNames = canonicalHosts.map((h) => h.hostName).sort();
  return {
    spec_version: "1",
    name: skillId,
    version: "1.0.0",
    author,
    description: `Extracted from ${hostNames.join(", ")}`,
    type: "skill",
    store: {
      slug: `${author}/${skillId}`,
      category: "other",
      license: "MIT",
    },
  };
}

/**
 * Promote a skill group: copy canonical files into packages/skills/<author>/<id>/,
 * write spec.yaml. Returns the new skill dir path.
 *
 * Idempotent: if the target dir already exists with a spec.yaml, it is treated
 * as already-extracted and the function is a no-op (returns existing dir).
 */
function promoteSkill(group, author) {
  const targetDir = promotedSkillDir(author, group.skillId);
  if (existsSync(join(targetDir, "spec.yaml"))) {
    log("already-promoted", relRepo(targetDir));
    return { targetDir, alreadyExisted: true };
  }

  mkdirSync(targetDir, { recursive: true });
  log("mkdir", relRepo(targetDir));

  // Copy every file from the canonical source directory verbatim.
  const sourceDir = join(group.canonicalSource.hostDir, "skills", group.skillId);
  // Use cpSync recursive — preserves any nested structure declared in `files`.
  cpSync(sourceDir, targetDir, { recursive: true });
  log("copy-dir", `${relRepo(sourceDir)} -> ${relRepo(targetDir)}`);

  const spec = buildSkillSpec(author, group.skillId, group.canonicalHosts);
  const specYaml = stringifyYaml(spec, { lineWidth: 0 });
  writeFileSync(join(targetDir, "spec.yaml"), specYaml, "utf8");
  log("write", relRepo(join(targetDir, "spec.yaml")));

  return { targetDir, alreadyExisted: false };
}

/**
 * Rewrite a host bundle's spec.yaml: replace the bundled-true skill entry
 * for `skillId` with a scoped reference `{ id: "<author>/<skillId>", reason }`.
 * Reason text is preserved if present.
 */
function rewriteHostSpec(host, author) {
  const raw = readFileSync(host.specPath, "utf8");
  const spec = parseYaml(raw);
  const skills = spec && spec.requires && spec.requires.skills;
  if (!Array.isArray(skills)) {
    throw new Error(`${relRepo(host.specPath)}: requires.skills missing on rewrite`);
  }
  let rewritten = false;
  for (let i = 0; i < skills.length; i++) {
    const s = skills[i];
    if (s && typeof s === "object" && s.id === host.skillId && s.bundled === true) {
      const replacement = { id: `${author}/${host.skillId}` };
      if (typeof s.reason === "string" && s.reason.length > 0) {
        replacement.reason = s.reason;
      }
      skills[i] = replacement;
      rewritten = true;
      break;
    }
  }
  if (!rewritten) {
    throw new Error(
      `${relRepo(host.specPath)}: could not locate bundled entry for ${host.skillId}`
    );
  }
  writeFileSync(host.specPath, stringifyYaml(spec, { lineWidth: 0 }), "utf8");
  log("rewrite-spec", relRepo(host.specPath));
}

/**
 * Delete the host's skills/<skillId>/ subdir AFTER spec rewrite. Safety: only
 * called for hosts that matched canonical content, so the contents already
 * live under packages/skills/<author>/<skillId>/.
 */
function deleteHostSkillDir(host) {
  const dir = join(host.hostDir, "skills", host.skillId);
  if (!existsSync(dir)) return;
  rmSync(dir, { recursive: true, force: true });
  log("delete-dir", relRepo(dir));
  // If parent skills/ dir is now empty, prune it too.
  const parent = join(host.hostDir, "skills");
  if (existsSync(parent) && readdirSync(parent).length === 0) {
    rmSync(parent, { recursive: true, force: true });
    log("delete-dir", relRepo(parent));
  }
}

/**
 * Apply extraction for one group with per-skill atomic rollback:
 *   - promote skill
 *   - for each canonical host: rewrite spec + delete dir
 *   - on any failure: revert by deleting the new skill dir and restoring any
 *     rewritten host specs from their pre-image
 */
function applyGroup(group, author) {
  // Idempotency check: if already extracted AND all canonical hosts already
  // reference scoped slug, treat as no-op.
  const targetSpecPath = join(promotedSkillDir(author, group.skillId), "spec.yaml");
  if (existsSync(targetSpecPath)) {
    // Verify each canonical host: does it still have bundled: true?
    const stillBundled = group.canonicalHosts.filter((h) => {
      const raw = readFileSync(h.specPath, "utf8");
      const spec = parseYaml(raw);
      const skills = (spec && spec.requires && spec.requires.skills) || [];
      return skills.some(
        (s) => s && typeof s === "object" && s.id === h.skillId && s.bundled === true
      );
    });
    if (stillBundled.length === 0) {
      log("noop-already-extracted", group.skillId);
      return { promoted: false, alreadyExtracted: true, rewroteHosts: [] };
    }
    // Partial prior state — continue and rewrite the remaining bundled hosts.
  }

  // Snapshot pre-images for rollback.
  const preImages = new Map();
  for (const h of group.canonicalHosts) {
    preImages.set(h.specPath, readFileSync(h.specPath, "utf8"));
  }

  let promotionResult = null;
  const rewroteHosts = [];

  try {
    promotionResult = promoteSkill(group, author);
    for (const h of group.canonicalHosts) {
      rewriteHostSpec(h, author);
      deleteHostSkillDir(h);
      rewroteHosts.push(h);
    }
    return { promoted: true, alreadyExtracted: false, rewroteHosts };
  } catch (err) {
    // Rollback: restore rewritten spec files and remove newly-created skill dir.
    log("rollback-begin", group.skillId);
    for (const [specPath, content] of preImages) {
      try {
        writeFileSync(specPath, content, "utf8");
        log("rollback-spec", relRepo(specPath));
      } catch (e) {
        process.stderr.write(
          `[extract] WARNING failed to restore ${relRepo(specPath)}: ${String(e)}\n`
        );
      }
    }
    if (promotionResult && !promotionResult.alreadyExisted) {
      try {
        rmSync(promotionResult.targetDir, { recursive: true, force: true });
        log("rollback-rmdir", relRepo(promotionResult.targetDir));
      } catch (e) {
        process.stderr.write(
          `[extract] WARNING failed to remove ${relRepo(promotionResult.targetDir)}: ${String(e)}\n`
        );
      }
    }
    throw err;
  }
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

const DIFF_TRUNCATE_LINES = 40;

function truncatedUnifiedDiff(aLabel, aContent, bLabel, bContent) {
  // Minimal hand-rolled diff — line-based, with simple LCS-ish marker. We aren't
  // shelling out to `diff` to keep the script self-contained.
  const aLines = aContent ? aContent.toString("utf8").split("\n") : [];
  const bLines = bContent ? bContent.toString("utf8").split("\n") : [];
  const out = [`--- ${aLabel}`, `+++ ${bLabel}`];
  const max = Math.max(aLines.length, bLines.length);
  for (let i = 0; i < max && out.length < DIFF_TRUNCATE_LINES + 2; i++) {
    const a = aLines[i];
    const b = bLines[i];
    if (a === b) {
      out.push(`  ${a ?? ""}`);
    } else {
      if (a !== undefined) out.push(`- ${a}`);
      if (b !== undefined) out.push(`+ ${b}`);
    }
  }
  if (max > DIFF_TRUNCATE_LINES) out.push("... (truncated)");
  return out.join("\n");
}

function fileLabel(host, path) {
  return `${host.hostName}/skills/${host.skillId}/${path}`;
}

function renderReport(groups, author, mode) {
  const lines = [];
  lines.push("# Bundled Skill Extraction Report");
  lines.push("");
  lines.push(`- Mode: \`${mode}\``);
  lines.push(`- Default author scope: \`${author}\``);
  lines.push(`- Generated: ${new Date().toISOString()}`);
  lines.push("");

  const totalDeclarations = groups.reduce((s, g) => s + g.members.length, 0);
  const uniqueIds = groups.length;
  const auto = groups.filter((g) => g.state === "single" || g.state === "identical" || g.state === "partial");
  const conflicts = groups.filter((g) => g.state === "conflict");
  const partials = groups.filter((g) => g.state === "partial");

  lines.push("## Summary");
  lines.push("");
  lines.push(`- Total bundled skill declarations: **${totalDeclarations}**`);
  lines.push(`- Unique skill ids: **${uniqueIds}**`);
  lines.push(`- Auto-promotable groups: **${auto.length}**`);
  lines.push(`  - Single-host: ${groups.filter((g) => g.state === "single").length}`);
  lines.push(`  - Multi-host identical: ${groups.filter((g) => g.state === "identical").length}`);
  lines.push(`  - Multi-host partial (majority wins, minority left for human): ${partials.length}`);
  lines.push(`- Full conflict groups (no automatic action): **${conflicts.length}**`);
  lines.push("");

  lines.push("## Groups");
  lines.push("");
  lines.push("| Skill id | Hosts | State | Canonical hosts | Conflict hosts | Action |");
  lines.push("| --- | --- | --- | --- | --- | --- |");
  for (const g of groups) {
    const canonical = g.canonicalHosts.map((h) => h.hostName).join(", ") || "—";
    const conflict = g.conflictHosts.map((h) => h.hostName).join(", ") || "—";
    let action;
    if (g.state === "single" || g.state === "identical") {
      action = `promote → \`packages/skills/${author}/${g.skillId}\``;
    } else if (g.state === "partial") {
      action = `promote majority → \`packages/skills/${author}/${g.skillId}\`; leave minority bundled`;
    } else {
      action = "**[conflict]** no automatic action";
    }
    lines.push(
      `| \`${g.skillId}\` | ${g.members.length} | ${g.state} | ${canonical} | ${conflict} | ${action} |`
    );
  }
  lines.push("");

  const needsDiff = groups.filter((g) => g.state === "partial" || g.state === "conflict");
  if (needsDiff.length > 0) {
    lines.push("## Conflicts / Partial diffs");
    lines.push("");
    lines.push(
      "Diffs are truncated to the first 40 lines per file. Resolve conflicts by manually choosing the authoritative version and re-running the script."
    );
    lines.push("");
    for (const g of needsDiff) {
      lines.push(`### \`${g.skillId}\` (${g.state})`);
      lines.push("");
      lines.push(`Canonical: ${g.canonicalHosts.map((h) => `\`${h.hostName}\``).join(", ")}`);
      lines.push(`Conflict: ${g.conflictHosts.map((h) => `\`${h.hostName}\``).join(", ")}`);
      lines.push("");
      const canon = g.canonicalSource;
      const canonRead = g.readPerHost.get(canon.hostName);
      for (const other of g.conflictHosts) {
        const otherRead = g.readPerHost.get(other.hostName);
        const diffPaths = g.allFilePaths.filter((p) => {
          const a = canonRead.files[p];
          const b = otherRead.files[p];
          return (a ? a.hash : null) !== (b ? b.hash : null);
        });
        for (const p of diffPaths) {
          const a = canonRead.files[p];
          const b = otherRead.files[p];
          lines.push("```diff");
          lines.push(
            truncatedUnifiedDiff(
              fileLabel(canon, p),
              a ? a.content : Buffer.from(""),
              fileLabel(other, p),
              b ? b.content : Buffer.from("")
            )
          );
          lines.push("```");
          lines.push("");
        }
      }
    }
  }

  lines.push("## Next steps");
  lines.push("");
  if (mode === "dry-run") {
    lines.push("Re-run with `--apply` to perform the promotions listed above. Conflicts must be resolved manually first.");
  } else {
    lines.push("Extraction applied. Run `npm run build` to regenerate `index.json` and verify nothing broke.");
  }
  lines.push("");

  return lines.join("\n");
}

// ---------------------------------------------------------------------------
// Lockfile heuristic for --apply safety
// ---------------------------------------------------------------------------

/**
 * Compute a coarse source fingerprint: hashes of every spec.yaml under
 * packages/digital-humans/ + every file under existing skills dirs. This is
 * what dry-run records so --apply can refuse if sources have drifted.
 */
function computeSourceFingerprint(hosts) {
  const hash = createHash("sha256");
  const items = [];
  for (const h of hosts) {
    items.push(`SPEC:${relRepo(h.specPath)}`);
    items.push(readFileSync(h.specPath));
    const skillDir = join(h.hostDir, "skills", h.skillId);
    if (existsSync(skillDir)) {
      const walk = [skillDir];
      const collected = [];
      while (walk.length) {
        const d = walk.pop();
        for (const e of readdirSync(d, { withFileTypes: true })) {
          const f = join(d, e.name);
          if (e.isDirectory()) walk.push(f);
          else if (e.isFile()) collected.push(f);
        }
      }
      collected.sort();
      for (const f of collected) {
        items.push(`FILE:${relRepo(f)}`);
        items.push(readFileSync(f));
      }
    }
  }
  for (const item of items) hash.update(typeof item === "string" ? Buffer.from(item) : item);
  return hash.digest("hex");
}

function writeLock(fp) {
  writeFileSync(
    LOCK_PATH,
    JSON.stringify({ fingerprint: fp, dry_run_at: new Date().toISOString() }, null, 2),
    "utf8"
  );
}

function readLock() {
  if (!existsSync(LOCK_PATH)) return null;
  try {
    return JSON.parse(readFileSync(LOCK_PATH, "utf8"));
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    process.stdout.write(HELP_TEXT);
    return;
  }
  const mode = args.apply ? "apply" : "dry-run";
  log("mode", mode);
  log("author", args.author);

  const hosts = discoverBundledSkills();
  log("discovered-hosts", `${hosts.length} bundled skill declarations`);
  const groups = analyzeGroups(hosts);
  log("grouped", `${groups.length} unique skill ids`);

  const fingerprint = computeSourceFingerprint(hosts);

  if (args.apply) {
    const lock = readLock();
    if (!lock || lock.fingerprint !== fingerprint) {
      process.stderr.write(
        "[extract] refusing --apply: no fresh dry-run for current source state.\n" +
          "  Run `node scripts/extract-bundled-skills.mjs --dry-run` first, review extraction-report.md, then retry --apply.\n"
      );
      process.exit(2);
    }
    log("lock", "verified");
  }

  if (args.apply) {
    let promotedCount = 0;
    let noopCount = 0;
    let conflictSkipped = 0;
    for (const g of groups) {
      if (g.state === "conflict") {
        log("skip-conflict", g.skillId);
        conflictSkipped++;
        continue;
      }
      try {
        const result = applyGroup(g, args.author);
        if (result.alreadyExtracted) noopCount++;
        else if (result.promoted) promotedCount++;
      } catch (err) {
        process.stderr.write(
          `[extract] ERROR extracting ${g.skillId}: ${String(err)}\n  (rollback for this skill completed; other groups unaffected)\n`
        );
      }
    }
    log("done", `promoted=${promotedCount} noop=${noopCount} conflict-skipped=${conflictSkipped}`);
    // After mutation, fingerprint is stale; clear the lock so a fresh dry-run is required for any subsequent apply.
    if (existsSync(LOCK_PATH)) {
      rmSync(LOCK_PATH, { force: true });
      log("lock", "cleared (state mutated)");
    }
  } else {
    writeLock(fingerprint);
    log("lock", `wrote ${relRepo(LOCK_PATH)}`);
  }

  const report = renderReport(groups, args.author, mode);
  writeFileSync(REPORT_PATH, report, "utf8");
  log("report", relRepo(REPORT_PATH));
}

main();
