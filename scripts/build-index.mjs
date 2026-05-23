#!/usr/bin/env node

import { createHash } from "crypto";
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from "fs";
import { basename, join, relative, sep } from "path";
import { parse as parseYaml, stringify as stringifyYaml } from "yaml";

const DEFAULT_SOURCE = "https://openkursar.github.io/digital-human-protocol";

// Slug formats:
//   - Unscoped (legacy):  "hn-daily"               → kebab-case, no slash
//   - Scoped (DHP v2+):   "openkursar/xhs-search"  → "<author>/<id>", single slash
//
// Scoped slugs are reserved for type=skill (and standalone skills under
// packages/skills/<author>/<id>/). Digital humans and MCPs continue to use
// unscoped slugs for back-compat with the 33 existing bundles.
const UNSCOPED_SLUG_REGEX = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;
const SCOPED_SLUG_REGEX = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\/[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;
const STRICT_SEMVER_REGEX = /^\d+\.\d+\.\d+$/;

function isValidSlug(slug) {
  return UNSCOPED_SLUG_REGEX.test(slug) || SCOPED_SLUG_REGEX.test(slug);
}

function isScopedSlug(slug) {
  return SCOPED_SLUG_REGEX.test(slug);
}

function toPosixPath(pathValue) {
  return pathValue.split(sep).join("/");
}

function parseArgs(argv) {
  let source = process.env.REGISTRY_SOURCE || DEFAULT_SOURCE;
  let check = false;

  for (const arg of argv) {
    if (arg === "--check") {
      check = true;
      continue;
    }

    if (arg.startsWith("--source=")) {
      const value = arg.slice("--source=".length).trim();
      if (value.length > 0) {
        source = value.replace(/\/+$/, "");
      }
    }
  }

  return { source, check };
}

function parseSpec(raw, relPath) {
  try {
    return parseYaml(raw);
  } catch (error) {
    throw new Error(
      `Failed to parse ${relPath} as YAML: ${String(error)}`
    );
  }
}

/**
 * Skill dependency normalization.
 *
 * DHP v2 adds an optional `version` constraint per dependency. Output shape is
 * always `Array<{id: string, version?: string}>` so consumers can rely on a
 * single contract; legacy string-shorthand inputs are upgraded into objects.
 *
 * Version constraint syntax (validated, not resolved here):
 *   - Exact:        "2.1.0"     → strict semver
 *   - Caret range:  "^2.1" | "^2.1.0"
 *   - Omitted       → consumer treats as "latest"
 */
const CARET_RANGE_REGEX = /^\^\d+\.\d+(?:\.\d+)?$/;

function isValidSkillVersionConstraint(value) {
  if (value === undefined || value === null) return true;
  if (typeof value !== "string") return false;
  return STRICT_SEMVER_REGEX.test(value) || CARET_RANGE_REGEX.test(value);
}

function mapSkillDependencies(skills, specRelPath) {
  if (!Array.isArray(skills)) return undefined;
  const out = [];
  for (const item of skills) {
    if (typeof item === "string" && item.length > 0) {
      out.push({ id: item });
      continue;
    }
    if (item && typeof item === "object" && typeof item.id === "string" && item.id.length > 0) {
      const entry = { id: item.id };
      if (typeof item.version === "string" && item.version.length > 0) {
        if (!isValidSkillVersionConstraint(item.version)) {
          throw new Error(
            `${specRelPath}: requires.skills entry "${item.id}" has invalid version constraint "${item.version}". ` +
              `Expected strict semver ("2.1.0") or caret range ("^2.1" / "^2.1.0").`
          );
        }
        entry.version = item.version;
      }
      out.push(entry);
    }
  }
  return out.length > 0 ? out : undefined;
}

// Kept for back-compat with consumers that expect a string[] of skill ids.
// New consumers should use mapSkillDependencies which preserves version constraints.
function mapSkillDependencyIds(skills) {
  if (!Array.isArray(skills)) return undefined;
  const ids = [];
  for (const item of skills) {
    if (typeof item === "string" && item.length > 0) {
      ids.push(item);
    } else if (item && typeof item === "object" && typeof item.id === "string" && item.id.length > 0) {
      ids.push(item.id);
    }
  }
  return ids.length > 0 ? ids : undefined;
}

function mapMcpDependencyIds(mcps) {
  if (!Array.isArray(mcps)) return undefined;
  const ids = mcps
    .map((item) => (item && typeof item === "object" ? item.id : undefined))
    .filter((id) => typeof id === "string" && id.length > 0);
  return ids.length > 0 ? ids : undefined;
}

/**
 * Extract the i18n summary for the registry index.
 * Only name and description are included per locale — full config_schema
 * overrides are available only in the complete spec.
 *
 * @param {Record<string, unknown> | undefined} i18nBlock - Raw i18n object from spec
 * @returns {Record<string, { name?: string; description?: string }> | undefined}
 */
function extractI18nSummary(i18nBlock) {
  if (!i18nBlock || typeof i18nBlock !== "object") return undefined;

  const result = {};

  for (const [locale, v] of Object.entries(i18nBlock)) {
    if (!v || typeof v !== "object") continue;

    const entry = {};
    if (typeof v.name === "string" && v.name.length > 0) {
      entry.name = v.name;
    }
    if (typeof v.description === "string" && v.description.length > 0) {
      entry.description = v.description;
    }

    if (Object.keys(entry).length > 0) {
      result[locale] = entry;
    }
  }

  return Object.keys(result).length > 0 ? result : undefined;
}

function collectBundleFiles(bundleDir) {
  const files = [];
  const stack = [bundleDir];

  while (stack.length > 0) {
    const dir = stack.pop();
    if (!dir) continue;

    const entries = readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = join(dir, entry.name);
      if (entry.isDirectory()) {
        stack.push(fullPath);
      } else if (entry.isFile()) {
        files.push(fullPath);
      }
    }
  }

  return files.sort();
}

