import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import ts from "typescript";
import {
  declaredCssVariables,
  undefinedCssVariables,
} from "./css-variable-contract.mjs";

const appRoots = ["web/apps/enterprise/src", "web/apps/platform/src"];
const styleRoots = [...appRoots, "web/packages/ui/src"];
const cssFiles = files(styleRoots, ["*.css"]);
const sourceFiles = files(appRoots, ["*.ts", "*.tsx"]);
const failures = [];
const variableFiles = files(
  [...styleRoots, "web/packages/design-tokens/src"],
  ["*.css", "*.tsx"],
);
const declaredVariables = declaredCssVariables(
  variableFiles.map((file) => readFileSync(file, "utf8")),
);
for (const file of cssFiles) {
  // HeroUI/React Aria writes --trigger-width on the select popover at runtime.
  const runtime = file
    .replaceAll("\\", "/")
    .endsWith("web/packages/ui/src/foundation.css")
    ? new Set(["--trigger-width"])
    : new Set();
  for (const missing of undefinedCssVariables(
    readFileSync(file, "utf8"),
    declaredVariables,
    runtime,
  ))
    failures.push(
      `${file}:${missing.line} undefined design variable ${missing.name}`,
    );
}
const allowedClass = /^(?:argus-|is-|active$|dark$|light$)/;
const governedProperties =
  /^(?:font|font-size|gap|row-gap|column-gap|padding(?:-[a-z]+)?|margin(?:-[a-z]+)?|border-radius)$/;

function files(roots, globs) {
  const args = ["--files", ...roots];
  for (const glob of globs) args.push("-g", glob);
  const output = execFileSync("rg", args, { encoding: "utf8" }).trim();
  return output ? output.split("\n") : [];
}

function lineOf(source, offset) {
  return source.slice(0, offset).split("\n").length;
}

for (const file of cssFiles) {
  const source = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  if (source.includes("argus-var(")) {
    failures.push(`${file}: malformed CSS custom property reference`);
  }
  if (file.includes("/apps/")) {
    for (const rule of source.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      if (
        !/\.argus-(?:button|input|textarea|select__trigger|combobox|checkbox|choice-list__item|switch)(?:--[a-z]+)?(?![-\w])/.test(
          rule[1],
        )
      )
        continue;
      if (
        /(?:^|;)\s*(?:height|min-height|font(?:-[a-z]+)?|padding(?:-[a-z]+)?|border-radius|--control-[a-z-]+)\s*:/.test(
          rule[2],
        )
      ) {
        failures.push(
          `${file}:${lineOf(source, rule.index)} shared control geometry and typography belong in @argus/ui`,
        );
      }
    }
  }
  for (const match of source.matchAll(/\.([A-Za-z_][A-Za-z0-9_-]*)/g)) {
    if (file.includes("/apps/") && !allowedClass.test(match[1])) {
      failures.push(
        `${file}:${lineOf(source, match.index)} class .${match[1]} must use .argus-*`,
      );
    }
  }
  for (const match of source.matchAll(
    /#[0-9a-fA-F]{3,8}|(?:rgb|hsl)a?\(|(?<![-\w])(?:white|black)(?![-\w])/g,
  )) {
    failures.push(
      `${file}:${lineOf(source, match.index)} hard-coded color ${match[0]}`,
    );
  }
  for (const match of source.matchAll(/([a-z-]+)\s*:\s*([^;{}]+)/g)) {
    if (!governedProperties.test(match[1])) continue;
    if (/-?(?:\d+\.?\d*|\.\d+)(?:px|rem|em)/.test(match[2])) {
      failures.push(
        `${file}:${lineOf(source, match.index)} ${match[1]} must use a design token`,
      );
    }
    if (match[1] === "border-radius" && /\d+%/.test(match[2])) {
      failures.push(
        `${file}:${lineOf(source, match.index)} border-radius must use a design token`,
      );
    }
  }
}

function checkLiteral(file, source, node) {
  const text = node.text ?? "";
  for (const token of text.split(/\s+/).filter(Boolean)) {
    if (/^[A-Za-z_][A-Za-z0-9_-]*$/.test(token) && !allowedClass.test(token)) {
      failures.push(
        `${file}:${lineOf(source, node.getStart())} class ${token} must use argus-*`,
      );
    }
  }
}

for (const file of sourceFiles) {
  const source = readFileSync(file, "utf8");
  const sourceFile = ts.createSourceFile(
    file,
    source,
    ts.ScriptTarget.Latest,
    true,
    file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  function inspectClassName(node) {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      checkLiteral(file, source, node);
      return;
    }
    if (ts.isJsxExpression(node) && node.expression) {
      const expression = node.expression;
      if (
        ts.isStringLiteral(expression) ||
        ts.isNoSubstitutionTemplateLiteral(expression)
      ) {
        checkLiteral(file, source, expression);
      } else if (ts.isTemplateExpression(expression)) {
        checkLiteral(file, source, expression.head);
        for (const span of expression.templateSpans) {
          checkLiteral(file, source, span.literal);
        }
      }
    }
  }
  function visit(node) {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      /^@(?:heroui|radix-ui)\//.test(node.moduleSpecifier.text)
    ) {
      failures.push(
        `${file}:${lineOf(source, node.getStart())} primitives must be imported from @argus/ui`,
      );
    }
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      /^(?:button|select|textarea|datalist)$/.test(
        node.tagName.getText(sourceFile),
      ) &&
      !file.includes(".test.")
    ) {
      failures.push(
        `${file}:${lineOf(source, node.getStart())} use shared control instead of ${node.tagName.getText(sourceFile)}`,
      );
    }
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      node.tagName.getText(sourceFile) === "input" &&
      !file.includes(".test.")
    ) {
      const type = node.attributes.properties.find(
        (p) => ts.isJsxAttribute(p) && p.name.getText(sourceFile) === "type",
      );
      if (
        !type ||
        !type.initializer ||
        !ts.isStringLiteral(type.initializer) ||
        type.initializer.text !== "file"
      )
        failures.push(
          `${file}:${lineOf(source, node.getStart())} only native file input is an allowed business control exception`,
        );
    }
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      node.tagName.getText(sourceFile) === "Button"
    ) {
      for (const p of node.attributes.properties)
        if (
          ts.isJsxAttribute(p) &&
          /^(onClick|disabled|loading)$/.test(p.name.getText(sourceFile))
        )
          failures.push(
            `${file}:${lineOf(source, p.getStart())} Button uses onPress, isDisabled and isPending`,
          );
    }
    if (
      ts.isJsxAttribute(node) &&
      node.name.getText(sourceFile) === "className" &&
      node.initializer
    ) {
      inspectClassName(node.initializer);
    }
    ts.forEachChild(node, visit);
  }
  visit(sourceFile);
}

if (failures.length > 0) {
  throw new Error(`Web style checks failed:\n${failures.join("\n")}`);
}

console.log("Web style checks passed");