/**
 * Recursively list all files under a skill directory, returning POSIX-style
 * paths relative to that directory (e.g. ["SKILL.md", "index.js", "lib/utils.js"]).
 */
function collectSkillFiles(skillDir) {
  const files = [];
  const stack = [skillDir];

  while (stack.length > 0) {
    const dir = stack.pop();
    if (!dir) continue;

    const entries = readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = join(dir, entry.name);
      if (entry.isDirectory()) {
        stack.push(fullPath);
      } else if (entry.isFile()) {
        files.push(toPosixPath(relative(skillDir, fullPath)));
      }
    }
  }

  return files.sort();
}

/**
 * For every skill declared with `bundled: true` in a spec, scan the corresponding
 * `skills/{id}/` directory and update the `files` field in-place.
 *
 * Writes the updated YAML back to disk only when the file list has changed.
 * Throws on missing or empty skill directories so CI catches authoring mistakes early.
 *
 * @param {object} spec - Parsed spec object (mutated in-place)
 * @param {string} bundleDir - Absolute path to the bundle directory
 * @param {string} specPath - Absolute path to spec.yaml (for writing)
 * @param {string} specRelPath - Repo-relative path (for error messages)
 * @returns {boolean} true if spec.yaml was rewritten
 */
function syncBundledSkillFiles(spec, bundleDir, specPath, specRelPath) {
  const skills = spec.requires && spec.requires.skills;
  if (!Array.isArray(skills)) return false;

  let changed = false;

  for (const skill of skills) {
    if (typeof skill !== "object" || !skill || !skill.bundled) continue;

    const skillId = skill.id;
    const skillDir = join(bundleDir, "skills", skillId);

    if (!existsSync(skillDir)) {
      throw new Error(
        `${specRelPath}: bundled skill "${skillId}" declares bundled: true ` +
        `but skills/${skillId}/ directory does not exist`
      );
    }

    const files = collectSkillFiles(skillDir);

    if (files.length === 0) {
      throw new Error(
        `${specRelPath}: bundled skill "${skillId}" — skills/${skillId}/ directory is empty`
      );
    }

    // Only mark changed if the list actually differs
    if (JSON.stringify(skill.files) !== JSON.stringify(files)) {
      skill.files = files;
      changed = true;
    }
  }

  if (changed) {
    writeFileSync(specPath, stringifyYaml(spec, { lineWidth: 0 }), "utf8");
  }

  return changed;
}

function computeBundleStats(bundleDir, repoRoot) {
  const files = collectBundleFiles(bundleDir);
  const hash = createHash("sha256");
  let totalSize = 0;
  let latestMtimeMs = 0;

  for (const filePath of files) {
    const rel = toPosixPath(relative(repoRoot, filePath));
    const content = readFileSync(filePath);
    const fileStat = statSync(filePath);

    totalSize += fileStat.size;
    latestMtimeMs = Math.max(latestMtimeMs, fileStat.mtimeMs);

    hash.update(rel);
    hash.update("\0");
    hash.update(content);
    hash.update("\0");
  }

  return {
    sizeBytes: totalSize,
    checksum: `sha256:${hash.digest("hex")}`,
    updatedAt: latestMtimeMs > 0 ? new Date(latestMtimeMs).toISOString() : new Date().toISOString(),
  };
}

function discoverBundles(repoRoot, packagesRoot) {
  const results = [];
  if (!existsSync(packagesRoot)) return results;
  const packageTypes = readdirSync(packagesRoot, { withFileTypes: true });

  for (const typeEntry of packageTypes) {
    if (!typeEntry.isDirectory()) continue;

    const typeDir = join(packagesRoot, typeEntry.name);

    // Skills get a 3-level walk: packages/skills/<author>/<id>/spec.yaml.
    // This lets standalone skills use scoped slugs ("author/skill-id") so the
    // registry can host independent skill contributions per DHP v2 §5/§6.
    if (typeEntry.name === "skills") {
      const authors = readdirSync(typeDir, { withFileTypes: true });
      for (const authorEntry of authors) {
        if (!authorEntry.isDirectory()) continue;
        const authorDir = join(typeDir, authorEntry.name);
        const skillEntries = readdirSync(authorDir, { withFileTypes: true });

        for (const skillEntry of skillEntries) {
          const skillPath = join(authorDir, skillEntry.name);
          if (skillEntry.isFile() && skillEntry.name.endsWith(".yaml")) {
            const rel = toPosixPath(relative(repoRoot, skillPath));
            throw new Error(
              `Legacy single-file skill detected at ${rel}. ` +
                `Use bundle directory format: packages/skills/<author>/<slug>/spec.yaml`
            );
          }
          if (!skillEntry.isDirectory()) continue;
          const specPath = join(skillPath, "spec.yaml");
          if (!existsSync(specPath)) continue;
          results.push({
            typeDirName: typeEntry.name,
            bundleDir: skillPath,
            specPath,
          });
        }
      }
      continue;
    }

    const children = readdirSync(typeDir, { withFileTypes: true });

    for (const child of children) {
      const childPath = join(typeDir, child.name);

      if (child.isFile() && child.name.endsWith(".yaml")) {
        const rel = toPosixPath(relative(repoRoot, childPath));
        throw new Error(
          `Legacy single-file package detected at ${rel}. Use bundle directory format: packages/<type>/<slug>/spec.yaml`
        );
      }

      if (!child.isDirectory()) continue;

      const specPath = join(childPath, "spec.yaml");
      if (!existsSync(specPath)) {
        continue;
      }

      results.push({
        typeDirName: typeEntry.name,
        bundleDir: childPath,
        specPath,
      });
    }
  }

  return results.sort((a, b) => a.bundleDir.localeCompare(b.bundleDir));
}

function normalizeForCheck(index) {
  const clone = {
    ...index,
    generated_at: "",
    apps: Array.isArray(index.apps)
      ? index.apps.map((app) => ({ ...app, updated_at: "" }))
      : [],
  };
  return JSON.stringify(clone);
}

function buildIndex(repoRoot, source) {
  const packagesRoot = join(repoRoot, "packages");
  const bundles = discoverBundles(repoRoot, packagesRoot);
  const apps = [];

  for (const { typeDirName, bundleDir, specPath } of bundles) {
    const raw = readFileSync(specPath, "utf8");
    const specRelPath = toPosixPath(relative(repoRoot, specPath));
    const spec = parseSpec(raw, specRelPath);

    // Auto-populate `files` for bundled skills by scanning skills/{id}/ directories.
    // Rewrites spec.yaml only when the file list has changed. Fails fast on missing dirs.
    const rewritten = syncBundledSkillFiles(spec, bundleDir, specPath, specRelPath);
    if (rewritten) {
      process.stdout.write(`[build-index] updated bundled skill files in ${specRelPath}\n`);
    }

    const bundleRelPath = toPosixPath(relative(repoRoot, bundleDir));

    // For unscoped slugs the directory basename is the slug.
    // For scoped slugs (skills only) the slug is "<author>/<id>" and the
    // path under packages/skills/ encodes the same two segments.
    const dirParts = bundleRelPath.split("/");
    const isSkillsTree = typeDirName === "skills";
    const slugFromDir = isSkillsTree && dirParts.length >= 4
      ? `${dirParts[dirParts.length - 2]}/${dirParts[dirParts.length - 1]}`
      : basename(bundleDir);
    const store = spec.store && typeof spec.store === "object" ? spec.store : {};
    const slug = typeof store.slug === "string" && store.slug.length > 0 ? store.slug : slugFromDir;

    if (!isValidSlug(slug)) {
      throw new Error(
        `Invalid slug "${slug}" in ${specRelPath}. ` +
          `Expected unscoped (e.g. "hn-daily") or scoped "<author>/<id>" (e.g. "openkursar/xhs-search").`
      );
    }

    // Scoped slugs are only legal under packages/skills/<author>/<id>/.
    // Reject mixing schemes (e.g. a scoped digital human slug).
    if (isScopedSlug(slug) && !isSkillsTree) {
      throw new Error(
        `Scoped slug "${slug}" in ${specRelPath} is only valid for type=skill under packages/skills/`
      );
    }

    if (slug !== slugFromDir) {
      throw new Error(
        `Slug mismatch in ${specRelPath}: store.slug=${slug} but bundle directory=${slugFromDir}`
      );
    }

    // Strict semver required for type=skill (DHP v2 §5.1).
    if (spec.type === "skill") {
      if (typeof spec.version !== "string" || !STRICT_SEMVER_REGEX.test(spec.version)) {
        throw new Error(
          `Invalid skill version "${spec.version}" in ${specRelPath}. ` +
            `Skills require strict semver MAJOR.MINOR.PATCH (e.g. "1.0.0").`
        );
      }
    }

    const bundleStats = computeBundleStats(bundleDir, repoRoot);
    const i18n = extractI18nSummary(spec.i18n);

    // Pass through store.meta as-is — an open extension container for
    // implementation-specific data. The build script does not interpret
    // the contents; it only ensures the value is a plain object.
    const meta =
      store.meta !== null &&
      typeof store.meta === "object" &&
      !Array.isArray(store.meta)
        ? store.meta
        : undefined;

    const entry = {
      slug,
      name: typeof spec.name === "string" ? spec.name : slug,
      version: typeof spec.version === "string" ? spec.version : "0.0.0",
      author: typeof spec.author === "string" ? spec.author : "unknown",
      description: typeof spec.description === "string" ? spec.description : "",
      type: typeof spec.type === "string" ? spec.type : "automation",
      format: "bundle",
      path: bundleRelPath,
      size_bytes: bundleStats.sizeBytes,
      checksum: bundleStats.checksum,
      category: typeof store.category === "string" ? store.category : "other",
      tags: Array.isArray(store.tags) ? store.tags.filter((tag) => typeof tag === "string") : [],
      icon: typeof spec.icon === "string" ? spec.icon : undefined,
      locale: typeof store.locale === "string" ? store.locale : undefined,
      min_app_version: typeof store.min_app_version === "string" ? store.min_app_version : undefined,
      requires_mcps: mapMcpDependencyIds(spec.requires && spec.requires.mcps),
      requires_skills: mapSkillDependencyIds(spec.requires && spec.requires.skills),
      // DHP v2: full skill dependency objects with optional version constraints.
      // Keep requires_skills (string[]) for back-compat consumers.
      requires_skills_full: mapSkillDependencies(spec.requires && spec.requires.skills, specRelPath),
      updated_at: bundleStats.updatedAt,
      ...(i18n ? { i18n } : {}),
      ...(meta ? { meta } : {}),
    };

    apps.push(entry);
  }

  apps.sort((a, b) => a.slug.localeCompare(b.slug));

  return {
    version: 1,
    generated_at: new Date().toISOString(),
    source,
    apps,
  };
}

// Split the consolidated index into per-type files.
// Each split file uses the same envelope as the consolidated index.
function splitIndexByType(index) {
  const filterByType = (typeName) => ({
    version: index.version,
    generated_at: index.generated_at,
    source: index.source,
    apps: index.apps.filter((app) => app.type === typeName),
  });
  return {
    "digital-humans.json": filterByType("automation"),
    "skills.json": filterByType("skill"),
    "mcps.json": filterByType("mcp"),
  };
}

// Legacy consolidated index — kept for back-compat for at least one major
// version cycle per DHP v2 §15. Consumers should migrate to the split files.
function legacyConsolidatedIndex(index) {
  return {
    ...index,
    deprecated:
      "Use digital-humans.json / skills.json / mcps.json. " +
      "index.json is scheduled for removal in DHP v3.",
  };
}

const SPLIT_FILENAMES = ["digital-humans.json", "skills.json", "mcps.json"];
const ALL_OUTPUT_FILENAMES = ["index.json", ...SPLIT_FILENAMES];

function main() {
  const repoRoot = process.cwd();
  const { source, check } = parseArgs(process.argv.slice(2));

  const index = buildIndex(repoRoot, source);
  const splits = splitIndexByType(index);
  const consolidated = legacyConsolidatedIndex(index);

  const outputs = {
    "index.json": consolidated,
    ...splits,
  };

  if (check) {
    for (const filename of ALL_OUTPUT_FILENAMES) {
      const filePath = join(repoRoot, filename);
      let currentContent = "";
      try {
        currentContent = readFileSync(filePath, "utf8");
      } catch {
        currentContent = "";
      }

      if (!currentContent) {
        process.stderr.write(`${filename} is missing. Run: node scripts/build-index.mjs\n`);
        process.exit(1);
      }

      let current;
      try {
        current = JSON.parse(currentContent);
      } catch {
        process.stderr.write(`${filename} is invalid JSON. Run: node scripts/build-index.mjs\n`);
        process.exit(1);
      }

      if (normalizeForCheck(current) !== normalizeForCheck(outputs[filename])) {
        process.stderr.write(`${filename} is out of date. Run: node scripts/build-index.mjs\n`);
        process.exit(1);
      }
    }

    process.stdout.write(
      `[build-index] all outputs up to date (` +
        `index.json=${consolidated.apps.length}, ` +
        `digital-humans=${splits["digital-humans.json"].apps.length}, ` +
        `skills=${splits["skills.json"].apps.length}, ` +
        `mcps=${splits["mcps.json"].apps.length})\n`
    );
    return;
  }

  for (const filename of ALL_OUTPUT_FILENAMES) {
    const filePath = join(repoRoot, filename);
    const content = `${JSON.stringify(outputs[filename], null, 2)}\n`;
    writeFileSync(filePath, content, "utf8");
  }

  process.stdout.write(
    `[build-index] wrote ` +
      `index.json (${consolidated.apps.length} apps, deprecated), ` +
      `digital-humans.json (${splits["digital-humans.json"].apps.length}), ` +
      `skills.json (${splits["skills.json"].apps.length}), ` +
      `mcps.json (${splits["mcps.json"].apps.length})\n`
  );
}

main();
